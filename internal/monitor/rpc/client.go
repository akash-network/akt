package rpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdkcodec "github.com/cosmos/cosmos-sdk/codec"
	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/cosmos/gogoproto/proto"

	bmetypes "pkg.akt.dev/go/node/bme/v1"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"

	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/governance"
)

const (
	// DefaultRPCEndpoint is the default Akash Network RPC endpoint
	DefaultRPCEndpoint = "https://rpc.akt.dev/rpc"

	// DefaultRESTEndpoint is the default Akash Network REST endpoint
	DefaultRESTEndpoint = "https://rpc.akt.dev/rest"

	// DefaultTimeout for HTTP requests
	DefaultTimeout = 10 * time.Second
)

// Client is an RPC client for fetching consensus state
type Client struct {
	rpcEndpoint    string
	restEndpoint   string
	httpClient     *http.Client
	validators     []consensus.Validator // cached validators
	validatorsErr  error
	validatorsOnce sync.Once

	// oracleVersion caches which oracle REST API version is available.
	// "" = not yet probed, "v1" or "v2" = detected, "none" = neither works.
	oracleVersion string
}

// NewClient creates a new RPC client
func NewClient(rpcEndpoint, restEndpoint string) *Client {
	if rpcEndpoint == "" {
		rpcEndpoint = DefaultRPCEndpoint
	}
	if restEndpoint == "" {
		restEndpoint = DefaultRESTEndpoint
	}

	return &Client{
		rpcEndpoint:  rpcEndpoint,
		restEndpoint: restEndpoint,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// GetConsensusState fetches the current consensus state from the RPC endpoint
func (c *Client) GetConsensusState(ctx context.Context) (*consensus.ConsensusResponse, error) {
	reqURL := fmt.Sprintf("%s/consensus_state", c.rpcEndpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch consensus state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result consensus.ConsensusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse consensus state: %w", err)
	}

	return &result, nil
}

// GetValidators fetches all validators from the RPC endpoint with pagination
// Results are cached for subsequent calls (thread-safe)
func (c *Client) GetValidators() ([]consensus.Validator, error) {
	c.validatorsOnce.Do(func() {
		c.validators, c.validatorsErr = c.fetchValidators()
	})
	return c.validators, c.validatorsErr
}

// fetchValidators does the actual fetch (uses background context since it's called from sync.Once)
func (c *Client) fetchValidators() ([]consensus.Validator, error) {
	ctx := context.Background()
	var allValidators []consensus.Validator
	page := 1
	perPage := 100

	for {
		validators, total, err := c.fetchValidatorsPage(ctx, page, perPage)
		if err != nil {
			return nil, err
		}

		allValidators = append(allValidators, validators...)

		if len(allValidators) >= total || len(validators) == 0 {
			break
		}

		page++
	}

	return allValidators, nil
}

func (c *Client) fetchValidatorsPage(ctx context.Context, page, perPage int) ([]consensus.Validator, int, error) {
	reqURL := fmt.Sprintf("%s/validators?per_page=%d&page=%d", c.rpcEndpoint, perPage, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch validators: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}

	var result consensus.ValidatorsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, 0, fmt.Errorf("failed to parse validators: %w", err)
	}

	total := 0
	if _, err := fmt.Sscanf(result.Result.Total, "%d", &total); err != nil {
		return nil, 0, fmt.Errorf("failed to parse total validators: %w", err)
	}

	return result.Result.Validators, total, nil
}

// CommitResponse represents the JSON-RPC response from /commit
type CommitResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		SignedHeader struct {
			Header struct {
				Height string `json:"height"`
			} `json:"header"`
			Commit struct {
				Height     string            `json:"height"`
				Signatures []CommitSignature `json:"signatures"`
			} `json:"commit"`
		} `json:"signed_header"`
	} `json:"result"`
}

// CommitSignature represents a single validator's signature in a block commit
type CommitSignature struct {
	BlockIDFlag      int    `json:"block_id_flag"`
	ValidatorAddress string `json:"validator_address"`
}

