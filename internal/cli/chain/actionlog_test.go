package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/akt/internal/actionlog"

	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
)

type fakeTxClient struct {
	resp interface{}
	err  error
}

func (f *fakeTxClient) BroadcastMsgs(_ context.Context, _ []sdk.Msg, _ ...cv1beta3.BroadcastOption) (interface{}, error) {
	return f.resp, f.err
}

func (f *fakeTxClient) BroadcastTx(_ context.Context, _ sdk.Tx, _ ...cv1beta3.BroadcastOption) (interface{}, error) {
	return f.resp, f.err
}

func newTestActionLogger(t *testing.T) *actionlog.Logger {
	t.Helper()

	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	return l
}

func TestLoggingTxClientRecordsBroadcast(t *testing.T) {
	l := newTestActionLogger(t)

	tx := &loggingTxClient{
		tx: &fakeTxClient{resp: &sdk.TxResponse{
			TxHash:  "ABC123",
			Height:  42,
			GasUsed: 100000,
			Code:    0,
		}},
		log:  l,
		cctx: sdkclient.Context{FromName: "alice"},
	}

	msg := &dv1beta.MsgCloseDeployment{ID: dv1.DeploymentID{Owner: "akash1owner", DSeq: 12345}}
	if _, err := tx.BroadcastMsgs(context.Background(), []sdk.Msg{msg}); err != nil {
		t.Fatalf("BroadcastMsgs: %v", err)
	}

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Type != actionlog.TypeTx {
		t.Errorf("type = %s, want tx", e.Type)
	}
	if e.Action != "deployment.MsgCloseDeployment" {
		t.Errorf("action = %s, want deployment.MsgCloseDeployment", e.Action)
	}
	if e.TxHash != "ABC123" || e.Height != 42 || e.GasUsed != 100000 {
		t.Errorf("tx result fields not recorded: %+v", e)
	}
	if e.DSeq != 12345 {
		t.Errorf("dseq = %d, want 12345", e.DSeq)
	}
	if e.Account != "alice" {
		t.Errorf("account = %s, want alice", e.Account)
	}
	if e.Status != "success" {
		t.Errorf("status = %s, want success", e.Status)
	}
}

// TestLoggingTxClientRecordsPendingBroadcast covers the default sync broadcast
// mode: the response is a CheckTx result with no height and no gas, so the entry
// must record neither (a zero is indistinguishable from a real reading) and must
// not claim the transaction succeeded (SPEC §5.4, §5.6).
func TestLoggingTxClientRecordsPendingBroadcast(t *testing.T) {
	l := newTestActionLogger(t)

	tx := &loggingTxClient{
		tx:   &fakeTxClient{resp: &sdk.TxResponse{TxHash: "PENDING1", Code: 0}},
		log:  l,
		cctx: sdkclient.Context{FromName: "alice"},
	}

	msg := &dv1beta.MsgCloseDeployment{ID: dv1.DeploymentID{DSeq: 99}}
	if _, err := tx.BroadcastMsgs(context.Background(), []sdk.Msg{msg}); err != nil {
		t.Fatalf("BroadcastMsgs: %v", err)
	}

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.TxHash != "PENDING1" {
		t.Errorf("tx hash = %q, want PENDING1", e.TxHash)
	}
	if e.Height != 0 || e.GasUsed != 0 {
		t.Errorf("unconfirmed broadcast must not record height/gas: %+v", e)
	}
	if e.Status != "pending" {
		t.Errorf("status = %q, want pending", e.Status)
	}
}

func TestLoggingTxClientRecordsFailure(t *testing.T) {
	l := newTestActionLogger(t)

	tx := &loggingTxClient{
		tx:  &fakeTxClient{err: errors.New("insufficient funds")},
		log: l,
	}

	msg := &mtypes.MsgCreateLease{BidID: mv1.BidID{
		Owner: "akash1owner", DSeq: 7, GSeq: 1, OSeq: 1, Provider: "akash1prov",
	}}
	if _, err := tx.BroadcastMsgs(context.Background(), []sdk.Msg{msg}); err == nil {
		t.Fatal("expected error to propagate")
	}

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Status != "failed" || e.Error != "insufficient funds" {
		t.Errorf("failure not recorded: %+v", e)
	}
	if e.Action != "market.MsgCreateLease" {
		t.Errorf("action = %s, want market.MsgCreateLease", e.Action)
	}
	if e.DSeq != 7 || e.Provider != "akash1prov" {
		t.Errorf("bid id fields not recorded: %+v", e)
	}
}

