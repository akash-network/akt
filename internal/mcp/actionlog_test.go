package mcp

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	aktconsole "pkg.akt.dev/akt/internal/console"
)

type actionLogTxStub struct {
	resp interface{}
	err  error
}

func (stub *actionLogTxStub) BroadcastMsgs(_ context.Context, _ []sdk.Msg, _ ...v1beta3.BroadcastOption) (interface{}, error) {
	return stub.resp, stub.err
}

func (stub *actionLogTxStub) BroadcastTx(_ context.Context, _ sdk.Tx, _ ...v1beta3.BroadcastOption) (interface{}, error) {
	return stub.resp, stub.err
}

func TestMCPChainWritesUseCLITransactionActionLog(t *testing.T) {
	logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	ctx := cliutil.WithActionLog(context.Background(), logger)
	srv, err := New(ctx, sdkclient.Context{}, "jwt", false, aktconsole.New("http://console.invalid", "key"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	deploymentQuery := &deploymentQueryStub{}
	tx := &actionLogTxStub{resp: &sdk.TxResponse{
		TxHash:  "MCP-SUCCESS",
		Height:  91,
		GasUsed: 1234,
	}}
	client := semanticClient{
		query: semanticQuery{deployment: deploymentQuery},
		tx:    tx,
		cctx: sdkclient.Context{
			FromName: "mcp-account",
		}.WithFromAddress(sdk.AccAddress(bytes.Repeat([]byte{1}, 20))),
	}
	srv.registerQueryTools(client, "jwt")
	srv.registerWriteTools(ctx, client, "jwt")

	read := callRegisteredTool(t, srv, 1, "akash_list_deployments", map[string]any{})
	if read.rpcError != nil || read.IsError {
		t.Fatalf("read-only chain call failed: rpc=%+v result=%s", read.rpcError, read.text())
	}

	success := callRegisteredTool(t, srv, 2, "akash_close_deployment", map[string]any{"dseq": float64(41)})
	if success.rpcError != nil || success.IsError {
		t.Fatalf("successful chain mutation failed: rpc=%+v result=%s", success.rpcError, success.text())
	}

	tx.resp = nil
	tx.err = errors.New("broadcast rejected")
	failure := callRegisteredTool(t, srv, 3, "akash_close_lease", map[string]any{
		"owner":    "akash1owner",
		"dseq":     float64(42),
		"gseq":     float64(2),
		"oseq":     float64(3),
		"provider": "akash1provider",
	})
	if failure.rpcError != nil || !failure.IsError {
		t.Fatalf("failed chain mutation result: rpc=%+v result=%s", failure.rpcError, failure.text())
	}

	entries, err := logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("one read and two MCP chain mutations logged %d entries, want exactly 2: %+v", len(entries), entries)
	}

	failed := entries[0]
	if failed.Type != actionlog.TypeTx || failed.Action != "market.MsgCloseLease" || failed.Status != "failed" || failed.Error != "broadcast rejected" {
		t.Errorf("failed chain entry = %+v", failed)
	}
	if failed.DSeq != 42 || failed.GSeq != 2 || failed.OSeq != 3 || failed.Provider != "akash1provider" {
		t.Errorf("failed chain identifiers = %+v", failed)
	}

	succeeded := entries[1]
	if succeeded.Type != actionlog.TypeTx || succeeded.Action != "deployment.MsgCloseDeployment" || succeeded.Status != "success" {
		t.Errorf("successful chain entry = %+v", succeeded)
	}
	if succeeded.DSeq != 41 || succeeded.Account != "mcp-account" || succeeded.TxHash != "MCP-SUCCESS" || succeeded.Height != 91 || succeeded.GasUsed != 1234 {
		t.Errorf("successful chain fields = %+v", succeeded)
	}
}

func TestMCPChainWriteWithoutActionLoggerStillExecutes(t *testing.T) {
	srv, err := New(context.Background(), sdkclient.Context{}, "jwt", false, aktconsole.New("http://console.invalid", "key"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tx := &actionLogTxStub{resp: &sdk.TxResponse{TxHash: "UNLOGGED", Height: 1}}
	client := semanticClient{
		tx:   tx,
		cctx: sdkclient.Context{}.WithFromAddress(sdk.AccAddress(bytes.Repeat([]byte{2}, 20))),
	}
	srv.registerWriteTools(context.Background(), client, "jwt")

	result := callRegisteredTool(t, srv, 1, "akash_close_deployment", map[string]any{"dseq": float64(9)})
	if result.rpcError != nil || result.IsError {
		t.Fatalf("nil-logger chain mutation failed: rpc=%+v result=%s", result.rpcError, result.text())
	}
}
