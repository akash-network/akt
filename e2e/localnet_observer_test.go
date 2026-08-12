package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// nativeNodeObserver reads the Docker localnet through the node image's own
// akash CLI. It intentionally shares no query client or decoder with akt.
type nativeNodeObserver struct {
	runner           nativeNodeCommandRunner
	commandTimeout   time.Duration
	operationTimeout time.Duration
	responseLimit    int
}

type nativeNodeCommandRunner interface {
	run(context.Context, io.Writer, io.Writer, ...string) error
}

type nativeNodeCommandRunnerFunc func(context.Context, io.Writer, io.Writer, ...string) error

func (runner nativeNodeCommandRunnerFunc) run(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	return runner(ctx, stdout, stderr, args...)
}

type dockerNativeNodeCommandRunner struct {
	container string
}

func (runner dockerNativeNodeCommandRunner) run(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	dockerArgs := make([]string, 0, len(args)+5)
	dockerArgs = append(dockerArgs, "exec", runner.container, "akash", "--home", "/data")
	dockerArgs = append(dockerArgs, args...)
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func nativeNodeObserverForLocalnet(net *localnet) (*nativeNodeObserver, error) {
	if net == nil {
		return nil, errors.New("native node observer requires a localnet")
	}
	if !net.HarnessOwned {
		return nil, errors.New("native node observer requires the Docker harness-owned localnet")
	}

	if strings.TrimSpace(net.Container) == "" || strings.ContainsAny(net.Container, "\t\r\n ") {
		return nil, errors.New("native node observer requires the exact Docker container identity")
	}
	return newNativeNodeObserver(dockerNativeNodeCommandRunner{container: net.Container}), nil
}

func newNativeNodeObserver(runner nativeNodeCommandRunner) *nativeNodeObserver {
	return &nativeNodeObserver{
		runner:           runner,
		commandTimeout:   10 * time.Second,
		operationTimeout: 30 * time.Second,
		responseLimit:    1 << 20,
	}
}

type nativeNodeBoundedBuffer struct {
	data      bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *nativeNodeBoundedBuffer) Write(data []byte) (int, error) {
	written := len(data)
	remaining := buffer.limit - buffer.data.Len()
	if remaining > len(data) {
		remaining = len(data)
	}
	if remaining > 0 {
		_, _ = buffer.data.Write(data[:remaining])
	}
	if remaining < len(data) {
		buffer.truncated = true
	}
	return written, nil
}

func (observer *nativeNodeObserver) runJSON(ctx context.Context, operation string, args ...string) ([]byte, error) {
	if observer == nil || observer.runner == nil {
		return nil, errors.New("native node observer is not configured")
	}
	if ctx == nil {
		return nil, fmt.Errorf("native node %s requires a context", operation)
	}
	if observer.commandTimeout <= 0 || observer.responseLimit <= 0 {
		return nil, errors.New("native node observer has invalid bounds")
	}

	commandCtx, cancel := context.WithTimeout(ctx, observer.commandTimeout)
	defer cancel()
	stdout := &nativeNodeBoundedBuffer{limit: observer.responseLimit + 1}
	stderr := &nativeNodeBoundedBuffer{limit: observer.responseLimit + 1}
	err := observer.runner.run(commandCtx, stdout, stderr, args...)
	if commandCtx.Err() != nil {
		return nil, fmt.Errorf("native node %s did not complete within its bound: %w", operation, commandCtx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("native node %s command failed (stdout %d bytes, stderr %d bytes)", operation, stdout.data.Len(), stderr.data.Len())
	}
	if stdout.truncated || stdout.data.Len() > observer.responseLimit {
		return nil, fmt.Errorf("native node %s response exceeded %d bytes", operation, observer.responseLimit)
	}
	if stderr.truncated || stderr.data.Len() > observer.responseLimit {
		return nil, fmt.Errorf("native node %s diagnostic exceeded %d bytes", operation, observer.responseLimit)
	}
	if stdout.data.Len() == 0 {
		return nil, fmt.Errorf("native node %s returned no JSON", operation)
	}

	return bytes.Clone(stdout.data.Bytes()), nil
}

func (observer *nativeNodeObserver) operationContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, errors.New("native node observation requires a context")
	}
	if observer == nil || observer.operationTimeout <= 0 {
		return nil, nil, errors.New("native node observer has invalid operation bound")
	}
	bounded, cancel := context.WithTimeout(ctx, observer.operationTimeout)
	return bounded, cancel, nil
}

type nativeNodeTransaction struct {
	Hash   string
	Height int64
	Code   uint32
	RawLog string
}

