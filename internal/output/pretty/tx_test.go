package pretty

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/charmbracelet/x/exp/golden"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
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

const testTxHash = "9F3C0A2E7B1D4F6089AC5321EE7740B8D2C619F5A48037BC12ED5A9F60B3417D"

// confirmedTxBody packs a transaction body carrying a fee, the way a
// --broadcast-mode block response does.
func confirmedTxBody(t *testing.T) *codectypes.Any {
	t.Helper()

	body, err := codectypes.NewAnyWithValue(&txtypes.Tx{
		Body: &txtypes.TxBody{},
		AuthInfo: &txtypes.AuthInfo{
			Fee: &txtypes.Fee{
				Amount:   sdk.NewCoins(sdk.NewInt64Coin("uakt", 5000)),
				GasLimit: 200000,
			},
		},
	})
	require.NoError(t, err)

	return body
}

func senderEvents(addr string) []abci.Event {
	return []abci.Event{{
		Type:       "message",
		Attributes: []abci.EventAttribute{{Key: "sender", Value: addr}},
	}}
}

// TestRenderTxSummaryConfirmed covers a transaction that reached a block: every
// field has a real value.
func TestRenderTxSummaryConfirmed(t *testing.T) {
	resp := &sdk.TxResponse{
		TxHash:    testTxHash,
		Height:    23_154_007,
		GasUsed:   118_432,
		GasWanted: 200_000,
		Code:      0,
		Tx:        confirmedTxBody(t),
		Events:    senderEvents("akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu"),
	}

	var buf bytes.Buffer
	renderTxSummaryWithCodec(&buf, sdkclient.Context{}, resp)

	golden.RequireEqual(t, buf.Bytes())
}

// TestRenderTxSummaryPending covers the default --broadcast-mode sync response:
// the transaction passed CheckTx and is sitting in the mempool, so height, gas
// and fee are unknown rather than zero, and the status is not "success".
func TestRenderTxSummaryPending(t *testing.T) {
	resp := &sdk.TxResponse{TxHash: testTxHash, RawLog: "[]"}

	var buf bytes.Buffer
	renderTxSummaryWithCodec(&buf, sdkclient.Context{}, resp)

	out := buf.String()
	require.Contains(t, out, "pending")
	require.NotContains(t, out, "success")
	require.Contains(t, out, "Fee:", "the fee row must never be silently omitted")
	require.Contains(t, out, "akt query tx "+testTxHash)
	require.Contains(t, out, "--broadcast-mode block")

	golden.RequireEqual(t, buf.Bytes())
}

// TestRenderTxSummaryFailed covers a CheckTx rejection: code is non-zero, so
// the result is failed rather than pending even though there is no height.
func TestRenderTxSummaryFailed(t *testing.T) {
	resp := &sdk.TxResponse{
		TxHash:    testTxHash,
		Code:      11,
		Codespace: "sdk",
		RawLog:    "out of gas in location: WritePerByte; gasWanted: 200000, gasUsed: 201045",
	}

	var buf bytes.Buffer
	renderTxSummaryWithCodec(&buf, sdkclient.Context{}, resp)

	require.NotContains(t, buf.String(), "pending")

	golden.RequireEqual(t, buf.Bytes())
}

func TestTxStatusClassification(t *testing.T) {
	cases := map[string]struct {
		resp *sdk.TxResponse
		want string
	}{
		"Confirmed": {resp: &sdk.TxResponse{Height: 10}, want: TxStatusConfirmed},
		"Pending":   {resp: &sdk.TxResponse{}, want: TxStatusPending},
		"Failed":    {resp: &sdk.TxResponse{Code: 5}, want: TxStatusFailed},
		// A non-zero code always outranks a height.
		"FailedInBlock": {resp: &sdk.TxResponse{Height: 10, Code: 5}, want: TxStatusFailed},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, TxStatus(tc.resp))
		})
	}
}

