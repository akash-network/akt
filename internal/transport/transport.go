// Package transport is the translation layer between workflow actions and
// the rails that execute them.
//
// Actions (deploy, update, close, and future ones) are defined exactly once,
// as workflow definitions, in terms of abstract tx/query/provider steps. A
// Transport translates those steps onto a concrete backing rail: the chain
// transport signs and broadcasts real transactions (with provider-gateway
// calls for manifests), while the console transport maps the same steps onto
// Console API REST calls (SPEC §7.4–§7.5). The CLI surface is generated from
// the workflow definition and the rail is chosen per context at execution
// time, so adding a new action never requires per-rail redesign: define the
// action once and each Transport decides how to carry it. Transports also
// normalize cross-rail argument syntax — notably the unified --deposit forms
// (see ParseDeposit) — so identical command arguments work on every rail.
package transport

import (
	"context"
	"encoding/json"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	"pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/akt/internal/workflow/adapters"
	"pkg.akt.dev/akt/internal/workflow/steps"
	aclient "pkg.akt.dev/go/node/client"
)

// Kind identifies a backing rail.
type Kind string

// The supported backing rails.
const (
	// KindChain executes actions by signing and broadcasting chain
	// transactions locally (keyring auth).
	KindChain Kind = "chain"

	// KindConsole executes actions as Console API REST calls
	// (console-api auth).
	KindConsole Kind = "console"
)

// Transport translates workflow actions onto a backing rail.
type Transport interface {
	Kind() Kind
	steps.ChainClient
}

// NewChain creates the chain transport: workflow tx steps are built, signed,
// and broadcast locally through the Akash node client, and queries run
// against chain RPC/gRPC.
func NewChain(cl aclient.Client) Transport {
	return newChainTransport(adapters.NewChainClient(cl))
}

// NewConsole creates the console transport: workflow tx steps are mapped to
// Console API REST calls (SPEC §7.5). chainQueries, when non-nil, handles
// query steps directly against the chain; root/ctxName locate the
// per-context manifest cache used to pass the deployment manifest from
// create to lease.
func NewConsole(cc *console.Client, chainQueries steps.ChainClient, root, ctxName string) Transport {
	return newConsoleTransport(adapters.NewConsoleChainClient(cc, chainQueries, root, ctxName))
}

// NewProvider creates the provider-gateway client used by workflow provider
// steps on the chain rail (JWT or mTLS auth). The console rail has no
// provider client: the Console API submits manifests internally during lease
// creation (SPEC §7.4).
func NewProvider(cctx sdkclient.Context, authType string) steps.ProviderClient {
	return adapters.NewProviderClient(cctx, authType)
}

// newChainTransport wraps an existing chain-rail steps.ChainClient. Split
// from NewChain so tests can inject fakes.
func newChainTransport(inner steps.ChainClient) Transport {
	return &railClient{kind: KindChain, inner: inner}
}

// newConsoleTransport wraps an existing console-rail steps.ChainClient.
// Split from NewConsole so tests can inject fakes.
func newConsoleTransport(inner steps.ChainClient) Transport {
	return &railClient{kind: KindConsole, inner: inner}
}

// railClient implements Transport by normalizing cross-rail argument syntax
// and delegating to the rail's underlying adapter.
type railClient struct {
	kind  Kind
	inner steps.ChainClient
}

// Kind reports the backing rail.
func (r *railClient) Kind() Kind { return r.kind }

// BroadcastTx translates rail-independent argument syntax (currently the
// unified "deposit" param, see ParseDeposit) into the rail's native form,
// then delegates to the rail adapter. Cross-rail mistakes — a USD deposit on
// the chain rail, a coin deposit on the console rail — fail here with a
// clear error before anything is sent.
func (r *railClient) BroadcastTx(ctx context.Context, msgType string, params map[string]string) (*steps.TxResult, error) {
	translated, err := translateDepositParam(r.kind, params)
	if err != nil {
		return nil, err
	}

	return r.inner.BroadcastTx(ctx, msgType, translated)
}

// Query delegates to the rail adapter unchanged.
func (r *railClient) Query(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	return r.inner.Query(ctx, path, params)
}

// translateDepositParam rewrites a "deposit" param from the unified syntax
// into the rail-native value the underlying adapter expects. Params without
// a deposit key pass through untouched; the input map is never mutated.
func translateDepositParam(kind Kind, params map[string]string) (map[string]string, error) {
	raw, ok := params["deposit"]
	if !ok {
		return params, nil
	}

	dep, err := ParseDeposit(raw)
	if err != nil {
		return nil, err
	}

	value, err := dep.RailValue(kind)
	if err != nil {
		return nil, err
	}

	if value == raw {
		return params, nil
	}

	out := make(map[string]string, len(params))
	for k, v := range params {
		out[k] = v
	}
	out["deposit"] = value

	return out, nil
}