func (observer *nativeNodeObserver) queryTransaction(ctx context.Context, hash string) (nativeNodeTransaction, error) {
	canonicalHash, err := canonicalNativeNodeHash(hash)
	if err != nil {
		return nativeNodeTransaction{}, err
	}
	body, err := observer.runJSON(ctx, "transaction query", "query", "tx", canonicalHash, "--type", "hash", "--output", "json")
	if err != nil {
		return nativeNodeTransaction{}, err
	}
	return decodeNativeNodeTransaction(body, canonicalHash)
}

func (observer *nativeNodeObserver) waitForTransaction(ctx context.Context, hash string) (nativeNodeTransaction, error) {
	operationCtx, cancel, err := observer.operationContext(ctx)
	if err != nil {
		return nativeNodeTransaction{}, err
	}
	defer cancel()

	attempts := 0
	for {
		attempts++
		transaction, queryErr := observer.queryTransaction(operationCtx, hash)
		if queryErr == nil {
			return transaction, nil
		}
		if operationCtx.Err() != nil {
			return nativeNodeTransaction{}, fmt.Errorf("native transaction was not committed after %d bounded attempts: %w", attempts, operationCtx.Err())
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-operationCtx.Done():
			timer.Stop()
			return nativeNodeTransaction{}, fmt.Errorf("native transaction was not committed after %d bounded attempts: %w", attempts, operationCtx.Err())
		case <-timer.C:
		}
	}
}

func canonicalNativeNodeHash(hash string) (string, error) {
	hash = strings.TrimSpace(hash)
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("native node transaction hash must be exactly 32 hexadecimal bytes")
	}
	return strings.ToUpper(hash), nil
}

func decodeNativeNodeTransaction(data []byte, expectedHash string) (nativeNodeTransaction, error) {
	var wire struct {
		Hash   string          `json:"txhash"`
		Height json.RawMessage `json:"height"`
		Code   json.RawMessage `json:"code"`
		RawLog *string         `json:"raw_log"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nativeNodeTransaction{}, fmt.Errorf("decode native transaction response (%d bytes): %w", len(data), err)
	}
	if wire.Hash == "" || len(wire.Height) == 0 || len(wire.Code) == 0 || wire.RawLog == nil {
		return nativeNodeTransaction{}, errors.New("native transaction response omitted hash, height, code, or raw log")
	}
	if !strings.EqualFold(wire.Hash, expectedHash) {
		return nativeNodeTransaction{}, errors.New("native transaction response returned a different hash")
	}
	height, err := parseNativeNodeUint(wire.Height, 64)
	if err != nil || height == 0 || height > ^uint64(0)>>1 {
		return nativeNodeTransaction{}, errors.New("native transaction response contained an invalid height")
	}
	code, err := parseNativeNodeUint(wire.Code, 32)
	if err != nil {
		return nativeNodeTransaction{}, errors.New("native transaction response contained an invalid code")
	}
	return nativeNodeTransaction{
		Hash:   strings.ToUpper(wire.Hash),
		Height: int64(height),
		Code:   uint32(code),
		RawLog: *wire.RawLog,
	}, nil
}

func (observer *nativeNodeObserver) bankBalance(ctx context.Context, address, denom string) (*big.Int, error) {
	if err := validateNativeNodeToken("address", address); err != nil {
		return nil, err
	}
	if err := validateNativeNodeToken("denomination", denom); err != nil {
		return nil, err
	}
	body, err := observer.runJSON(ctx, "bank balance query", "query", "bank", "balances", address, "--denom", denom, "--output", "json")
	if err != nil {
		return nil, err
	}
	return decodeNativeNodeBankBalance(body, denom)
}

func decodeNativeNodeBankBalance(data []byte, expectedDenom string) (*big.Int, error) {
	var wire struct {
		Denom  string `json:"denom"`
		Amount string `json:"amount"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("decode native bank response (%d bytes): %w", len(data), err)
	}
	if wire.Denom != expectedDenom {
		return nil, errors.New("native bank response returned a different denomination")
	}
	amount, ok := new(big.Int).SetString(wire.Amount, 10)
	if !ok || amount.Sign() < 0 {
		return nil, errors.New("native bank response contained an invalid amount")
	}
	return amount, nil
}

type nativeNodeCertificateID struct {
	Owner  string
	Serial string
}

