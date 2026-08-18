// Package adapters implements the narrow client interfaces the workflow
// engine's step executors depend on (steps.ChainClient and
// steps.ProviderClient), backed by the real Akash node and provider gateway
// clients.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"

	"pkg.akt.dev/akt/internal/workflow/steps"
	aclient "pkg.akt.dev/go/node/client"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	depositv1 "pkg.akt.dev/go/node/types/deposit/v1"

	aktprovider "pkg.akt.dev/akt/internal/provider"
)

// Workflow tx message identifiers, matching the `msg:` values used by the
// builtin workflow definitions (internal/workflow/builtin/*.yaml).
const (
	msgCreateDeployment = "deployment.MsgCreateDeployment"
	msgUpdateDeployment = "deployment.MsgUpdateDeployment"
	msgCloseDeployment  = "deployment.MsgCloseDeployment"
	msgCreateLease      = "market.MsgCreateLease"
)

// Workflow query path identifiers, matching the `query:` values used by the
// builtin workflow definitions.
const (
	queryMarketBids   = "market.bids"
	queryMarketLeases = "market.leases"
	queryDeployments  = "deployment.deployments"
)

// depositAuto is the deposit param value that requests the chain minimum.
const depositAuto = "auto"

var (
	errDeploymentUpdate              = errors.New("deployment update failed")
	errDeploymentUpdateGroupsChanged = fmt.Errorf("%w: groups are different than existing deployment, you cannot update groups", errDeploymentUpdate)
)

// chainClient adapts the Akash node client to the workflow steps.ChainClient
// interface.
type chainClient struct {
	cl aclient.Client
}

// NewChainClient wraps an Akash node client into the narrow interface the
// workflow step executors use to broadcast transactions and run queries.
func NewChainClient(cl aclient.Client) steps.ChainClient {
	return &chainClient{cl: cl}
}

// BroadcastTx builds the sdk.Msg identified by msgType from the resolved
// workflow step params, broadcasts it, and returns the tx result. The
// TxResult.Data payload is a flat JSON object of the identifying fields of
// the message (always including "dseq" for deployment/market messages) so
// that workflow templates such as
// {{ (index .Steps "create-deployment").dseq }} resolve correctly.
func (c *chainClient) BroadcastTx(ctx context.Context, msgType string, params map[string]string) (*steps.TxResult, error) {
	owner := c.cl.ClientContext().GetFromAddress()

	var (
		msg  sdk.Msg
		data map[string]string
	)

	switch msgType {
	case msgCreateDeployment:
		dseq, err := c.deriveDSeq(ctx, params)
		if err != nil {
			return nil, err
		}

		dep, err := c.resolveDeposit(ctx, params["deposit"])
		if err != nil {
			return nil, err
		}

		m, err := buildCreateDeploymentMsg(owner, params["sdl"], dseq, dep)
		if err != nil {
			return nil, err
		}

		msg = m
		data = map[string]string{
			"dseq":  strconv.FormatUint(dseq, 10),
			"owner": owner.String(),
		}

	case msgUpdateDeployment:
		m, groups, err := buildUpdateDeploymentMsg(owner, params)
		if err != nil {
			return nil, err
		}

		if err := c.verifyGroupsUnchanged(ctx, m.ID, groups); err != nil {
			return nil, err
		}

		msg = m
		data = map[string]string{
			"dseq":  strconv.FormatUint(m.ID.DSeq, 10),
			"owner": owner.String(),
		}

	case msgCloseDeployment:
		m, err := buildCloseDeploymentMsg(owner, params)
		if err != nil {
			return nil, err
		}

		msg = m
		data = map[string]string{
			"dseq":  strconv.FormatUint(m.ID.DSeq, 10),
			"owner": owner.String(),
		}

	case msgCreateLease:
		m, err := buildCreateLeaseMsg(owner, params)
		if err != nil {
			return nil, err
		}

		msg = m
		data = map[string]string{
			"dseq":     strconv.FormatUint(m.BidID.DSeq, 10),
			"gseq":     strconv.FormatUint(uint64(m.BidID.GSeq), 10),
			"oseq":     strconv.FormatUint(uint64(m.BidID.OSeq), 10),
			"provider": m.BidID.Provider,
			"owner":    owner.String(),
		}

	default:
		return nil, fmt.Errorf("unsupported workflow tx message %q", msgType)
	}

	return c.broadcast(ctx, msg, data)
}