// GetLatestCommit fetches the commit (signatures) for a recent block.
// Returns the block height and a set of validator addresses (uppercase hex)
// that signed. Signatures are ordered by Tendermint internal index (address
// order), NOT by voting power, so callers must match by address.
//
// The latest block's commit is non-canonical: it only contains the precommit
// signatures the node had locally collected when it committed (typically ~70%).
// The complete set of signatures is stored in the next block's LastCommit and
// is available via /commit?height=H-1 once that block exists. This method
// therefore fetches /commit to learn the tip height, then re-fetches the
// commit for height-1 which is guaranteed to be canonical and complete.
func (c *Client) GetLatestCommit(ctx context.Context) (int64, map[string]bool, error) {
	// First call: learn the latest height.
	tipHeight, _, err := c.fetchCommit(ctx, "")
	if err != nil {
		return 0, nil, err
	}

	// Re-fetch the previous block's commit which has the full signature set.
	if tipHeight > 1 {
		return c.fetchCommit(ctx, strconv.FormatInt(tipHeight-1, 10))
	}

	// Chain is at height 1 — fall back to whatever the tip has.
	return c.fetchCommit(ctx, "")
}

// fetchCommit fetches /commit for the given height (empty string = latest).
// Returns the block height, the set of signer addresses, and any error.
func (c *Client) fetchCommit(ctx context.Context, height string) (int64, map[string]bool, error) {
	reqURL := fmt.Sprintf("%s/commit", c.rpcEndpoint)
	if height != "" {
		reqURL = fmt.Sprintf("%s?height=%s", reqURL, height)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create commit request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to fetch commit: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("commit returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read commit response: %w", err)
	}

	var result CommitResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, nil, fmt.Errorf("failed to parse commit: %w", err)
	}

	h, _ := strconv.ParseInt(result.Result.SignedHeader.Commit.Height, 10, 64)

	signers := make(map[string]bool)
	for _, sig := range result.Result.SignedHeader.Commit.Signatures {
		if sig.BlockIDFlag == 2 && sig.ValidatorAddress != "" {
			signers[strings.ToUpper(sig.ValidatorAddress)] = true
		}
	}

	return h, signers, nil
}

// GetConsensusStateWithValidators fetches consensus state and parses it with cached validators
func (c *Client) GetConsensusStateWithValidators(ctx context.Context) (*consensus.State, error) {
	// Ensure validators are loaded
	validators, err := c.GetValidators()
	if err != nil {
		return nil, err
	}

	resp, err := c.GetConsensusState(ctx)
	if err != nil {
		return nil, err
	}

	return consensus.ParseConsensusState(resp, validators)
}

// Endpoint returns the current RPC endpoint
func (c *Client) Endpoint() string {
	return c.rpcEndpoint
}

// RESTEndpoint returns the current REST endpoint
func (c *Client) RESTEndpoint() string {
	return c.restEndpoint
}

// LCDValidatorsResponse represents the LCD API response for validators
type LCDValidatorsResponse struct {
	Validators []struct {
		Description struct {
			Moniker string `json:"moniker"`
		} `json:"description"`
		ConsensusPubkey struct {
			Type string `json:"@type"`
			Key  string `json:"key"`
		} `json:"consensus_pubkey"`
	} `json:"validators"`
	Pagination struct {
		NextKey string `json:"next_key"`
		Total   string `json:"total"`
	} `json:"pagination"`
}

// GetValidatorMonikers fetches validator monikers from the REST endpoint
// Returns a map of consensus pubkey (base64) -> moniker
func (c *Client) GetValidatorMonikers(ctx context.Context) (map[string]string, error) {
	monikers := make(map[string]string)
	nextKey := ""

	for {
		pageMonikers, newNextKey, err := c.fetchMonikersPage(ctx, nextKey)
		if err != nil {
			return nil, err
		}

		for k, v := range pageMonikers {
			monikers[k] = v
		}

		if newNextKey == "" {
			break
		}
		nextKey = newNextKey
	}

	return monikers, nil
}

func (c *Client) fetchMonikersPage(ctx context.Context, nextKey string) (map[string]string, string, error) {
	reqURL := fmt.Sprintf("%s/cosmos/staking/v1beta1/validators?pagination.limit=100", c.restEndpoint)
	if nextKey != "" {
		reqURL += "&pagination.key=" + url.QueryEscape(nextKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch validators from LCD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("LCD returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read LCD response: %w", err)
	}

	var result LCDValidatorsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse LCD validators: %w", err)
	}

	monikers := make(map[string]string)
	for _, v := range result.Validators {
		if v.ConsensusPubkey.Key != "" && v.Description.Moniker != "" {
			monikers[v.ConsensusPubkey.Key] = v.Description.Moniker
		}
	}

	return monikers, result.Pagination.NextKey, nil
}