func (observer *nativeNodeObserver) certificateStates(ctx context.Context, owner string) (map[nativeNodeCertificateID]string, error) {
	if err := validateNativeNodeToken("certificate owner", owner); err != nil {
		return nil, err
	}
	operationCtx, cancel, err := observer.operationContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()

	result := make(map[nativeNodeCertificateID]string)
	pageKey := ""
	seenPageKeys := make(map[string]struct{})
	var expectedTotal *uint64
	for {
		args := []string{"query", "cert", "list", "--owner", owner, "--limit", "100", "--count-total", "--output", "json"}
		if pageKey != "" {
			args = append(args, "--page-key", pageKey)
		}
		body, err := observer.runJSON(operationCtx, "certificate list query", args...)
		if err != nil {
			return nil, err
		}
		page, nextKey, total, err := decodeNativeNodeCertificatePage(body, owner)
		if err != nil {
			return nil, err
		}
		if expectedTotal == nil {
			expectedTotal = &total
		} else if *expectedTotal != total {
			return nil, errors.New("native certificate pagination changed its total")
		}
		for id, state := range page {
			if _, duplicate := result[id]; duplicate {
				return nil, errors.New("native certificate pagination returned a duplicate identity")
			}
			result[id] = state
		}
		if nextKey == "" {
			if uint64(len(result)) != total {
				return nil, errors.New("native certificate pagination total did not match its entries")
			}
			return result, nil
		}
		if _, repeated := seenPageKeys[nextKey]; repeated {
			return nil, errors.New("native certificate pagination repeated a page key")
		}
		seenPageKeys[nextKey] = struct{}{}
		pageKey = nextKey
	}
}

func decodeNativeNodeCertificatePage(data []byte, owner string) (map[nativeNodeCertificateID]string, string, uint64, error) {
	var wire struct {
		Certificates []struct {
			Certificate struct {
				State string `json:"state"`
			} `json:"certificate"`
			Serial string `json:"serial"`
		} `json:"certificates"`
		Pagination json.RawMessage `json:"pagination"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, "", 0, fmt.Errorf("decode native certificate response (%d bytes): %w", len(data), err)
	}
	nextKey, total, err := decodeNativeNodePagination(wire.Pagination)
	if err != nil {
		return nil, "", 0, fmt.Errorf("decode native certificate pagination: %w", err)
	}
	result := make(map[nativeNodeCertificateID]string, len(wire.Certificates))
	for _, certificate := range wire.Certificates {
		if certificate.Serial == "" || certificate.Certificate.State == "" {
			return nil, "", 0, errors.New("native certificate response omitted serial or state")
		}
		if _, err := strconv.ParseUint(certificate.Serial, 10, 64); err != nil {
			return nil, "", 0, errors.New("native certificate response contained an invalid serial")
		}
		id := nativeNodeCertificateID{Owner: owner, Serial: certificate.Serial}
		if _, duplicate := result[id]; duplicate {
			return nil, "", 0, errors.New("native certificate page returned a duplicate identity")
		}
		result[id] = certificate.Certificate.State
	}
	return result, nextKey, total, nil
}

type nativeNodeDeploymentID struct {
	Owner string
	DSeq  string
}

func (observer *nativeNodeObserver) deploymentStates(ctx context.Context, owner string) (map[nativeNodeDeploymentID]string, error) {
	if err := validateNativeNodeToken("deployment owner", owner); err != nil {
		return nil, err
	}
	operationCtx, cancel, err := observer.operationContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()

	result := make(map[nativeNodeDeploymentID]string)
	pageKey := ""
	seenPageKeys := make(map[string]struct{})
	var expectedTotal *uint64
	for {
		args := []string{"query", "deployment", "list", "--owner", owner, "--limit", "100", "--count-total", "--output", "json"}
		if pageKey != "" {
			args = append(args, "--page-key", pageKey)
		}
		body, err := observer.runJSON(operationCtx, "deployment list query", args...)
		if err != nil {
			return nil, err
		}
		page, nextKey, total, err := decodeNativeNodeDeploymentPage(body, owner)
		if err != nil {
			return nil, err
		}
		if expectedTotal == nil {
			expectedTotal = &total
		} else if *expectedTotal != total {
			return nil, errors.New("native deployment pagination changed its total")
		}
		for id, state := range page {
			if _, duplicate := result[id]; duplicate {
				return nil, errors.New("native deployment pagination returned a duplicate identity")
			}
			result[id] = state
		}
		if nextKey == "" {
			if uint64(len(result)) != total {
				return nil, errors.New("native deployment pagination total did not match its entries")
			}
			return result, nil
		}
		if _, repeated := seenPageKeys[nextKey]; repeated {
			return nil, errors.New("native deployment pagination repeated a page key")
		}
		seenPageKeys[nextKey] = struct{}{}
		pageKey = nextKey
	}
}

func decodeNativeNodeDeploymentPage(data []byte, expectedOwner string) (map[nativeNodeDeploymentID]string, string, uint64, error) {
	var wire struct {
		Deployments []struct {
			Deployment struct {
				ID struct {
					Owner string `json:"owner"`
					DSeq  string `json:"dseq"`
				} `json:"id"`
				State string `json:"state"`
			} `json:"deployment"`
		} `json:"deployments"`
		Pagination json.RawMessage `json:"pagination"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, "", 0, fmt.Errorf("decode native deployment response (%d bytes): %w", len(data), err)
	}
	nextKey, total, err := decodeNativeNodePagination(wire.Pagination)
	if err != nil {
		return nil, "", 0, fmt.Errorf("decode native deployment pagination: %w", err)
	}
	result := make(map[nativeNodeDeploymentID]string, len(wire.Deployments))
	for _, item := range wire.Deployments {
		id := nativeNodeDeploymentID{Owner: item.Deployment.ID.Owner, DSeq: item.Deployment.ID.DSeq}
		if id.Owner != expectedOwner || item.Deployment.State == "" {
			return nil, "", 0, errors.New("native deployment response omitted or changed its identity or state")
		}
		if err := validateNativeNodeDecimal("deployment sequence", id.DSeq, 64); err != nil {
			return nil, "", 0, err
		}
		if _, duplicate := result[id]; duplicate {
			return nil, "", 0, errors.New("native deployment page returned a duplicate identity")
		}
		result[id] = item.Deployment.State
	}
	return result, nextKey, total, nil
}

