package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/akt/internal/workflow"
	aclient "pkg.akt.dev/go/node/client"
	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	depositv1 "pkg.akt.dev/go/node/types/deposit/v1"
)

const testSDLPath = "testdata/deployment.yaml"

func testOwner() sdk.AccAddress {
	return sdk.AccAddress(bytes.Repeat([]byte{0x01}, 20))
}

func testProviderAddr() sdk.AccAddress {
	return sdk.AccAddress(bytes.Repeat([]byte{0x02}, 20))
}

func testClientContext() sdkclient.Context {
	return sdkclient.Context{FromAddress: testOwner()}
}

func testDeposit(t *testing.T) depositv1.Deposit {
	t.Helper()

	dep, err := depositFromString("5000000uakt")
	if err != nil {
		t.Fatalf("depositFromString: %v", err)
	}

	return dep
}

// --- fakes ---

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

func newFakeChainClient(tx cv1beta3.TxClient, owner sdk.AccAddress) *chainClient {
	return &chainClient{cl: &fakeChainSDKClient{
		tx:   tx,
		cctx: sdkclient.Context{FromAddress: owner},
	}}
}

// --- msg builder tests ---

func TestBuildCreateDeploymentMsg(t *testing.T) {
	owner := testOwner()
	dep := testDeposit(t)

	msg, err := buildCreateDeploymentMsg(owner, testSDLPath, 12345, dep)
	if err != nil {
		t.Fatalf("buildCreateDeploymentMsg: %v", err)
	}

	if msg.ID.Owner != owner.String() {
		t.Errorf("owner = %s, want %s", msg.ID.Owner, owner.String())
	}
	if msg.ID.DSeq != 12345 {
		t.Errorf("dseq = %d, want 12345", msg.ID.DSeq)
	}
	if len(msg.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(msg.Groups))
	}
	if msg.Groups[0].Name != "westcoast" {
		t.Errorf("group name = %s, want westcoast", msg.Groups[0].Name)
	}
	if len(msg.Hash) == 0 {
		t.Error("hash is empty, want SDL version hash")
	}
	if !msg.Deposit.Amount.Equal(dep.Amount) {
		t.Errorf("deposit = %s, want %s", msg.Deposit.Amount, dep.Amount)
	}
	if len(msg.Deposit.Sources) != 2 {
		t.Errorf("deposit sources = %v, want [grant balance]", msg.Deposit.Sources)
	}
}

func TestBuildCreateDeploymentMsgErrors(t *testing.T) {
	owner := testOwner()
	dep := testDeposit(t)

	if _, err := buildCreateDeploymentMsg(owner, "", 1, dep); err == nil {
		t.Error("expected error for missing sdl path")
	}
	if _, err := buildCreateDeploymentMsg(owner, testSDLPath, 0, dep); err == nil {
		t.Error("expected error for zero dseq")
	}
	if _, err := buildCreateDeploymentMsg(owner, filepath.Join("testdata", "nope.yaml"), 1, dep); err == nil {
		t.Error("expected error for missing sdl file")
	}
}

func TestBuildUpdateDeploymentMsg(t *testing.T) {
	owner := testOwner()

	msg, groups, err := buildUpdateDeploymentMsg(owner, map[string]string{
		"sdl":  testSDLPath,
		"dseq": "42",
	})
	if err != nil {
		t.Fatalf("buildUpdateDeploymentMsg: %v", err)
	}

	if msg.ID.Owner != owner.String() || msg.ID.DSeq != 42 {
		t.Errorf("id = %+v, want owner %s dseq 42", msg.ID, owner.String())
	}
	if len(msg.Hash) == 0 {
		t.Error("hash is empty, want SDL version hash")
	}
	if len(groups) != 1 {
		t.Errorf("groups = %d, want 1", len(groups))
	}

	// The update hash must match the create hash for the same SDL.
	create, err := buildCreateDeploymentMsg(owner, testSDLPath, 42, testDeposit(t))
	if err != nil {
		t.Fatalf("buildCreateDeploymentMsg: %v", err)
	}
	if !bytes.Equal(msg.Hash, create.Hash) {
		t.Error("update hash differs from create hash for the same SDL")
	}
}

func TestBuildUpdateDeploymentMsgMissingParams(t *testing.T) {
	owner := testOwner()

	if _, _, err := buildUpdateDeploymentMsg(owner, map[string]string{"dseq": "42"}); err == nil {
		t.Error("expected error for missing sdl")
	}
	if _, _, err := buildUpdateDeploymentMsg(owner, map[string]string{"sdl": testSDLPath}); err == nil {
		t.Error("expected error for missing dseq")
	}
}