// Provider represents an Akash provider with version info
type Provider struct {
	Owner        string
	HostURI      string
	Name         string
	AkashVersion string
	IsOnline     bool
	Country      string
	CPUAvailable uint64
	CPUTotal     uint64
	MemAvailable uint64
	MemTotal     uint64
	GPUAvailable uint64
	GPUTotal     uint64
	GPUModels    []string // unique GPU model names (e.g., "NVIDIA H100")
}

// ResourceStats represents CPU/memory/storage stats
type ResourceStats struct {
	Available uint64 `json:"available"`
	Total     uint64 `json:"total"`
}

// ProviderStatusResponse represents the response from provider's /status endpoint
type ProviderStatusResponse struct {
	Cluster struct {
		Inventory struct {
			Available struct {
				Nodes []struct {
					Name        string `json:"name"`
					Allocatable struct {
						CPU    uint64 `json:"cpu"`
						GPU    uint64 `json:"gpu"`
						Memory uint64 `json:"memory"`
					} `json:"allocatable"`
					Available struct {
						CPU    uint64 `json:"cpu"`
						GPU    uint64 `json:"gpu"`
						Memory uint64 `json:"memory"`
					} `json:"available"`
				} `json:"nodes"`
			} `json:"available"`
		} `json:"inventory"`
	} `json:"cluster"`
}

// ProviderNode represents a single node's resource information
type ProviderNode struct {
	Name           string
	CPUAllocatable uint64
	CPUAvailable   uint64
	MemAllocatable uint64
	MemAvailable   uint64
	GPUAllocatable uint64
	GPUAvailable   uint64
}

// GetNodes extracts node information from the status response
func (r *ProviderStatusResponse) GetNodes() []ProviderNode {
	rawNodes := r.Cluster.Inventory.Available.Nodes
	nodes := make([]ProviderNode, 0, len(rawNodes))
	for _, n := range rawNodes {
		nodes = append(nodes, ProviderNode{
			Name:           n.Name,
			CPUAllocatable: n.Allocatable.CPU,
			CPUAvailable:   n.Available.CPU,
			MemAllocatable: n.Allocatable.Memory,
			MemAvailable:   n.Available.Memory,
			GPUAllocatable: n.Allocatable.GPU,
			GPUAvailable:   n.Available.GPU,
		})
	}
	return nodes
}

// ProviderVersionResponse represents the response from provider's /version endpoint
type ProviderVersionResponse struct {
	Akash struct {
		Version string `json:"version"`
	} `json:"akash"`
}

// CompareVersions compares two semver-like version strings
// Returns: 1 if a > b, -1 if a < b, 0 if equal
func CompareVersions(a, b string) int {
	coreA, preA, okA := parseProviderVersion(a)
	coreB, preB, okB := parseProviderVersion(b)
	if !okA || !okB {
		return strings.Compare(a, b)
	}

	for i := 0; i < max(len(coreA), len(coreB)); i++ {
		numA := 0
		if i < len(coreA) {
			numA = coreA[i]
		}
		numB := 0
		if i < len(coreB) {
			numB = coreB[i]
		}
		if numA != numB {
			return compareInts(numA, numB)
		}
	}

	return compareProviderPrerelease(preA, preB)
}

func parseProviderVersion(value string) ([]int, string, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
	if coreAndPre, _, found := strings.Cut(value, "+"); found {
		value = coreAndPre
	}
	core, prerelease, _ := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	if len(parts) == 0 {
		return nil, "", false
	}

	numbers := make([]int, len(parts))
	for i, part := range parts {
		if part == "" {
			return nil, "", false
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return nil, "", false
		}
		numbers[i] = number
	}
	return numbers, prerelease, true
}

func compareProviderPrerelease(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}

	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	for i := 0; i < min(len(partsA), len(partsB)); i++ {
		if cmp := compareProviderVersionIdentifier(partsA[i], partsB[i]); cmp != 0 {
			return cmp
		}
	}
	return compareInts(len(partsA), len(partsB))
}