// Query runs the chain query identified by path with the given filter params
// and returns the response as JSON. The JSON shape matches the query proto
// response, i.e. a top-level "bids", "leases", or "deployments" array, which
// is what the wait step's `until:` templates (e.g.
// {{ ge (len .Result.bids) 1 }}) expect.
func (c *chainClient) Query(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	switch path {
	case queryMarketBids:
		filters := mtypes.BidFilters{
			Owner:    params["owner"],
			Provider: params["provider"],
			State:    params["state"],
		}

		var err error
		if filters.DSeq, err = optionalUint64Param(params, "dseq"); err != nil {
			return nil, err
		}
		if filters.GSeq, err = optionalUint32Param(params, "gseq"); err != nil {
			return nil, err
		}
		if filters.OSeq, err = optionalUint32Param(params, "oseq"); err != nil {
			return nil, err
		}

		res, err := c.cl.Query().Market().Bids(ctx, &mtypes.QueryBidsRequest{Filters: filters})
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", path, err)
		}

		raw, err := c.marshalJSON(res)
		if err != nil {
			return nil, err
		}
		providers := make([]string, 0, len(res.Bids))
		for _, bid := range res.Bids {
			providers = append(providers, bid.Bid.ID.Provider)
		}

		return attachProviderMetadata(raw, aktprovider.FetchChainMetadata(ctx, c.cl.Query(), providers))

	case queryMarketLeases:
		filters := mv1.LeaseFilters{
			Owner:    params["owner"],
			Provider: params["provider"],
			State:    params["state"],
		}

		var err error
		if filters.DSeq, err = optionalUint64Param(params, "dseq"); err != nil {
			return nil, err
		}
		if filters.GSeq, err = optionalUint32Param(params, "gseq"); err != nil {
			return nil, err
		}
		if filters.OSeq, err = optionalUint32Param(params, "oseq"); err != nil {
			return nil, err
		}

		res, err := c.cl.Query().Market().Leases(ctx, &mtypes.QueryLeasesRequest{Filters: filters})
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", path, err)
		}

		return c.marshalJSON(res)

	case queryDeployments:
		filters := dv1beta.DeploymentFilters{
			Owner: params["owner"],
			State: params["state"],
		}

		var err error
		if filters.DSeq, err = optionalUint64Param(params, "dseq"); err != nil {
			return nil, err
		}

		res, err := c.cl.Query().Deployment().Deployments(ctx, &dv1beta.QueryDeploymentsRequest{Filters: filters})
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", path, err)
		}

		return c.marshalJSON(res)

	default:
		return nil, fmt.Errorf("unsupported workflow query path %q", path)
	}
}

// broadcast sends a single message and converts the response into a
// steps.TxResult carrying the given data payload.
func (c *chainClient) broadcast(ctx context.Context, msg sdk.Msg, data map[string]string) (*steps.TxResult, error) {
	res, err := c.cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
	if err != nil {
		return nil, err
	}

	txResp, ok := res.(*sdk.TxResponse)
	if !ok || txResp == nil {
		return nil, fmt.Errorf("unexpected broadcast response type %T", res)
	}

	if txResp.Code != 0 {
		return nil, fmt.Errorf("tx failed with code %d: %s", txResp.Code, txResp.RawLog)
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal tx result data: %w", err)
	}

	return &steps.TxResult{
		TxHash: txResp.TxHash,
		Height: txResp.Height,
		Code:   txResp.Code,
		Data:   raw,
	}, nil
}

// deriveDSeq returns the deployment sequence from params if present, or
// defaults it to the current block height like `akt tx deployment create`.
func (c *chainClient) deriveDSeq(ctx context.Context, params map[string]string) (uint64, error) {
	if v := params["dseq"]; v != "" {
		dseq, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse param %q: %w", "dseq", err)
		}

		return dseq, nil
	}

	syncInfo, err := c.cl.Node().SyncInfo(ctx)
	if err != nil {
		return 0, err
	}

	if syncInfo.CatchingUp {
		return 0, fmt.Errorf("cannot generate DSEQ from last block height. node is catching up")
	}

	return uint64(syncInfo.LatestBlockHeight), nil // nolint: gosec
}

// resolveDeposit turns the workflow "deposit" param into a deposit object.
// "auto" (or empty) queries the chain minimum deployment deposit, mirroring
// DetectDeploymentDeposit used by `akt tx deployment create`.
func (c *chainClient) resolveDeposit(ctx context.Context, depositStr string) (depositv1.Deposit, error) {
	resolved, err := ResolveChainDepositValue(ctx, c.cl, depositStr)
	if err != nil {
		return depositv1.Deposit{}, err
	}

	return depositFromString(resolved)
}

// ResolveChainDepositValue returns the exact coin the chain adapter will put
// on a deployment message. Planning and execution share this function so an
// auto dry-run cannot advertise a value different from the broadcast path.
func ResolveChainDepositValue(ctx context.Context, cl aclient.LightClient, depositStr string) (string, error) {
	if depositStr != "" && depositStr != depositAuto {
		return depositStr, nil
	}

	resp, err := cl.Query().Deployment().Params(ctx, &dv1beta.QueryParamsRequest{})
	if err != nil {
		return "", err
	}
	for _, coin := range resp.Params.MinDeposits {
		if coin.Denom == "uact" {
			return fmt.Sprintf("%s%s", coin.Amount, coin.Denom), nil
		}
	}

	return "", fmt.Errorf("chain deployment parameters contain no uact minimum deposit")
}

// verifyGroupsUnchanged replicates the safety check performed by
// `akt tx deployment update`: the SDL's groups must match the deployment's
// existing on-chain groups, since group changes are not allowed on update.
func (c *chainClient) verifyGroupsUnchanged(ctx context.Context, id dv1.DeploymentID, groups dv1beta.GroupSpecs) error {
	existingDeployment, err := c.cl.Query().Deployment().Deployment(ctx, &dv1beta.QueryDeploymentRequest{ID: id})
	if err != nil {
		return err
	}

	existingGroups := existingDeployment.GetGroups()

	if len(existingGroups) != len(groups) {
		return errDeploymentUpdateGroupsChanged
	}

	for i, existingGroup := range existingGroups {
		if !existingGroup.GroupSpec.Equal(&groups[i]) {
			return errDeploymentUpdateGroupsChanged
		}
	}

	return nil
}

// marshalJSON encodes a query response to JSON, preferring the client's proto
// codec (canonical field names, enums as strings, empty arrays emitted) and
// falling back to encoding/json.
func (c *chainClient) marshalJSON(msg proto.Message) (json.RawMessage, error) {
	if cdc := c.cl.ClientContext().Codec; cdc != nil {
		if bz, err := cdc.MarshalJSON(msg); err == nil {
			return bz, nil
		}
	}

	return json.Marshal(msg)
}