func TestBuildCloseDeploymentMsg(t *testing.T) {
	owner := testOwner()

	msg, err := buildCloseDeploymentMsg(owner, map[string]string{"dseq": "7"})
	if err != nil {
		t.Fatalf("buildCloseDeploymentMsg: %v", err)
	}

	if msg.ID.Owner != owner.String() || msg.ID.DSeq != 7 {
		t.Errorf("id = %+v, want owner %s dseq 7", msg.ID, owner.String())
	}

	if _, err := buildCloseDeploymentMsg(owner, nil); err == nil {
		t.Error("expected error for missing dseq")
	}
	if _, err := buildCloseDeploymentMsg(owner, map[string]string{"dseq": "abc"}); err == nil {
		t.Error("expected error for non-numeric dseq")
	}
}

func TestBuildCreateLeaseMsg(t *testing.T) {
	owner := testOwner()
	provider := testProviderAddr().String()

	msg, err := buildCreateLeaseMsg(owner, map[string]string{
		"dseq":     "7",
		"gseq":     "2",
		"oseq":     "3",
		"provider": provider,
	})
	if err != nil {
		t.Fatalf("buildCreateLeaseMsg: %v", err)
	}

	id := msg.BidID
	if id.Owner != owner.String() || id.DSeq != 7 || id.GSeq != 2 || id.OSeq != 3 || id.Provider != provider {
		t.Errorf("bid id = %+v", id)
	}
}

func TestBuildCreateLeaseMsgDefaults(t *testing.T) {
	msg, err := buildCreateLeaseMsg(testOwner(), map[string]string{
		"dseq":     "7",
		"provider": testProviderAddr().String(),
	})
	if err != nil {
		t.Fatalf("buildCreateLeaseMsg: %v", err)
	}

	if msg.BidID.GSeq != 1 || msg.BidID.OSeq != 1 {
		t.Errorf("gseq/oseq = %d/%d, want 1/1", msg.BidID.GSeq, msg.BidID.OSeq)
	}
}

func TestBuildCreateLeaseMsgErrors(t *testing.T) {
	owner := testOwner()
	provider := testProviderAddr().String()

	if _, err := buildCreateLeaseMsg(owner, map[string]string{"provider": provider}); err == nil {
		t.Error("expected error for missing dseq")
	}
	if _, err := buildCreateLeaseMsg(owner, map[string]string{"dseq": "7"}); err == nil {
		t.Error("expected error for missing provider")
	}
	if _, err := buildCreateLeaseMsg(owner, map[string]string{"dseq": "7", "provider": "not-bech32"}); err == nil {
		t.Error("expected error for invalid provider address")
	}
}

func TestDepositFromString(t *testing.T) {
	dep, err := depositFromString("5000000uakt")
	if err != nil {
		t.Fatalf("depositFromString: %v", err)
	}

	if dep.Amount.Denom != "uakt" || dep.Amount.Amount.Int64() != 5000000 {
		t.Errorf("deposit = %s", dep.Amount)
	}
	if len(dep.Sources) != 2 || dep.Sources[0] != depositv1.SourceGrant || dep.Sources[1] != depositv1.SourceBalance {
		t.Errorf("sources = %v, want [grant balance]", dep.Sources)
	}

	if _, err := depositFromString("not-a-coin"); err == nil {
		t.Error("expected error for invalid coin string")
	}
}

// --- BroadcastTx plumbing tests ---

func TestBroadcastTxUnsupportedMsgType(t *testing.T) {
	c := newFakeChainClient(&fakeTxClient{}, testOwner())

	_, err := c.BroadcastTx(context.Background(), "bank.MsgSend", nil)
	if err == nil {
		t.Fatal("expected error for unsupported msg type")
	}
	if !strings.Contains(err.Error(), `unsupported workflow tx message "bank.MsgSend"`) {
		t.Errorf("error = %v, want unsupported workflow tx message", err)
	}
}

func TestBroadcastTxCloseDeployment(t *testing.T) {
	tx := &fakeTxClient{resp: &sdk.TxResponse{
		TxHash: "ABC123",
		Height: 42,
		Code:   0,
	}}
	c := newFakeChainClient(tx, testOwner())

	result, err := c.BroadcastTx(context.Background(), msgCloseDeployment, map[string]string{"dseq": "12345"})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	if result.TxHash != "ABC123" || result.Height != 42 || result.Code != 0 {
		t.Errorf("result = %+v", result)
	}
	if len(tx.msgs) != 1 {
		t.Fatalf("broadcast msgs = %d, want 1", len(tx.msgs))
	}

	var data map[string]string
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("unmarshal result data: %v", err)
	}
	if data["dseq"] != "12345" {
		t.Errorf("data dseq = %q, want \"12345\"", data["dseq"])
	}
	if data["owner"] != testOwner().String() {
		t.Errorf("data owner = %q, want %q", data["owner"], testOwner().String())
	}
}