type nativeNodeOrderID struct {
	Owner string
	DSeq  string
	GSeq  uint32
	OSeq  uint32
}

type nativeNodeOrder struct {
	ID    nativeNodeOrderID
	State string
}

func (observer *nativeNodeObserver) order(ctx context.Context, id nativeNodeOrderID) (nativeNodeOrder, error) {
	if err := validateNativeNodeToken("order owner", id.Owner); err != nil {
		return nativeNodeOrder{}, err
	}
	if err := validateNativeNodeDecimal("deployment sequence", id.DSeq, 64); err != nil {
		return nativeNodeOrder{}, err
	}
	body, err := observer.runJSON(ctx, "market order query",
		"query", "market", "order", "get",
		"--owner", id.Owner,
		"--dseq", id.DSeq,
		"--gseq", strconv.FormatUint(uint64(id.GSeq), 10),
		"--oseq", strconv.FormatUint(uint64(id.OSeq), 10),
		"--output", "json",
	)
	if err != nil {
		return nativeNodeOrder{}, err
	}
	return decodeNativeNodeOrder(body, id)
}

func decodeNativeNodeOrder(data []byte, expected nativeNodeOrderID) (nativeNodeOrder, error) {
	var wire struct {
		ID struct {
			Owner string          `json:"owner"`
			DSeq  string          `json:"dseq"`
			GSeq  json.RawMessage `json:"gseq"`
			OSeq  json.RawMessage `json:"oseq"`
		} `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nativeNodeOrder{}, fmt.Errorf("decode native order response (%d bytes): %w", len(data), err)
	}
	gseq, err := parseNativeNodeUint(wire.ID.GSeq, 32)
	if err != nil {
		return nativeNodeOrder{}, errors.New("native order response contained an invalid group sequence")
	}
	oseq, err := parseNativeNodeUint(wire.ID.OSeq, 32)
	if err != nil {
		return nativeNodeOrder{}, errors.New("native order response contained an invalid order sequence")
	}
	actual := nativeNodeOrderID{Owner: wire.ID.Owner, DSeq: wire.ID.DSeq, GSeq: uint32(gseq), OSeq: uint32(oseq)}
	if actual != expected || wire.State == "" {
		return nativeNodeOrder{}, errors.New("native order response omitted or changed its identity or state")
	}
	return nativeNodeOrder{ID: actual, State: wire.State}, nil
}

type nativeNodeGroupID struct {
	Owner string
	DSeq  string
	GSeq  uint32
}

type nativeNodeGroup struct {
	ID    nativeNodeGroupID
	State string
}

func (observer *nativeNodeObserver) group(ctx context.Context, id nativeNodeGroupID) (nativeNodeGroup, error) {
	if err := validateNativeNodeToken("group owner", id.Owner); err != nil {
		return nativeNodeGroup{}, err
	}
	if err := validateNativeNodeDecimal("deployment sequence", id.DSeq, 64); err != nil {
		return nativeNodeGroup{}, err
	}
	body, err := observer.runJSON(ctx, "deployment group query",
		"query", "deployment", "group", "get",
		"--owner", id.Owner,
		"--dseq", id.DSeq,
		"--gseq", strconv.FormatUint(uint64(id.GSeq), 10),
		"--output", "json",
	)
	if err != nil {
		return nativeNodeGroup{}, err
	}
	return decodeNativeNodeGroup(body, id)
}

func decodeNativeNodeGroup(data []byte, expected nativeNodeGroupID) (nativeNodeGroup, error) {
	var wire struct {
		ID struct {
			Owner string          `json:"owner"`
			DSeq  string          `json:"dseq"`
			GSeq  json.RawMessage `json:"gseq"`
		} `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nativeNodeGroup{}, fmt.Errorf("decode native group response (%d bytes): %w", len(data), err)
	}
	gseq, err := parseNativeNodeUint(wire.ID.GSeq, 32)
	if err != nil {
		return nativeNodeGroup{}, errors.New("native group response contained an invalid group sequence")
	}
	actual := nativeNodeGroupID{Owner: wire.ID.Owner, DSeq: wire.ID.DSeq, GSeq: uint32(gseq)}
	if actual != expected || wire.State == "" {
		return nativeNodeGroup{}, errors.New("native group response omitted or changed its identity or state")
	}
	return nativeNodeGroup{ID: actual, State: wire.State}, nil
}

