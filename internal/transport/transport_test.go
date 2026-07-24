package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/akt/internal/workflow/steps"
	aclient "pkg.akt.dev/go/node/client"
	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

// Workflow message identifiers, matching internal/workflow/adapters and the
// builtin workflow definitions.
const (
	msgCreateDeployment = "deployment.MsgCreateDeployment"
	msgCloseDeployment  = "deployment.MsgCloseDeployment"
)

// --- fakes (mirroring internal/workflow/adapters test fakes) ---

// fakeTxClient records broadcast messages and returns a canned response.
type fakeTxClient struct {
	resp interface{}
	err  error
	msgs []sdk.Msg
}

func (f *fakeTxClient) BroadcastMsgs(_ context.Context, msgs []sdk.Msg, _ ...cv1beta3.BroadcastOption) (interface{}, error) {
	f.msgs = msgs
	return f.resp, f.err
}

func (f *fakeTxClient) BroadcastTx(_ context.Context, _ sdk.Tx, _ ...cv1beta3.BroadcastOption) (interface{}, error) {
	return f.resp, f.err
}

// fakeChainSDKClient embeds the aclient.Client interface so only the methods
// exercised by a test need to be stubbed; anything else panics.
type fakeChainSDKClient struct {
	aclient.Client

	tx   cv1beta3.TxClient
	cctx sdkclient.Context
}

func (f *fakeChainSDKClient) Tx() cv1beta3.TxClient            { return f.tx }
func (f *fakeChainSDKClient) ClientContext() sdkclient.Context { return f.cctx }

func testOwner() sdk.AccAddress {
	return sdk.AccAddress(bytes.Repeat([]byte{0x01}, 20))
}

// recordingStepsClient records delegated calls for translation tests.
type recordingStepsClient struct {
	calls   int
	msgType string
	params  map[string]string
	path    string
}

func (r *recordingStepsClient) BroadcastTx(_ context.Context, msgType string, params map[string]string) (*steps.TxResult, error) {
	r.calls++
	r.msgType = msgType
	r.params = params

	return &steps.TxResult{TxHash: "REC"}, nil
}

func (r *recordingStepsClient) Query(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	r.path = path
	r.params = params

	return json.RawMessage(`{"bids":[]}`), nil
}

// --- Kind routing smoke tests ---

// TestChainTransportKindAndRouting verifies NewChain reports KindChain and
// routes a tx step through the real chain adapter to the node client.
func TestChainTransportKindAndRouting(t *testing.T) {
	tx := &fakeTxClient{resp: &sdk.TxResponse{TxHash: "ABC123", Height: 42}}
	tr := NewChain(&fakeChainSDKClient{
		tx:   tx,
		cctx: sdkclient.Context{FromAddress: testOwner()},
	})

	if tr.Kind() != KindChain {
		t.Errorf("Kind() = %q, want %q", tr.Kind(), KindChain)
	}

	res, err := tr.BroadcastTx(context.Background(), msgCloseDeployment, map[string]string{"dseq": "12345"})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}
	if res.TxHash != "ABC123" || res.Height != 42 {
		t.Errorf("result = %+v, want the fake tx response", res)
	}
	if len(tx.msgs) != 1 {
		t.Errorf("broadcast msgs = %d, want 1", len(tx.msgs))
	}
}

// TestConsoleTransportKindAndRouting verifies NewConsole reports KindConsole
// and routes a create-deployment through the real console adapter with the
// unified deposit syntax translated to a plain USD number on the wire.
func TestConsoleTransportKindAndRouting(t *testing.T) {
	var gotMethod, gotPath string
	var gotData map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path

		var body struct {
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		gotData = body.Data

		_, _ = w.Write([]byte(`{"data":{"dseq":"777","manifest":"[]","signTx":{"code":0,"transactionHash":"H1"}}}`))
	}))
	t.Cleanup(srv.Close)

	tr := NewConsole(console.New(srv.URL, "test-key"), nil, t.TempDir(), "test-ctx")

	if tr.Kind() != KindConsole {
		t.Errorf("Kind() = %q, want %q", tr.Kind(), KindConsole)
	}

	res, err := tr.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
		"sdl":     "services:\n  web:\n    image: nginx\n", // raw SDL content
		"deposit": "5usd",                                  // unified syntax, not the wire form
	})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/v1/deployments" {
		t.Errorf("request = %s %s, want POST /v1/deployments", gotMethod, gotPath)
	}
	if dep, ok := gotData["deposit"].(float64); !ok || dep != 5 {
		t.Errorf("wire deposit = %v, want 5 (USD) translated from \"5usd\"", gotData["deposit"])
	}
	if res.TxHash != "H1" {
		t.Errorf("TxHash = %q, want H1", res.TxHash)
	}
}