func TestBroadcastTxCreateLease(t *testing.T) {
	tx := &fakeTxClient{resp: &sdk.TxResponse{TxHash: "LEASE1", Height: 10}}
	c := newFakeChainClient(tx, testOwner())
	provider := testProviderAddr().String()

	result, err := c.BroadcastTx(context.Background(), msgCreateLease, map[string]string{
		"dseq":     "9000001",
		"provider": provider,
	})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	var data map[string]string
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("unmarshal result data: %v", err)
	}
	if data["dseq"] != "9000001" || data["gseq"] != "1" || data["oseq"] != "1" || data["provider"] != provider {
		t.Errorf("data = %v", data)
	}
}

func TestBroadcastTxNonZeroCode(t *testing.T) {
	tx := &fakeTxClient{resp: &sdk.TxResponse{
		TxHash: "DEAD",
		Code:   5,
		RawLog: "insufficient funds",
	}}
	c := newFakeChainClient(tx, testOwner())

	_, err := c.BroadcastTx(context.Background(), msgCloseDeployment, map[string]string{"dseq": "1"})
	if err == nil {
		t.Fatal("expected error for non-zero tx code")
	}
	if !strings.Contains(err.Error(), "code 5") || !strings.Contains(err.Error(), "insufficient funds") {
		t.Errorf("error = %v, want code and raw log", err)
	}
}

func TestBroadcastTxUnexpectedResponseType(t *testing.T) {
	c := newFakeChainClient(&fakeTxClient{resp: "not-a-tx-response"}, testOwner())

	if _, err := c.BroadcastTx(context.Background(), msgCloseDeployment, map[string]string{"dseq": "1"}); err == nil {
		t.Fatal("expected error for unexpected broadcast response type")
	}
}

// TestTxResultDataDSeqExtraction verifies the deploy workflow's output
// pipeline end to end: TxResult.Data -> workflow.ExtractOutputs -> the
// {{ (index .Steps "create-deployment").dseq }} template used by
// builtin/deploy.yaml. dseq is emitted as a JSON string so that large values
// (block heights in the millions) never render in scientific notation.
func TestTxResultDataDSeqExtraction(t *testing.T) {
	tx := &fakeTxClient{resp: &sdk.TxResponse{TxHash: "XYZ", Height: 5335560}}
	c := newFakeChainClient(tx, testOwner())

	// Close reuses the same data plumbing as create (dseq + owner) without
	// needing node/deposit queries.
	result, err := c.BroadcastTx(context.Background(), msgCloseDeployment, map[string]string{"dseq": "5335559"})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	state := workflow.NewRunState("wf-1", "deploy", testOwner().String(), nil)

	outputs, err := workflow.ExtractOutputs(nil, result.Data, state)
	if err != nil {
		t.Fatalf("ExtractOutputs: %v", err)
	}

	state.SetStepResult("create-deployment", &workflow.StepResult{
		Name:      "create-deployment",
		Status:    "success",
		Output:    outputs,
		RawResult: result.Data,
		TxHash:    result.TxHash,
		Height:    result.Height,
	})

	rendered, err := workflow.ResolveTemplate(`{{ (index .Steps "create-deployment").dseq }}`, state)
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}

	if rendered != "5335559" {
		t.Errorf("rendered dseq = %q, want \"5335559\"", rendered)
	}
}

// --- Query tests ---

func TestQueryUnsupportedPath(t *testing.T) {
	c := newFakeChainClient(&fakeTxClient{}, testOwner())

	_, err := c.Query(context.Background(), "bank.balances", nil)
	if err == nil {
		t.Fatal("expected error for unsupported query path")
	}
	if !strings.Contains(err.Error(), `unsupported workflow query path "bank.balances"`) {
		t.Errorf("error = %v, want unsupported workflow query path", err)
	}
}

func TestQueryBadNumericParam(t *testing.T) {
	c := newFakeChainClient(&fakeTxClient{}, testOwner())

	if _, err := c.Query(context.Background(), queryMarketBids, map[string]string{"dseq": "abc"}); err == nil {
		t.Fatal("expected error for non-numeric dseq filter")
	}
}