type nativeNodeEscrowID struct {
	Scope string
	XID   string
}

type nativeNodeEscrowAccount struct {
	ID       nativeNodeEscrowID
	Owner    string
	State    string
	Balances map[string]*big.Rat
}

func (account nativeNodeEscrowAccount) balance(denom string) *big.Rat {
	if amount, exists := account.Balances[denom]; exists {
		return new(big.Rat).Set(amount)
	}
	return new(big.Rat)
}

func (observer *nativeNodeObserver) escrowAccount(ctx context.Context, owner, dseq string) (nativeNodeEscrowAccount, error) {
	if err := validateNativeNodeToken("escrow owner", owner); err != nil {
		return nativeNodeEscrowAccount{}, err
	}
	if err := validateNativeNodeDecimal("deployment sequence", dseq, 64); err != nil {
		return nativeNodeEscrowAccount{}, err
	}
	operationCtx, cancel, err := observer.operationContext(ctx)
	if err != nil {
		return nativeNodeEscrowAccount{}, err
	}
	defer cancel()

	xid := owner + "/" + dseq
	var result *nativeNodeEscrowAccount
	for _, state := range []string{"open", "closed", "overdrawn"} {
		pageKey := ""
		seenPageKeys := make(map[string]struct{})
		var stateCount uint64
		var expectedTotal *uint64
		for {
			args := []string{"query", "escrow", "accounts", state, "deployment/" + xid, "--limit", "100", "--count-total", "--output", "json"}
			if pageKey != "" {
				args = append(args, "--page-key", pageKey)
			}
			body, err := observer.runJSON(operationCtx, "escrow account query", args...)
			if err != nil {
				return nativeNodeEscrowAccount{}, err
			}
			accounts, nextKey, total, err := decodeNativeNodeEscrowPage(body, owner, xid, state)
			if err != nil {
				return nativeNodeEscrowAccount{}, err
			}
			if expectedTotal == nil {
				expectedTotal = &total
			} else if *expectedTotal != total {
				return nativeNodeEscrowAccount{}, errors.New("native escrow pagination changed its total")
			}
			stateCount += uint64(len(accounts))
			for index := range accounts {
				if result != nil {
					return nativeNodeEscrowAccount{}, errors.New("native escrow query returned more than one exact account")
				}
				account := accounts[index]
				result = &account
			}
			if nextKey == "" {
				if stateCount != total {
					return nativeNodeEscrowAccount{}, errors.New("native escrow pagination total did not match its entries")
				}
				break
			}
			if _, repeated := seenPageKeys[nextKey]; repeated {
				return nativeNodeEscrowAccount{}, errors.New("native escrow pagination repeated a page key")
			}
			seenPageKeys[nextKey] = struct{}{}
			pageKey = nextKey
		}
	}
	if result == nil {
		return nativeNodeEscrowAccount{}, errors.New("native escrow query returned no exact deployment account")
	}
	return *result, nil
}