func compareProviderVersionIdentifier(a, b string) int {
	numA, errA := strconv.Atoi(a)
	numB, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		return compareInts(numA, numB)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}

	prefixA, suffixA, hasSuffixA := trailingVersionNumber(a)
	prefixB, suffixB, hasSuffixB := trailingVersionNumber(b)
	if hasSuffixA && hasSuffixB && prefixA == prefixB {
		return compareInts(suffixA, suffixB)
	}
	return strings.Compare(a, b)
}

func trailingVersionNumber(value string) (string, int, bool) {
	index := len(value)
	for index > 0 && value[index-1] >= '0' && value[index-1] <= '9' {
		index--
	}
	if index == len(value) {
		return "", 0, false
	}
	number, err := strconv.Atoi(value[index:])
	if err != nil {
		return "", 0, false
	}
	return value[:index], number, true
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// GetProviderVersions returns unique versions from providers, sorted latest first
func GetProviderVersions(providers []Provider) []string {
	versionSet := make(map[string]bool)
	for _, p := range providers {
		if p.AkashVersion != "" {
			versionSet[p.AkashVersion] = true
		}
	}

	versions := make([]string, 0, len(versionSet))
	for v := range versionSet {
		versions = append(versions, v)
	}

	sort.Slice(versions, func(i, j int) bool {
		return CompareVersions(versions[i], versions[j]) > 0
	})

	return versions
}

const ProviderQueryTimeout = 5 * time.Second

// NewProviderHTTPClient creates an HTTP client configured for querying providers.
// If insecureSkipVerify is true, TLS certificate verification is disabled.
// This is often needed for providers with self-signed certificates.
func NewProviderHTTPClient(insecureSkipVerify bool) *http.Client {
	return &http.Client{
		Timeout: ProviderQueryTimeout,
		Transport: &http.Transport{
			TLSClientConfig:     providerTLSConfig(insecureSkipVerify),
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

func providerTLSConfig(insecureSkipVerify bool) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec
	}
}

// QueryProviderStatus queries a provider's /status endpoint
func QueryProviderStatus(ctx context.Context, httpClient *http.Client, hostURI string) (*ProviderStatusResponse, error) {
	reqURL := strings.TrimSuffix(hostURI, "/") + "/status"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result ProviderStatusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QueryProviderVersion queries a provider's /version endpoint
func QueryProviderVersion(ctx context.Context, httpClient *http.Client, hostURI string) (*ProviderVersionResponse, error) {
	reqURL := strings.TrimSuffix(hostURI, "/") + "/version"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result ProviderVersionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ExtractHostname extracts the hostname from a URL
func ExtractHostname(hostURI string) string {
	// Remove protocol
	host := strings.TrimPrefix(hostURI, "https://")
	host = strings.TrimPrefix(host, "http://")

	// Remove port
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	return host
}

// GetModuleParams fetches parameters for a specific module from a REST endpoint
func (c *Client) GetModuleParams(ctx context.Context, module, endpoint string) (json.RawMessage, error) {
	reqURL := c.restEndpoint + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", module, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s params: %w", module, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s params returned status %d", module, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s params response: %w", module, err)
	}

	return json.RawMessage(body), nil
}

// GetGenericParams fetches generic parameters for a subspace
func (c *Client) GetGenericParams(ctx context.Context, subspace string) ([]governance.GenericParam, error) {
	// First get the list of keys for this subspace
	reqURL := c.restEndpoint + "/cosmos/params/v1beta1/subspaces"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create subspaces request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subspaces: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subspaces returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read subspaces response: %w", err)
	}

	var subspacesResp struct {
		Subspaces []struct {
			Subspace string   `json:"subspace"`
			Keys     []string `json:"keys"`
		} `json:"subspaces"`
	}

	if err := json.Unmarshal(body, &subspacesResp); err != nil {
		return nil, fmt.Errorf("failed to parse subspaces: %w", err)
	}

	// Find the subspace
	var keys []string
	for _, s := range subspacesResp.Subspaces {
		if s.Subspace == subspace {
			keys = s.Keys
			break
		}
	}

	if keys == nil {
		return nil, fmt.Errorf("subspace %s not found", subspace)
	}

	// Fetch each key
	var params []governance.GenericParam
	for _, key := range keys {
		param, err := c.GetGenericParam(ctx, subspace, key)
		if err != nil {
			// Log but continue with other keys
			continue
		}
		params = append(params, *param)
	}

	return params, nil
}

// GetGenericParam fetches a single generic parameter
func (c *Client) GetGenericParam(ctx context.Context, subspace, key string) (*governance.GenericParam, error) {
	reqURL := fmt.Sprintf("%s/cosmos/params/v1beta1/params?subspace=%s&key=%s", c.restEndpoint, subspace, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create param request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch param %s/%s: %w", subspace, key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("param %s/%s returned status %d", subspace, key, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read param response: %w", err)
	}

	var paramResp struct {
		Param governance.GenericParam `json:"param"`
	}

	if err := json.Unmarshal(body, &paramResp); err != nil {
		return nil, fmt.Errorf("failed to parse param: %w", err)
	}

	return &paramResp.Param, nil
}

