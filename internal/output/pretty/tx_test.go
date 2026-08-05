package pretty

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	dtypes "pkg.akt.dev/go/node/types/deposit/v1"
)

func TestPrintTxResultWritesEncodedTransactionBytesAsJSONObject(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "json", "")
	var out bytes.Buffer
	cmd.SetOut(&out)

	payload := []byte(`{"body":{"messages":[]},"auth_info":{"fee":{"amount":[]}}}`)
	if err := PrintTxResult(cmd, sdkclient.Context{}, payload); err != nil {
		t.Fatalf("PrintTxResult: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode generated transaction: %v\n%s", err, out.String())
	}
	if _, ok := decoded["body"]; !ok {
		t.Fatalf("top-level JSON is not a transaction object: %#v", decoded)
	}
}

func TestPrintTxResultsWritesOneJSONArray(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "json", "")
	var out bytes.Buffer
	cmd.SetOut(&out)

	responses := []interface{}{
		[]byte(`{"body":{"memo":"first"}}`),
		[]byte(`{"body":{"memo":"second"}}`),
	}
	if err := PrintTxResults(cmd, sdkclient.Context{}, responses); err != nil {
		t.Fatalf("PrintTxResults: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode transaction array: %v\n%s", err, out.String())
	}
	if len(decoded) != 2 {
		t.Fatalf("transaction count = %d, want 2", len(decoded))
	}
}

var registerOnce sync.Once

func ensureFormattersRegistered() {
	registerOnce.Do(RegisterAllTxFormatters)
}

func TestTxFmtBankSend(t *testing.T) {
	ensureFormattersRegistered()

	msg := &banktypes.MsgSend{
		FromAddress: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu",
		ToAddress:   "akash1z3a2m5v6smf7m9gk3dxtrwep9xyeqahxqy6045",
		Amount:      sdk.NewCoins(sdk.NewInt64Coin("uakt", 5000000)),
	}

	f, ok := LookupTx(msg)
	require.True(t, ok, "formatter should be registered for MsgSend")

	var buf bytes.Buffer
	err := f.FormatTx(&buf, nil, sdkclient.Context{}, msg, &sdk.TxResponse{}, 0)
	require.NoError(t, err)

	golden.RequireEqual(t, buf.Bytes())
}

func TestTxFmtDeploymentCreate(t *testing.T) {
	ensureFormattersRegistered()

	msg := &dv1beta.MsgCreateDeployment{
		ID: dv1.DeploymentID{
			Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu",
			DSeq:  12345678,
		},
		Deposit: dtypes.Deposit{
			Amount: sdk.NewInt64Coin("uakt", 50000000),
		},
		Groups: nil,
	}

	f, ok := LookupTx(msg)
	require.True(t, ok, "formatter should be registered for MsgCreateDeployment")

	var buf bytes.Buffer
	err := f.FormatTx(&buf, nil, sdkclient.Context{}, msg, &sdk.TxResponse{}, 0)
	require.NoError(t, err)

	golden.RequireEqual(t, buf.Bytes())
}

// TestTxFmtDeploymentUpdate pins the caveat: MsgUpdateDeployment changes only
// the chain record, so the result must say the providers have not seen it yet.
func TestTxFmtDeploymentUpdate(t *testing.T) {
	ensureFormattersRegistered()

	msg := &dv1beta.MsgUpdateDeployment{
		ID: dv1.DeploymentID{
			Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu",
			DSeq:  12345678,
		},
	}

	f, ok := LookupTx(msg)
	require.True(t, ok, "formatter should be registered for MsgUpdateDeployment")

	var buf bytes.Buffer
	err := f.FormatTx(&buf, nil, sdkclient.Context{}, msg, &sdk.TxResponse{}, 0)
	require.NoError(t, err)

	golden.RequireEqual(t, buf.Bytes())
}

func TestTxFmtDelegate(t *testing.T) {
	ensureFormattersRegistered()

	msg := &stakingtypes.MsgDelegate{
		DelegatorAddress: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu",
		ValidatorAddress: "akashvaloper1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5axudam",
		Amount:           sdk.NewInt64Coin("uakt", 10000000),
	}

	f, ok := LookupTx(msg)
	require.True(t, ok, "formatter should be registered for MsgDelegate")

	var buf bytes.Buffer
	err := f.FormatTx(&buf, nil, sdkclient.Context{}, msg, &sdk.TxResponse{}, 0)
	require.NoError(t, err)

	golden.RequireEqual(t, buf.Bytes())
}

func TestTxFmtVote(t *testing.T) {
	ensureFormattersRegistered()

	msg := &govv1.MsgVote{
		ProposalId: 42,
		Voter:      "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu",
		Option:     govv1.OptionYes,
	}

	f, ok := LookupTx(msg)
	require.True(t, ok, "formatter should be registered for MsgVote")

	var buf bytes.Buffer
	err := f.FormatTx(&buf, nil, sdkclient.Context{}, msg, &sdk.TxResponse{}, 0)
	require.NoError(t, err)

	golden.RequireEqual(t, buf.Bytes())
}

func TestTxFmtWasmExecute(t *testing.T) {
	ensureFormattersRegistered()

	msg := &wasmtypes.MsgExecuteContract{
		Sender:   "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu",
		Contract: "akash14hj2tavq8fpesdwxxcu44rty3hh90vhujrvcmstl4zr3txmfvw9s4se2wp",
		Msg:      []byte(`{"swap":{"offer_asset":{"amount":"1000000"}}}`),
		Funds:    sdk.NewCoins(sdk.NewInt64Coin("uakt", 1000000)),
	}

	f, ok := LookupTx(msg)
	require.True(t, ok, "formatter should be registered for MsgExecuteContract")

	var buf bytes.Buffer
	err := f.FormatTx(&buf, nil, sdkclient.Context{}, msg, &sdk.TxResponse{}, 0)
	require.NoError(t, err)

	golden.RequireEqual(t, buf.Bytes())
}

func TestTxFmtMultipleFormatters(t *testing.T) {
	ensureFormattersRegistered()

	tests := map[string]struct {
		msg           sdk.Msg
		expectedTitle string
	}{
		"BankSend": {
			msg:           &banktypes.MsgSend{},
			expectedTitle: "Send",
		},
		"DeploymentCreate": {
			msg:           &dv1beta.MsgCreateDeployment{},
			expectedTitle: "Deployment Created",
		},
		"Delegate": {
			msg:           &stakingtypes.MsgDelegate{},
			expectedTitle: "Delegate",
		},
		"Vote": {
			msg:           &govv1.MsgVote{},
			expectedTitle: "Vote",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f, ok := LookupTx(tc.msg)
			require.True(t, ok, "formatter should be registered for %T", tc.msg)
			require.Equal(t, tc.expectedTitle, f.Title())
		})
	}
}