// --- deposit translation tests ---

// TestChainTransportDepositPassThrough verifies coin and auto forms reach
// the chain adapter unchanged.
func TestChainTransportDepositPassThrough(t *testing.T) {
	for _, deposit := range []string{"5000000uakt", "5akt", "auto", ""} {
		rec := &recordingStepsClient{}
		tr := newChainTransport(rec)

		if _, err := tr.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
			"sdl":     "app.yaml",
			"deposit": deposit,
		}); err != nil {
			t.Errorf("deposit %q: unexpected error: %v", deposit, err)
			continue
		}
		if rec.params["deposit"] != deposit {
			t.Errorf("deposit %q: delegated as %q, want pass-through", deposit, rec.params["deposit"])
		}
	}
}

// TestChainTransportRejectsUSDDeposit verifies USD and bare forms fail on
// the chain rail with the cross-rail guidance, before any delegation.
func TestChainTransportRejectsUSDDeposit(t *testing.T) {
	for _, deposit := range []string{"5usd", "$5", "5.50usd", "5"} {
		rec := &recordingStepsClient{}
		tr := newChainTransport(rec)

		_, err := tr.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{"deposit": deposit})
		if err == nil {
			t.Errorf("deposit %q: expected cross-rail error", deposit)
			continue
		}
		for _, want := range []string{"console-api context", "5000000uakt"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("deposit %q: error %q does not mention %q", deposit, err, want)
			}
		}
		if rec.calls != 0 {
			t.Errorf("deposit %q: adapter was called %d times, want 0", deposit, rec.calls)
		}
	}
}

// TestConsoleTransportRewritesUSDDeposit verifies USD and bare forms are
// rewritten to the plain USD number the console adapter expects, without
// mutating the caller's params.
func TestConsoleTransportRewritesUSDDeposit(t *testing.T) {
	tests := []struct {
		deposit string
		want    string
	}{
		{"5usd", "5"},
		{"$5.50", "5.5"},
		{"5", "5"},
		{"0.5usd", "0.5"},
	}

	for _, tt := range tests {
		rec := &recordingStepsClient{}
		tr := newConsoleTransport(rec)

		params := map[string]string{"sdl": "app.yaml", "deposit": tt.deposit}
		if _, err := tr.BroadcastTx(context.Background(), msgCreateDeployment, params); err != nil {
			t.Errorf("deposit %q: unexpected error: %v", tt.deposit, err)
			continue
		}
		if rec.params["deposit"] != tt.want {
			t.Errorf("deposit %q: delegated as %q, want %q", tt.deposit, rec.params["deposit"], tt.want)
		}
		if rec.params["sdl"] != "app.yaml" {
			t.Errorf("deposit %q: sdl param lost in translation: %v", tt.deposit, rec.params)
		}
		if params["deposit"] != tt.deposit {
			t.Errorf("deposit %q: caller's params mutated to %q", tt.deposit, params["deposit"])
		}
	}
}

// TestConsoleTransportRejectsCoinDeposit verifies coin forms fail on the
// console rail with the cross-rail guidance, before any delegation.
func TestConsoleTransportRejectsCoinDeposit(t *testing.T) {
	for _, deposit := range []string{"5000000uakt", "5akt"} {
		rec := &recordingStepsClient{}
		tr := newConsoleTransport(rec)

		_, err := tr.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{"deposit": deposit})
		if err == nil {
			t.Errorf("deposit %q: expected cross-rail error", deposit)
			continue
		}
		if !strings.Contains(err.Error(), "console deposits are in USD; use e.g. 5usd") {
			t.Errorf("deposit %q: error %q lacks the console USD guidance", deposit, err)
		}
		if rec.calls != 0 {
			t.Errorf("deposit %q: adapter was called %d times, want 0", deposit, rec.calls)
		}
	}
}

// TestTransportWithoutDepositParam verifies steps without a deposit param
// (close, lease, queries) delegate untouched.
func TestTransportWithoutDepositParam(t *testing.T) {
	rec := &recordingStepsClient{}
	tr := newConsoleTransport(rec)

	params := map[string]string{"dseq": "42"}
	if _, err := tr.BroadcastTx(context.Background(), msgCloseDeployment, params); err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}
	if rec.msgType != msgCloseDeployment || rec.params["dseq"] != "42" {
		t.Errorf("delegated call = %q %v", rec.msgType, rec.params)
	}

	if _, err := tr.Query(context.Background(), "market.bids", map[string]string{"dseq": "42"}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if rec.path != "market.bids" {
		t.Errorf("delegated query path = %q, want market.bids", rec.path)
	}
}