// OracleState holds the result of oracle ABCI queries.
type OracleState struct {
	Prices     *oracletypes.QueryPricesResponse
	Aggregated map[string]*oracletypes.QueryAggregatedPriceResponse
	// Version is the detected oracle API version ("v1", "v2", or "none").
	Version string
}

// GetOracleState fetches oracle prices and aggregated prices via ABCI
// queries through the RPC endpoint.  On the first call it probes v2
// then v1 to discover which module version the chain supports, and
// caches the result.  Errors on individual sub-queries are swallowed
// so partial results are returned.
func (c *Client) GetOracleState(ctx context.Context) (*OracleState, error) {
	state := &OracleState{
		Aggregated: make(map[string]*oracletypes.QueryAggregatedPriceResponse),
	}

	// Determine which API version to use.
	version := c.oracleVersion
	if version == "" {
		version = c.probeOracleVersion(ctx)
		c.oracleVersion = version
	}
	state.Version = version

	if version == "none" {
		return state, nil
	}

	// 1. Fetch recent prices (also tells us which denoms exist).
	pricesPath := fmt.Sprintf("/akash.oracle.%s.Query/Prices", version)
	pricesReq := &oracletypes.QueryPricesRequest{
		Pagination: &querytypes.PageRequest{Limit: 50},
	}
	var pricesResp oracletypes.QueryPricesResponse
	if err := c.abciQuery(ctx, pricesPath, pricesReq, &pricesResp); err == nil {
		state.Prices = &pricesResp
	}

	// 2. Extract unique denoms.
	denoms := extractOracleDenoms(state.Prices)

	// 3. Fetch aggregated price for each denom.
	agPath := fmt.Sprintf("/akash.oracle.%s.Query/AggregatedPrice", version)
	for _, denom := range denoms {
		agReq := &oracletypes.QueryAggregatedPriceRequest{Denom: denom}
		var agResp oracletypes.QueryAggregatedPriceResponse
		if err := c.abciQuery(ctx, agPath, agReq, &agResp); err == nil {
			state.Aggregated[denom] = &agResp
		}
	}

	return state, nil
}

// probeOracleVersion tries the oracle Prices ABCI query at v2, then v1.
// Returns "v2", "v1", or "none".
func (c *Client) probeOracleVersion(ctx context.Context) string {
	req := &oracletypes.QueryPricesRequest{
		Pagination: &querytypes.PageRequest{Limit: 1},
	}
	var resp oracletypes.QueryPricesResponse
	for _, v := range []string{"v2", "v1"} {
		path := fmt.Sprintf("/akash.oracle.%s.Query/Prices", v)
		if err := c.abciQuery(ctx, path, req, &resp); err == nil {
			return v
		}
	}
	return "none"
}