func decodeNativeNodeEscrowPage(data []byte, expectedOwner, expectedXID, expectedState string) ([]nativeNodeEscrowAccount, string, uint64, error) {
	var wire struct {
		Accounts []struct {
			ID struct {
				Scope string `json:"scope"`
				XID   string `json:"xid"`
			} `json:"id"`
			State struct {
				Owner string `json:"owner"`
				State string `json:"state"`
				Funds []struct {
					Denom  string `json:"denom"`
					Amount string `json:"amount"`
				} `json:"funds"`
			} `json:"state"`
		} `json:"accounts"`
		Pagination json.RawMessage `json:"pagination"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, "", 0, fmt.Errorf("decode native escrow response (%d bytes): %w", len(data), err)
	}
	nextKey, total, err := decodeNativeNodePagination(wire.Pagination)
	if err != nil {
		return nil, "", 0, fmt.Errorf("decode native escrow pagination: %w", err)
	}
	accounts := make([]nativeNodeEscrowAccount, 0, len(wire.Accounts))
	for _, item := range wire.Accounts {
		if item.ID.Scope != "deployment" || item.ID.XID != expectedXID || item.State.Owner != expectedOwner || item.State.State != expectedState {
			return nil, "", 0, errors.New("native escrow response omitted or changed its identity or state")
		}
		account := nativeNodeEscrowAccount{
			ID:       nativeNodeEscrowID{Scope: item.ID.Scope, XID: item.ID.XID},
			Owner:    item.State.Owner,
			State:    item.State.State,
			Balances: make(map[string]*big.Rat, len(item.State.Funds)),
		}
		for _, coin := range item.State.Funds {
			if coin.Denom == "" {
				return nil, "", 0, errors.New("native escrow response contained a balance without a denomination")
			}
			amount, ok := new(big.Rat).SetString(coin.Amount)
			if !ok || amount.Sign() < 0 {
				return nil, "", 0, errors.New("native escrow response contained an invalid balance")
			}
			if _, duplicate := account.Balances[coin.Denom]; duplicate {
				return nil, "", 0, errors.New("native escrow response contained a duplicate denomination")
			}
			account.Balances[coin.Denom] = amount
		}
		accounts = append(accounts, account)
	}
	return accounts, nextKey, total, nil
}

func decodeNativeNodePagination(data json.RawMessage) (string, uint64, error) {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return "", 0, errors.New("pagination metadata was omitted")
	}
	var wire struct {
		NextKey json.RawMessage `json:"next_key"`
		Total   json.RawMessage `json:"total"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return "", 0, err
	}
	if len(wire.NextKey) == 0 || len(wire.Total) == 0 {
		return "", 0, errors.New("pagination omitted next_key or total")
	}
	nextKey := ""
	if !bytes.Equal(bytes.TrimSpace(wire.NextKey), []byte("null")) {
		if err := json.Unmarshal(wire.NextKey, &nextKey); err != nil || strings.TrimSpace(nextKey) == "" {
			return "", 0, errors.New("pagination contained an invalid next_key")
		}
	}
	total, err := parseNativeNodeUint(wire.Total, 64)
	if err != nil {
		return "", 0, errors.New("pagination contained an invalid total")
	}
	return nextKey, total, nil
}

func parseNativeNodeUint(raw json.RawMessage, bitSize int) (uint64, error) {
	value := strings.TrimSpace(string(raw))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return 0, err
		}
	}
	return strconv.ParseUint(value, 10, bitSize)
}

func validateNativeNodeToken(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\t\r\n ") {
		return fmt.Errorf("native node %s must be one non-empty token", name)
	}
	return nil
}

func validateNativeNodeDecimal(name, value string, bitSize int) error {
	if err := validateNativeNodeToken(name, value); err != nil {
		return err
	}
	parsed, err := strconv.ParseUint(value, 10, bitSize)
	if err != nil || strconv.FormatUint(parsed, 10) != value {
		return fmt.Errorf("native node %s must be a canonical unsigned integer", name)
	}
	return nil
}

func TestDecodeNativeNodeTransaction(t *testing.T) {
	t.Parallel()

	hash := "CA230C0FB50621EE98C864A0CFD0BE44770470CE47242694788EB1712F8041B2"
	got, err := decodeNativeNodeTransaction([]byte(`{"height":"6","txhash":"`+hash+`","code":0,"raw_log":""}`), hash)
	if err != nil {
		t.Fatalf("decodeNativeNodeTransaction() error = %v", err)
	}
	if got.Hash != hash || got.Height != 6 || got.Code != 0 || got.RawLog != "" {
		t.Fatalf("decodeNativeNodeTransaction() = %+v", got)
	}

	if _, err := decodeNativeNodeTransaction([]byte(`{"height":"0","txhash":"`+hash+`","code":0,"raw_log":""}`), hash); err == nil {
		t.Fatal("decodeNativeNodeTransaction() accepted a zero height")
	}
	if _, err := decodeNativeNodeTransaction([]byte(`{"height":"6","txhash":"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF","code":0,"raw_log":""}`), hash); err == nil {
		t.Fatal("decodeNativeNodeTransaction() accepted the wrong hash")
	}
}

