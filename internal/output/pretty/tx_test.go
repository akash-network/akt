package pretty

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/charmbracelet/x/exp/golden"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	dtypes "pkg.akt.dev/go/node/types/deposit/v1"
)

func TestPrintTxResultWritesEncodedTransactionBytesAsJSONObject(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String(flagdefs.FlagOutput, "json", "")
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
	cmd.Flags().String(flagdefs.FlagOutput, "json", "")
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

func TestTransactionOutputModesPropagateCommandWriterFailures(t *testing.T) {
	protoCodec := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	legacyAmino := codec.NewLegacyAmino()
	wantErr := errors.New("command stdout failed")

	operations := []struct {
		name   string
		format string
		run    func(*cobra.Command, sdkclient.Context) error
	}{
		{
			name:   "generate-only JSON",
			format: cflags.OutputJSON,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, []byte(`{"body":{"memo":"generated"}}`))
			},
		},
		{
			name:   "generate-only YAML",
			format: cflags.OutputYAML,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, []byte(`{"body":{"memo":"generated"}}`))
			},
		},
		{
			name:   "simulation pretty",
			format: cflags.OutputPretty,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, &txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 42}})
			},
		},
		{
			name:   "simulation JSON",
			format: cflags.OutputJSON,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, &txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 42}})
			},
		},
		{
			name:   "simulation YAML",
			format: cflags.OutputYAML,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, &txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 42}})
			},
		},
		{
			name:   "structured JSON",
			format: cflags.OutputJSON,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, &sdk.TxResponse{TxHash: testTxHash})
			},
		},
		{
			name:   "structured YAML",
			format: cflags.OutputYAML,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, &sdk.TxResponse{TxHash: testTxHash})
			},
		},
		{
			name:   "multi-result pretty",
			format: cflags.OutputPretty,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResults(cmd, cctx, []interface{}{
					&sdk.TxResponse{TxHash: "FIRST"},
					&sdk.TxResponse{TxHash: "SECOND"},
				})
			},
		},
		{
			name:   "multi-result JSON",
			format: cflags.OutputJSON,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResults(cmd, cctx, []interface{}{
					[]byte(`{"body":{"memo":"first"}}`),
					[]byte(`{"body":{"memo":"second"}}`),
				})
			},
		},
		{
			name:   "multi-result YAML",
			format: cflags.OutputYAML,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResults(cmd, cctx, []interface{}{
					[]byte(`{"body":{"memo":"first"}}`),
					[]byte(`{"body":{"memo":"second"}}`),
				})
			},
		},
		{
			name:   "proto fallback pretty",
			format: cflags.OutputPretty,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, &banktypes.QueryParamsResponse{})
			},
		},
		{
			name:   "proto fallback JSON",
			format: cflags.OutputJSON,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, &banktypes.QueryParamsResponse{})
			},
		},
		{
			name:   "proto fallback YAML",
			format: cflags.OutputYAML,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, &banktypes.QueryParamsResponse{})
			},
		},
		{
			name:   "legacy fallback pretty",
			format: cflags.OutputPretty,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, map[string]string{"status": "ready"})
			},
		},
		{
			name:   "legacy fallback JSON",
			format: cflags.OutputJSON,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, map[string]string{"status": "ready"})
			},
		},
		{
			name:   "legacy fallback YAML",
			format: cflags.OutputYAML,
			run: func(cmd *cobra.Command, cctx sdkclient.Context) error {
				return PrintTxResult(cmd, cctx, map[string]string{"status": "ready"})
			},
		},
	}
	failures := []struct {
		name string
		w    io.Writer
		want error
	}{
		{name: "hard error", w: prettyBoundaryWriter{err: wantErr}, want: wantErr},
		{name: "short write", w: prettyBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, failure := range failures {
				t.Run(failure.name, func(t *testing.T) {
					cmd := &cobra.Command{}
					cmd.Flags().String(flagdefs.FlagOutput, operation.format, "")
					cmd.SetOut(failure.w)

					var wrongDestination bytes.Buffer
					cctx := sdkclient.Context{
						Codec:       protoCodec,
						LegacyAmino: legacyAmino,
					}.WithOutput(&wrongDestination)
					err := operation.run(cmd, cctx)
					require.ErrorIs(t, err, failure.want)
					require.Empty(t, wrongDestination.String(), "client context output must be replaced by command output")
				})
			}
		})
	}
}