// extractOracleDenoms extracts unique asset denoms from a prices response.
func extractOracleDenoms(resp *oracletypes.QueryPricesResponse) []string {
	if resp == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, p := range resp.Prices {
		d := p.ID.Denom
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// BMEState holds the result of BME ABCI queries.
type BMEState struct {
	Status *bmetypes.QueryStatusResponse
	Ledger *bmetypes.QueryLedgerRecordsResponse
}

// GetBMEState fetches BME status and recent ledger entries via ABCI
// queries through the RPC endpoint.  Errors on individual sub-queries
// are swallowed so partial results are returned.
func (c *Client) GetBMEState(ctx context.Context) (*BMEState, error) {
	state := &BMEState{}

	var statusResp bmetypes.QueryStatusResponse
	if err := c.abciQuery(ctx, "/akash.bme.v1.Query/Status", &bmetypes.QueryStatusRequest{}, &statusResp); err == nil {
		state.Status = &statusResp
	}

	ledgerReq := &bmetypes.QueryLedgerRecordsRequest{
		Pagination: &querytypes.PageRequest{Limit: 20, Reverse: true},
	}
	var ledgerResp bmetypes.QueryLedgerRecordsResponse
	if err := c.abciQuery(ctx, "/akash.bme.v1.Query/LedgerRecords", ledgerReq, &ledgerResp); err == nil {
		state.Ledger = &ledgerResp
	}

	return state, nil
}

// abciQuery performs a protobuf ABCI query via the RPC endpoint and
// unmarshals the response into dst.
func (c *Client) abciQuery(ctx context.Context, path string, req proto.Marshaler, dst proto.Unmarshaler) error {
	queryURL := fmt.Sprintf("%s/abci_query?path=%s",
		c.rpcEndpoint,
		url.QueryEscape(fmt.Sprintf("%q", path)),
	)

	reqData, err := req.Marshal()
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	if len(reqData) > 0 {
		queryURL += "&data=0x" + hex.EncodeToString(reqData)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var abciResp ABCIQueryResponse
	if err := json.Unmarshal(body, &abciResp); err != nil {
		return fmt.Errorf("parse ABCI response: %w", err)
	}
	if abciResp.Result.Response.Code != 0 {
		return fmt.Errorf("ABCI error: code=%d log=%s", abciResp.Result.Response.Code, abciResp.Result.Response.Log)
	}

	valueBytes, err := base64.StdEncoding.DecodeString(abciResp.Result.Response.Value)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}

	return dst.Unmarshal(valueBytes)
}

// GetAllGovernanceParams fetches all governance parameters
func (c *Client) GetAllGovernanceParams(ctx context.Context) (*governance.AllParams, error) {
	allParams := governance.NewAllParams()
	allParams.LastUpdated = time.Now()

	// Fetch standard module params
	for module, endpoint := range governance.StandardModuleEndpoints {
		var rawJSON json.RawMessage
		var err error
		if module == "gov" {
			rawJSON, err = c.getGovernanceParams(ctx)
		} else {
			rawJSON, err = c.GetModuleParams(ctx, module, endpoint)
		}
		if err != nil {
			// Log but continue with other modules
			allParams.Modules[module] = &governance.ModuleParams{
				Module:      module,
				Source:      "direct",
				Error:       err,
				LastFetched: time.Now(),
			}
			continue
		}

		allParams.Modules[module] = &governance.ModuleParams{
			Module:      module,
			Source:      "direct",
			RawJSON:     rawJSON,
			LastFetched: time.Now(),
		}
	}

	// Fetch generic params for each subspace
	for _, subspace := range governance.GenericModuleSubspaces {
		params, err := c.GetGenericParams(ctx, subspace)
		if err != nil {
			// Log but continue with other subspaces
			allParams.Modules[subspace] = &governance.ModuleParams{
				Module:      subspace,
				Source:      "generic",
				Error:       err,
				LastFetched: time.Now(),
			}
			continue
		}

		// Combine params into a single JSON object
		paramMap := make(map[string]json.RawMessage)
		for _, param := range params {
			paramMap[param.Key] = param.Value
		}

		rawJSON, err := json.Marshal(paramMap)
		if err != nil {
			allParams.Modules[subspace] = &governance.ModuleParams{
				Module:      subspace,
				Source:      "generic",
				Error:       err,
				LastFetched: time.Now(),
			}
			continue
		}

		allParams.Modules[subspace] = &governance.ModuleParams{
			Module:      subspace,
			Source:      "generic",
			RawJSON:     json.RawMessage(rawJSON),
			LastFetched: time.Now(),
		}
	}

	return allParams, nil
}

func (c *Client) getGovernanceParams(ctx context.Context) (json.RawMessage, error) {
	var response govv1.QueryParamsResponse
	request := govv1.QueryParamsRequest{}
	if err := c.abciQuery(
		ctx,
		"/cosmos.gov.v1.Query/Params",
		&request,
		&response,
	); err != nil {
		return nil, fmt.Errorf("fetch governance params: %w", err)
	}

	rawJSON, err := sdkcodec.ProtoMarshalJSON(&response, nil)
	if err != nil {
		return nil, fmt.Errorf("encode governance params: %w", err)
	}

	return json.RawMessage(rawJSON), nil
}