func TestDecodeNativeNodeBankBalance(t *testing.T) {
	t.Parallel()

	got, err := decodeNativeNodeBankBalance([]byte(`{"denom":"uakt","amount":"1000000"}`), "uakt")
	if err != nil {
		t.Fatalf("decodeNativeNodeBankBalance() error = %v", err)
	}
	if got.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("decodeNativeNodeBankBalance() = %s", got)
	}

	for _, body := range []string{
		`{"denom":"uact","amount":"1000000"}`,
		`{"denom":"uakt","amount":"-1"}`,
		`{"denom":"uakt","amount":"1.5"}`,
	} {
		if _, err := decodeNativeNodeBankBalance([]byte(body), "uakt"); err == nil {
			t.Fatalf("decodeNativeNodeBankBalance() accepted %s", body)
		}
	}
}

func TestNativeNodeObserverCommandContract(t *testing.T) {
	owner := "akash1owner"
	hash := "CA230C0FB50621EE98C864A0CFD0BE44770470CE47242694788EB1712F8041B2"
	var calls []string
	runner := nativeNodeCommandRunnerFunc(func(_ context.Context, stdout, _ io.Writer, args ...string) error {
		call := strings.Join(args, " ")
		calls = append(calls, call)
		switch {
		case strings.HasPrefix(call, "query tx "):
			_, _ = io.WriteString(stdout, `{"height":"6","txhash":"`+hash+`","code":0,"raw_log":""}`)
		case strings.HasPrefix(call, "query bank balances "):
			_, _ = io.WriteString(stdout, `{"denom":"uakt","amount":"1000000"}`)
		case call == "query market order get --owner akash1owner --dseq 2 --gseq 1 --oseq 1 --output json":
			_, _ = io.WriteString(stdout, `{"id":{"owner":"akash1owner","dseq":"2","gseq":1,"oseq":1},"state":"open"}`)
		case call == "query deployment group get --owner akash1owner --dseq 2 --gseq 1 --output json":
			_, _ = io.WriteString(stdout, `{"id":{"owner":"akash1owner","dseq":"2","gseq":1},"state":"open"}`)
		case strings.HasPrefix(call, "query escrow accounts open "):
			_, _ = io.WriteString(stdout, `{"accounts":[{"id":{"scope":"deployment","xid":"akash1owner/2"},"state":{"owner":"akash1owner","state":"open","funds":[{"denom":"uact","amount":"5000000.000000000000000000"}]}}],"pagination":{"next_key":null,"total":"1"}}`)
		case strings.HasPrefix(call, "query escrow accounts closed "), strings.HasPrefix(call, "query escrow accounts overdrawn "):
			_, _ = io.WriteString(stdout, `{"accounts":[],"pagination":{"next_key":null,"total":"0"}}`)
		default:
			return errors.New("unexpected native command")
		}
		return nil
	})
	observer := newNativeNodeObserver(runner)
	ctx := context.Background()

	if transaction, err := observer.queryTransaction(ctx, hash); err != nil || transaction.Hash != hash {
		t.Fatalf("queryTransaction() = %+v, %v", transaction, err)
	}
	if balance, err := observer.bankBalance(ctx, owner, "uakt"); err != nil || balance.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("bankBalance() = %v, %v", balance, err)
	}
	orderID := nativeNodeOrderID{Owner: owner, DSeq: "2", GSeq: 1, OSeq: 1}
	if order, err := observer.order(ctx, orderID); err != nil || order.ID != orderID || order.State != "open" {
		t.Fatalf("order() = %+v, %v", order, err)
	}
	groupID := nativeNodeGroupID{Owner: owner, DSeq: "2", GSeq: 1}
	if group, err := observer.group(ctx, groupID); err != nil || group.ID != groupID || group.State != "open" {
		t.Fatalf("group() = %+v, %v", group, err)
	}
	escrow, err := observer.escrowAccount(ctx, owner, "2")
	if err != nil {
		t.Fatalf("escrowAccount() error = %v", err)
	}
	if escrow.ID != (nativeNodeEscrowID{Scope: "deployment", XID: owner + "/2"}) || escrow.Owner != owner || escrow.State != "open" {
		t.Fatalf("escrowAccount() = %+v", escrow)
	}
	if got := escrow.balance("uact"); got.Cmp(big.NewRat(5_000_000, 1)) != 0 {
		t.Fatalf("escrow balance = %s", got)
	}

	wantCalls := []string{
		"query tx " + hash + " --type hash --output json",
		"query bank balances " + owner + " --denom uakt --output json",
		"query market order get --owner " + owner + " --dseq 2 --gseq 1 --oseq 1 --output json",
		"query deployment group get --owner " + owner + " --dseq 2 --gseq 1 --output json",
		"query escrow accounts open deployment/" + owner + "/2 --limit 100 --count-total --output json",
		"query escrow accounts closed deployment/" + owner + "/2 --limit 100 --count-total --output json",
		"query escrow accounts overdrawn deployment/" + owner + "/2 --limit 100 --count-total --output json",
	}
	if strings.Join(calls, "\n") != strings.Join(wantCalls, "\n") {
		t.Fatalf("native commands:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(wantCalls, "\n"))
	}
}