func TestPrintTxResultPrettyPropagatesWriterFailures(t *testing.T) {
	ensureFormattersRegistered()

	wantErr := errors.New("stdout failed")

	t.Run("summary destination error", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputPretty, "")
		cmd.SetOut(txResultErrorWriter{err: wantErr})

		err := PrintTxResult(cmd, sdkclient.Context{}, &sdk.TxResponse{TxHash: testTxHash})
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("summary short write", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputPretty, "")
		cmd.SetOut(txResultShortWriter{})

		err := PrintTxResult(cmd, sdkclient.Context{}, &sdk.TxResponse{TxHash: testTxHash})
		require.ErrorIs(t, err, io.ErrShortWrite)
	})

	t.Run("message header destination error", func(t *testing.T) {
		cctx, response := txResponseWithMessages(t, &banktypes.MsgSend{})
		writer := &txResultMatchingErrorWriter{match: "Send", err: wantErr}
		cmd := &cobra.Command{}
		cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputPretty, "")
		cmd.SetOut(writer)

		err := PrintTxResult(cmd, cctx, response)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("fallback destination error", func(t *testing.T) {
		msg := &banktypes.MsgUpdateParams{Authority: "fallback-writer-probe"}
		cctx, response := txResponseWithMessages(t, msg)
		writer := &txResultMatchingErrorWriter{match: "fallback-writer-probe", err: wantErr}
		cmd := &cobra.Command{}
		cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputPretty, "")
		cmd.SetOut(writer)

		err := PrintTxResult(cmd, cctx, response)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestPrintTxResultPrettyPropagatesLateWriterFailures(t *testing.T) {
	ensureFormattersRegistered()
	wantErr := errors.New("command stdout failed")

	operations := []struct {
		name  string
		match string
		msgs  []sdk.Msg
	}{
		{name: "registered message", match: "Send", msgs: []sdk.Msg{&banktypes.MsgSend{}}},
		{
			name:  "fallback message",
			match: "fallback-authority",
			msgs:  []sdk.Msg{&banktypes.MsgUpdateParams{Authority: "fallback-authority"}},
		},
		{
			name:  "second message",
			match: "second-fallback-authority",
			msgs: []sdk.Msg{
				&banktypes.MsgSend{},
				&banktypes.MsgUpdateParams{Authority: "second-fallback-authority"},
			},
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, failure := range []struct {
				name  string
				err   error
				short bool
				want  error
			}{
				{name: "hard error", err: wantErr, want: wantErr},
				{name: "short write", short: true, want: io.ErrShortWrite},
			} {
				t.Run(failure.name, func(t *testing.T) {
					cctx, response := txResponseWithMessages(t, operation.msgs...)
					cmd := &cobra.Command{}
					cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputPretty, "")
					cmd.SetOut(&txResultMatchingErrorWriter{
						match: operation.match,
						err:   failure.err,
						short: failure.short,
					})

					require.ErrorIs(t, PrintTxResult(cmd, cctx, response), failure.want)
				})
			}
		})
	}
}