func TestLoggingTxClientSkipsGenerateOnly(t *testing.T) {
	l := newTestActionLogger(t)

	tx := &loggingTxClient{
		tx:   &fakeTxClient{resp: &sdk.TxResponse{TxHash: "XYZ"}},
		log:  l,
		cctx: sdkclient.Context{GenerateOnly: true},
	}

	msg := &dv1beta.MsgCloseDeployment{ID: dv1.DeploymentID{DSeq: 1}}
	if _, err := tx.BroadcastMsgs(context.Background(), []sdk.Msg{msg}); err != nil {
		t.Fatalf("BroadcastMsgs: %v", err)
	}

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("generate-only broadcast must not be logged, got %d entries", len(entries))
	}
}

func TestBroadcastNonZeroCodeReturnsError(t *testing.T) {
	l := newTestActionLogger(t)

	tx := &loggingTxClient{
		tx: &fakeTxClient{resp: &sdk.TxResponse{
			TxHash:    "FAIL123",
			Code:      11,
			Codespace: "sdk",
			RawLog:    "out of gas",
		}},
		log: l,
	}

	msg := &dv1beta.MsgCloseDeployment{ID: dv1.DeploymentID{DSeq: 1}}
	resp, err := tx.BroadcastMsgs(context.Background(), []sdk.Msg{msg})
	if err == nil {
		t.Fatal("non-zero code must surface as an error")
	}
	if resp == nil {
		t.Error("response must still be returned alongside the error")
	}
	for _, want := range []string{"code 11", "FAIL123", "out of gas"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}

	entries, readErr := l.Read(actionlog.Filter{})
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if len(entries) != 1 || entries[0].Status != "failed" {
		t.Errorf("failed broadcast not recorded as failed: %+v", entries)
	}
}

func TestBroadcastNonZeroCodeGenerateOnlyNoError(t *testing.T) {
	tx := &loggingTxClient{
		tx:   &fakeTxClient{resp: &sdk.TxResponse{Code: 11}},
		cctx: sdkclient.Context{GenerateOnly: true},
	}

	msg := &dv1beta.MsgCloseDeployment{ID: dv1.DeploymentID{DSeq: 1}}
	if _, err := tx.BroadcastMsgs(context.Background(), []sdk.Msg{msg}); err != nil {
		t.Fatalf("generate-only must not convert codes to errors: %v", err)
	}
}

func TestBroadcastNonZeroCodeSimulationReturnsErrorWithoutLog(t *testing.T) {
	l := newTestActionLogger(t)
	tx := &loggingTxClient{
		tx: &fakeTxClient{resp: &sdk.TxResponse{
			Code:      32,
			Codespace: "sdk",
			RawLog:    "account sequence mismatch",
		}},
		log:  l,
		cctx: sdkclient.Context{Simulate: true},
	}

	msg := &dv1beta.MsgCloseDeployment{ID: dv1.DeploymentID{DSeq: 1}}
	if _, err := tx.BroadcastMsgs(context.Background(), []sdk.Msg{msg}); err == nil {
		t.Fatal("non-zero simulation code must surface as an error")
	}

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("simulation must not be logged, got %d entries", len(entries))
	}
}

func TestWithActionLogWrapsWithoutLogger(t *testing.T) {
	// The wrapper must apply even without a logger so failed broadcasts
	// exit non-zero regardless of action log availability.
	tx := &loggingTxClient{
		tx: &fakeTxClient{resp: &sdk.TxResponse{Code: 5, RawLog: "boom"}},
	}

	msg := &dv1beta.MsgCloseDeployment{ID: dv1.DeploymentID{DSeq: 1}}
	if _, err := tx.BroadcastMsgs(context.Background(), []sdk.Msg{msg}); err == nil {
		t.Fatal("expected error without logger present")
	}
}

func TestShortMsgType(t *testing.T) {
	cases := map[string]string{
		"/akash.deployment.v1beta4.MsgCreateDeployment": "deployment.MsgCreateDeployment",
		"/cosmos.bank.v1beta1.MsgSend":                  "bank.MsgSend",
		"weird":                                         "weird",
	}

	for in, want := range cases {
		if got := shortMsgType(in); got != want {
			t.Errorf("shortMsgType(%q) = %q, want %q", in, got, want)
		}
	}
}