// TestPrintTxResultJSONHeightPresence is the regression guard for the zero
// height leaking into machine-readable output: an unconfirmed transaction must
// carry no height key at all, never "height": 0.
func TestPrintTxResultJSONHeightPresence(t *testing.T) {
	cases := map[string]struct {
		resp        *sdk.TxResponse
		wantStatus  string
		wantHeight  bool
		wantHeightN json.Number
	}{
		"Pending": {
			resp:       &sdk.TxResponse{TxHash: testTxHash},
			wantStatus: TxStatusPending,
			wantHeight: false,
		},
		"Confirmed": {
			resp:        &sdk.TxResponse{TxHash: testTxHash, Height: 23_154_007, GasUsed: 118_432, GasWanted: 200_000},
			wantStatus:  TxStatusConfirmed,
			wantHeight:  true,
			wantHeightN: "23154007",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String(cflags.FlagOutput, cflags.OutputJSON, "")
			var out bytes.Buffer
			cmd.SetOut(&out)

			require.NoError(t, PrintTxResult(cmd, sdkclient.Context{}, tc.resp))

			decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
			decoder.UseNumber()
			var decoded map[string]any
			require.NoError(t, decoder.Decode(&decoded))

			require.Equal(t, tc.wantStatus, decoded["status"])
			require.Equal(t, tc.wantHeight, decoded["confirmed"])

			height, ok := decoded["height"]
			require.Equal(t, tc.wantHeight, ok, "height key presence")
			if tc.wantHeight {
				require.Equal(t, tc.wantHeightN, height)
			} else {
				require.NotContains(t, out.String(), "height")
				require.NotContains(t, out.String(), "gas_used")
				require.NotContains(t, out.String(), "gas_wanted")
			}
		})
	}
}

func TestPrintTxResultYAMLOmitsUnconfirmedHeight(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String(cflags.FlagOutput, cflags.OutputYAML, "")
	var out bytes.Buffer
	cmd.SetOut(&out)

	require.NoError(t, PrintTxResult(cmd, sdkclient.Context{}, &sdk.TxResponse{TxHash: testTxHash}))

	require.Contains(t, out.String(), "status: pending")
	require.NotContains(t, out.String(), "height")
}

// TestPrintTxResultRendersSimulation covers --dry-run. The node echoes back the
// placeholder gas limit the CLI substitutes for --gas, so gas_wanted must never
// reach the user; what they asked for is the adjusted estimate and its fee.
func TestPrintTxResultRendersSimulation(t *testing.T) {
	sim := &txtypes.SimulateResponse{
		GasInfo: &sdk.GasInfo{GasWanted: 0, GasUsed: 118_432},
	}

	newCmd := func(format string) (*cobra.Command, *bytes.Buffer) {
		cmd := &cobra.Command{}
		cmd.Flags().String(cflags.FlagOutput, format, "")
		cmd.Flags().Float64(cflags.FlagGasAdjustment, 1.5, "")
		cmd.Flags().String(cflags.FlagFees, "", "")
		cmd.Flags().String(cflags.FlagGasPrices, "0.0025uakt", "")
		out := &bytes.Buffer{}
		cmd.SetOut(out)

		return cmd, out
	}

	t.Run("Pretty", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		cmd, out := newCmd("pretty")
		require.NoError(t, PrintTxResult(cmd, sdkclient.Context{}, sim))

		require.NotContains(t, out.String(), "gas_wanted")
		require.NotContains(t, out.String(), "Gas Wanted")

		golden.RequireEqual(t, out.Bytes())
	})

	t.Run("JSON", func(t *testing.T) {
		cmd, out := newCmd(cflags.OutputJSON)
		require.NoError(t, PrintTxResult(cmd, sdkclient.Context{}, sim))

		var decoded map[string]any
		decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&decoded))

		require.Equal(t, true, decoded["simulated"])
		require.Equal(t, json.Number("118432"), decoded["gas_used"])
		// 1.5 * 118432
		require.Equal(t, json.Number("177648"), decoded["gas_estimate"])
		require.NotContains(t, decoded, "gas_wanted")

		// ceil(0.0025 * 177648) = 445
		fee, ok := decoded["estimated_fee"].([]any)
		require.True(t, ok, "estimated_fee: %#v", decoded["estimated_fee"])
		require.Len(t, fee, 1)
		require.Equal(t, map[string]any{"denom": "uakt", "amount": "445"}, fee[0])
	})
}

func TestSimulationEstimatedFeePrefersExplicitFees(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Float64(cflags.FlagGasAdjustment, 1.5, "")
	cmd.Flags().String(cflags.FlagFees, "7500uakt", "")
	cmd.Flags().String(cflags.FlagGasPrices, "0.0025uakt", "")

	result := NewSimulationResult(cmd, &txtypes.SimulateResponse{
		GasInfo: &sdk.GasInfo{GasUsed: 118_432},
	})

	require.Equal(t, uint64(177_648), result.GasEstimate)
	require.Equal(t, "7500uakt", result.EstimatedFee.String())
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