func TestPrintTxResultPrettyPropagatesNestedFormatterError(t *testing.T) {
	ensureFormattersRegistered()

	innerMsg := &banktypes.MsgSend{}
	previous, ok := LookupTx(innerMsg)
	require.True(t, ok)

	wantErr := errors.New("nested formatter failed")
	RegisterTx(innerMsg, TxPrettyFormatterFunc{
		TitleStr: "Send",
		FormatFn: func(io.Writer, *cobra.Command, sdkclient.Context, sdk.Msg, *sdk.TxResponse, int) error {
			return wantErr
		},
	})
	t.Cleanup(func() { RegisterTx(innerMsg, previous) })

	inner, err := codectypes.NewAnyWithValue(innerMsg)
	require.NoError(t, err)
	cctx, response := txResponseWithMessages(t, &authz.MsgExec{Msgs: []*codectypes.Any{inner}})

	cmd := &cobra.Command{}
	cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputPretty, "")
	cmd.SetOut(io.Discard)

	require.ErrorIs(t, PrintTxResult(cmd, cctx, response), wantErr)

	// The multi-message loop must stop and return the formatter failure too;
	// otherwise later message output could make a partially rendered receipt
	// look successful.
	cctx, response = txResponseWithMessages(t, innerMsg, &banktypes.MsgUpdateParams{})
	require.ErrorIs(t, PrintTxResult(cmd, cctx, response), wantErr)
}

func TestPrintTxResultPrettyRendersRegisteredAndFallbackMessages(t *testing.T) {
	ensureFormattersRegistered()

	cctx, response := txResponseWithMessages(t,
		&banktypes.MsgSend{},
		&banktypes.MsgUpdateParams{Authority: "fallback-authority"},
	)
	response.Code = 5
	response.RawLog = "transaction failed"

	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputPretty, "")
	cmd.SetOut(&output)

	require.NoError(t, PrintTxResult(cmd, cctx, response))
	require.Contains(t, output.String(), "Message 1: Send")
	require.Contains(t, output.String(), "(failed)")
	require.Contains(t, output.String(), "Message 2")
	require.Contains(t, output.String(), "fallback-authority")
}

type txResultErrorWriter struct {
	err error
}

func (w txResultErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type txResultShortWriter struct{}

func (txResultShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	return len(p) - 1, nil
}

type txResultMatchingErrorWriter struct {
	match string
	err   error
	short bool
}

func (w *txResultMatchingErrorWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte(w.match)) {
		if w.err != nil {
			return 0, w.err
		}
		if w.short && len(p) > 0 {
			return len(p) - 1, nil
		}
	}

	return len(p), nil
}

func txResponseWithMessages(t *testing.T, msgs ...sdk.Msg) (sdkclient.Context, *sdk.TxResponse) {
	t.Helper()

	txBody, err := codectypes.NewAnyWithValue(&txResultTestTx{msgs: msgs})
	require.NoError(t, err)

	cctx := sdkclient.Context{Codec: codec.NewProtoCodec(codectypes.NewInterfaceRegistry())}
	return cctx, &sdk.TxResponse{TxHash: testTxHash, Height: 1, Tx: txBody}
}

type txResultTestTx struct {
	msgs []sdk.Msg
}

func (*txResultTestTx) Reset() {}

func (*txResultTestTx) String() string { return "test transaction" }

func (*txResultTestTx) ProtoMessage() {}

func (tx *txResultTestTx) GetMsgs() []sdk.Msg { return tx.msgs }

func (*txResultTestTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

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
			cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputJSON, "")
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
	cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputYAML, "")
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
		cmd.Flags().String(flagdefs.FlagOutput, format, "")
		cmd.Flags().Float64(flagdefs.FlagGasAdjustment, 1.5, "")
		cmd.Flags().String(flagdefs.FlagFees, "", "")
		cmd.Flags().String(flagdefs.FlagGasPrices, "0.0025uakt", "")
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
	cmd.Flags().Float64(flagdefs.FlagGasAdjustment, 1.5, "")
	cmd.Flags().String(flagdefs.FlagFees, "7500uakt", "")
	cmd.Flags().String(flagdefs.FlagGasPrices, "0.0025uakt", "")

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