func TestNativeNodeObserverExhaustsPagination(t *testing.T) {
	owner := "akash1owner"
	page := 0
	runner := nativeNodeCommandRunnerFunc(func(_ context.Context, stdout, _ io.Writer, args ...string) error {
		page++
		call := strings.Join(args, " ")
		switch page {
		case 1:
			if strings.Contains(call, "--page-key") {
				return errors.New("first page unexpectedly had a page key")
			}
			_, _ = io.WriteString(stdout, `{"certificates":[{"certificate":{"state":"valid"},"serial":"17"}],"pagination":{"next_key":"cGFnZS0y","total":"2"}}`)
		case 2:
			if !strings.HasSuffix(call, "--page-key cGFnZS0y") {
				return errors.New("second page omitted its native page key")
			}
			_, _ = io.WriteString(stdout, `{"certificates":[{"certificate":{"state":"revoked"},"serial":"18"}],"pagination":{"next_key":null,"total":"2"}}`)
		default:
			return errors.New("pagination requested an extra page")
		}
		return nil
	})

	states, err := newNativeNodeObserver(runner).certificateStates(context.Background(), owner)
	if err != nil {
		t.Fatalf("certificateStates() error = %v", err)
	}
	if page != 2 || states[nativeNodeCertificateID{Owner: owner, Serial: "17"}] != "valid" || states[nativeNodeCertificateID{Owner: owner, Serial: "18"}] != "revoked" {
		t.Fatalf("certificateStates() = %#v after %d pages", states, page)
	}
}

func TestDecodeNativeNodeDeploymentPage(t *testing.T) {
	t.Parallel()

	owner := "akash1owner"
	body := []byte(`{"deployments":[{"deployment":{"id":{"owner":"akash1owner","dseq":"2"},"state":"active"}}],"pagination":{"next_key":null,"total":"1"}}`)
	states, nextKey, total, err := decodeNativeNodeDeploymentPage(body, owner)
	if err != nil {
		t.Fatalf("decodeNativeNodeDeploymentPage() error = %v", err)
	}
	if nextKey != "" || total != 1 || states[nativeNodeDeploymentID{Owner: owner, DSeq: "2"}] != "active" {
		t.Fatalf("decodeNativeNodeDeploymentPage() = %#v, %q, %d", states, nextKey, total)
	}
}

func TestNativeNodeObserverDoesNotLeakCommandOutput(t *testing.T) {
	const secret = "twenty four mnemonic words must stay private"
	runner := nativeNodeCommandRunnerFunc(func(_ context.Context, stdout, stderr io.Writer, _ ...string) error {
		_, _ = io.WriteString(stdout, secret)
		_, _ = io.WriteString(stderr, secret)
		return errors.New(secret)
	})
	observer := newNativeNodeObserver(runner)

	_, err := observer.runJSON(context.Background(), "secret-safe probe", "query", "tx", "hash")
	if err == nil {
		t.Fatal("runJSON() accepted a failed command")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("runJSON() leaked command output: %v", err)
	}
}

func TestNativeNodeObserverBoundsCommands(t *testing.T) {
	runner := nativeNodeCommandRunnerFunc(func(ctx context.Context, _, _ io.Writer, _ ...string) error {
		<-ctx.Done()
		return ctx.Err()
	})
	observer := newNativeNodeObserver(runner)
	observer.commandTimeout = 5 * time.Millisecond

	_, err := observer.runJSON(context.Background(), "timeout probe", "query", "tx", "hash")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runJSON() error = %v, want deadline exceeded", err)
	}
}
