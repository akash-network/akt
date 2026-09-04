package pretty

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"encoding/json"
	"fmt"
	"io"

	abci "github.com/cometbft/cometbft/abci/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	clioutput "pkg.akt.dev/akt/internal/output"
)

// Confirmation states of a broadcast result (SPEC §10.11.1).
//
// akt broadcasts with --broadcast-mode sync by default, so the usual response
// is a CheckTx result: the transaction sits in the mempool and its height, gas
// and body are simply not known yet. They are not zero, and they are not blank.
const (
	TxStatusConfirmed = "confirmed"
	TxStatusPending   = "pending"
	TxStatusFailed    = "failed"
)

// PrintTxResult is the main dispatch function for transaction output (SPEC §10.11).
//
// Dispatch is by concrete response type first and by --output (-o) second, because
// chain-sdk's BroadcastMsgs returns interface{} and the concrete type tells us what
// actually happened:
//   - []byte: a --generate-only transaction body.
//   - *txtypes.SimulateResponse: a --dry-run simulation (SPEC §10.11.7).
//   - *sdk.TxResponse: a broadcast result; "pretty" renders the two-section
//     layout, "json"/"yaml" emit the structured document of SPEC §10.11.6.
func PrintTxResult(cmd *cobra.Command, cctx sdkclient.Context, resp interface{}) error {
	output, _ := cmd.Flags().GetString(flagdefs.FlagOutput)
	checked := clioutput.NewCheckedWriter(cmd.OutOrStdout())
	cctx = cctx.WithOutput(checked)
	if payload, ok := resp.([]byte); ok {
		return checked.Complete(printEncodedTransaction(checked, output, payload))
	}

	if sim, ok := resp.(*txtypes.SimulateResponse); ok {
		if output != cflags.OutputJSON && output != cflags.OutputYAML {
			checked = clioutput.NewCheckedTerminalWriter(cmd.OutOrStdout())
		}
		return checked.Complete(printSimulationResult(checked, cmd, output, sim))
	}

	txResp, isTxResponse := resp.(*sdk.TxResponse)

	// For json/yaml, emit the structured result rather than the raw proto: the
	// SDK printer marshals with EmitDefaults, which turns "not confirmed yet"
	// into a literal "height":"0" that a machine consumer cannot tell apart
	// from a real block height (SPEC §10.11.6).
	switch output {
	case cflags.OutputJSON:
		if isTxResponse {
			return checked.Complete(printTxResultStructured(checked, cctx, output, txResp))
		}

		if pm, ok := resp.(proto.Message); ok {
			return checked.Complete(cctx.WithOutputFormat("json").PrintProto(pm))
		}

		//nolint:staticcheck // SA1019: PrintObjectLegacy is the only client-context
		// printer that accepts a non-proto (amino) value, which is exactly what
		// this fallback branch has. See PrintQueryResultAny in printer.go.
		return checked.Complete(cctx.PrintObjectLegacy(resp))
	case cflags.OutputYAML:
		if isTxResponse {
			return checked.Complete(printTxResultStructured(checked, cctx, output, txResp))
		}

		if pm, ok := resp.(proto.Message); ok {
			return checked.Complete(cctx.WithOutputFormat("text").PrintProto(pm))
		}

		//nolint:staticcheck // SA1019: PrintObjectLegacy is the only client-context
		// printer that accepts a non-proto (amino) value, which is exactly what
		// this fallback branch has. See PrintQueryResultAny in printer.go.
		return checked.Complete(cctx.PrintObjectLegacy(resp))
	}

	// Pretty mode — render the two-section layout.
	if !isTxResponse {
		// Not a TxResponse — fall back to JSON.
		if pm, ok := resp.(proto.Message); ok {
			return checked.Complete(cctx.WithOutputFormat("json").PrintProto(pm))
		}

		//nolint:staticcheck // SA1019: PrintObjectLegacy is the only client-context
		// printer that accepts a non-proto (amino) value, which is exactly what
		// this fallback branch has. See PrintQueryResultAny in printer.go.
		return checked.Complete(cctx.PrintObjectLegacy(resp))
	}

	checked = clioutput.NewCheckedTerminalWriter(cmd.OutOrStdout())
	cctx = cctx.WithOutput(checked)
	w := io.Writer(checked)

	// Section 1: Common transaction summary.
	renderTxSummaryWithCodec(w, cctx, txResp)

	// Section 2: Message-specific detail.
	msgs := decodeTxMsgs(cctx, txResp)
	if len(msgs) == 0 {
		return checked.Err()
	}

	Newline(w)

	var renderErr error
	if len(msgs) == 1 {
		renderErr = renderSingleMessage(w, cmd, cctx, msgs[0], txResp, 0)
	} else {
		for i, msg := range msgs {
			if i > 0 {
				Newline(w)
			}

			if err := renderMultiMessage(w, cmd, cctx, msg, txResp, i, len(msgs)); err != nil {
				renderErr = err
				break
			}
		}
	}

	return checked.Complete(renderErr)
}

// PrintTxResults preserves one structured document when a command intentionally
// splits one message set into multiple transactions.
func PrintTxResults(cmd *cobra.Command, cctx sdkclient.Context, responses []interface{}) error {
	if len(responses) == 0 {
		return fmt.Errorf("no transaction results to print")
	}
	if len(responses) == 1 {
		return PrintTxResult(cmd, cctx, responses[0])
	}

	format, _ := cmd.Flags().GetString(flagdefs.FlagOutput)
	if format != cflags.OutputJSON && format != cflags.OutputYAML {
		for _, response := range responses {
			if err := PrintTxResult(cmd, cctx, response); err != nil {
				return err
			}
		}
		return nil
	}

	values := make([]any, 0, len(responses))
	for _, response := range responses {
		value, err := txResultJSONValue(cmd, cctx, response)
		if err != nil {
			return err
		}
		values = append(values, value)
	}

	outputFormat := clioutput.FormatJSON
	if format == cflags.OutputYAML {
		outputFormat = clioutput.FormatYAML
	}
	return clioutput.FprintJSONSemantics(cmd.OutOrStdout(), outputFormat, values)
}

func txResultJSONValue(cmd *cobra.Command, cctx sdkclient.Context, response interface{}) (any, error) {
	// Broadcast results and simulations get the same honest treatment they get
	// when printed alone (SPEC §10.11.6, §10.11.7).
	switch response := response.(type) {
	case *sdk.TxResponse:
		return NewTxResultDocument(cctx, response), nil
	case *txtypes.SimulateResponse:
		return NewSimulationResult(cmd, response), nil
	}

	var payload []byte
	var err error
	switch response := response.(type) {
	case []byte:
		payload = response
	case proto.Message:
		if cctx.Codec == nil {
			return nil, fmt.Errorf("cannot encode transaction result without a codec")
		}
		payload, err = cctx.Codec.MarshalJSON(response)
	default:
		payload, err = json.Marshal(response)
	}
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// TxResultDocument is the machine-readable form of a broadcast result
// (SPEC §10.11.6). Every field whose value is unknown because the transaction
// has not been confirmed is absent from the encoding rather than zero, so an
// automated consumer can never mistake "not yet in a block" for height 0 or
// for a genuine zero gas reading.
type TxResultDocument struct {
	TxHash    string `json:"txhash"`
	Status    string `json:"status"`
	Confirmed bool   `json:"confirmed"`
	Code      uint32 `json:"code"`
	Codespace string `json:"codespace,omitempty"`

	Height    *int64 `json:"height,omitempty"`
	GasUsed   *int64 `json:"gas_used,omitempty"`
	GasWanted *int64 `json:"gas_wanted,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`

	Data   string `json:"data,omitempty"`
	Info   string `json:"info,omitempty"`
	RawLog string `json:"raw_log,omitempty"`

	Logs   json.RawMessage `json:"logs,omitempty"`
	Events json.RawMessage `json:"events,omitempty"`
	Tx     json.RawMessage `json:"tx,omitempty"`
}

// NewTxResultDocument builds the structured result for a broadcast response.
func NewTxResultDocument(cctx sdkclient.Context, resp *sdk.TxResponse) *TxResultDocument {
	if resp == nil {
		return nil
	}

	doc := &TxResultDocument{
		TxHash:    resp.TxHash,
		Status:    TxStatus(resp),
		Confirmed: resp.Height > 0,
		Code:      resp.Code,
		Codespace: resp.Codespace,
		Timestamp: resp.Timestamp,
		Data:      resp.Data,
		Info:      resp.Info,
		RawLog:    resp.RawLog,
	}

	if resp.Height > 0 {
		height := resp.Height
		doc.Height = &height
	}
	if resp.GasUsed > 0 {
		gasUsed := resp.GasUsed
		doc.GasUsed = &gasUsed
	}
	if resp.GasWanted > 0 {
		gasWanted := resp.GasWanted
		doc.GasWanted = &gasWanted
	}

	doc.Logs, doc.Events, doc.Tx = txResponseExtras(cctx, resp)

	return doc
}

// TxStatus classifies a broadcast result (SPEC §10.11.1).
func TxStatus(resp *sdk.TxResponse) string {
	switch {
	case resp == nil:
		return TxStatusPending
	case resp.Code != 0:
		return TxStatusFailed
	case resp.Height > 0:
		return TxStatusConfirmed
	default:
		return TxStatusPending
	}
}

// txResponseExtras carries the nested proto fields through verbatim from the
// codec's encoding so interface fields (Any) resolve to their concrete types.
// They are dropped when the client context has no codec to resolve them with.
func txResponseExtras(cctx sdkclient.Context, resp *sdk.TxResponse) (logs, events, txBody json.RawMessage) {
	if cctx.Codec == nil {
		return nil, nil, nil
	}

	payload, err := cctx.Codec.MarshalJSON(resp)
	if err != nil {
		return nil, nil, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, nil, nil
	}

	return nonEmptyJSON(fields["logs"]), nonEmptyJSON(fields["events"]), nonEmptyJSON(fields["tx"])
}

// nonEmptyJSON drops the values the proto encoder emits for absent fields when
// EmitDefaults is on, so they do not reappear as empty keys in the document.
func nonEmptyJSON(raw json.RawMessage) json.RawMessage {
	trimmed := json.RawMessage(bytes.TrimSpace(raw))
	switch string(trimmed) {
	case "", "null", "[]", "{}", `""`:
		return nil
	}

	return trimmed
}

func printTxResultStructured(w io.Writer, cctx sdkclient.Context, format string, resp *sdk.TxResponse) error {
	outputFormat := clioutput.FormatJSON
	if format == cflags.OutputYAML {
		outputFormat = clioutput.FormatYAML
	}

	return clioutput.FprintJSONSemantics(w, outputFormat, NewTxResultDocument(cctx, resp))
}

func printEncodedTransaction(w io.Writer, format string, payload []byte) error {
	payload = bytes.TrimSpace(payload)
	if !json.Valid(payload) {
		return fmt.Errorf("generated transaction is not valid JSON")
	}

	if format != cflags.OutputYAML {
		_, err := fmt.Fprintln(w, string(payload))
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}

	return clioutput.FprintJSONSemantics(w, clioutput.FormatYAML, value)
}

// Placeholders for fields whose value the response simply does not carry. A
// bare "-" (or a zero) reads like a missing or real value; these say why the
// value is absent.
const (
	unconfirmedValue = "not yet confirmed"
	notIncludedValue = "not included in a block"
	unreportedValue  = "not reported"
)

// renderTxSummaryWithCodec renders Section 1 using the provided client context
// for decoding the Tx body (signer, fee).
//
// A transaction broadcast in sync or async mode has only passed CheckTx: it is
// in the mempool with no height, no gas reading and no body. Section 1 says so
// explicitly instead of printing zeros, blanks or a green "success"
// (SPEC §10.11.2).
func renderTxSummaryWithCodec(w io.Writer, cctx sdkclient.Context, resp *sdk.TxResponse) {
	fmt.Fprintln(w, Section("Transaction"))

	KV(w, "Hash", Bold(resp.TxHash))

	// Decode the full tx for fee information.
	fee, haveFee := decodeTxFee(cctx, resp)

	// Extract signer from events (message.sender attribute).
	if signer := extractSigner(resp); signer != "" {
		KV(w, "Signer", signer)
	}

	status := TxStatus(resp)

	// Height and gas only exist once the transaction has been executed in a
	// block. Only a confirmed result has them; the others say so rather than
	// printing "-" or "0 / 0", which read as real readings.
	switch {
	case resp.Height > 0:
		KV(w, "Height", FormatHeight(resp.Height))
		KV(w, "Gas Used", fmt.Sprintf("%s / %s", FormatGas(resp.GasUsed), FormatGas(resp.GasWanted)))
	case status == TxStatusPending:
		KV(w, "Height", Dim(unconfirmedValue))
		KV(w, "Gas Used", Dim(unconfirmedValue))
	default:
		// Rejected during CheckTx: it will never reach a block, and the
		// response carries no gas accounting.
		KV(w, "Height", Dim(notIncludedValue))
		KV(w, "Gas Used", Dim(unreportedValue))
	}

	// The fee row is always emitted. Dropping it whenever the body could not be
	// decoded silently hid the fee on the most common path of all — a sync
	// broadcast never returns a body.
	switch {
	case haveFee:
		KV(w, "Fee", FormatCoins(fee))
	case status == TxStatusPending:
		KV(w, "Fee", Dim("not reported by a sync broadcast (query the tx to see it)"))
	default:
		KV(w, "Fee", Dim("unknown (transaction body not returned)"))
	}

	switch status {
	case TxStatusFailed:
		KV(w, "Status", StyleRed.Render(fmt.Sprintf("failed: %s", resp.RawLog)))
	case TxStatusPending:
		KV(w, "Status", StyleYellow.Render("pending")+" "+Dim("(accepted into the mempool, not yet in a block)"))
	default:
		KV(w, "Status", StyleGreen.Render("success"))
	}

	if status == TxStatusPending {
		KV(w, "Confirm With", fmt.Sprintf("akt query tx %s", resp.TxHash))
		KV(w, "Tip", Dim("broadcast with --broadcast-mode block to wait for inclusion"))
	}
}

// FormatGas formats a gas amount with comma grouping.
//
// This is deliberately not FormatHeight: that formatter renders "-" for any
// value <= 0 because block height zero does not exist, whereas gas zero is a
// legitimate reading. The comma grouping the two share is a coincidence.
func FormatGas(gas int64) string {
	if gas < 0 {
		return "-"
	}

	return formatWithCommas(gas)
}

// renderSingleMessage renders Section 2 for a single-message transaction.
func renderSingleMessage(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg sdk.Msg, resp *sdk.TxResponse, idx int) error {
	f, ok := LookupTx(msg)
	if ok {
		fmt.Fprintln(w, Section(f.Title()))
		return f.FormatTx(w, cmd, cctx, msg, resp, idx)
	}

	// No formatter registered — fall back to highlighted JSON.
	fmt.Fprintln(w, Section("Message"))
	renderMsgJSON(w, cctx, msg)
	return nil
}

// renderMultiMessage renders a numbered message section for multi-msg transactions.
func renderMultiMessage(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg sdk.Msg, resp *sdk.TxResponse, idx int, total int) error {
	num := idx + 1
	resp = responseForMultiMessage(resp)

	f, ok := LookupTx(msg)
	if ok {
		title := fmt.Sprintf("Message %d: %s", num, f.Title())
		if resp.Code != 0 {
			title += StyleRed.Render(" (failed)")
		}

		fmt.Fprintln(w, Section(title))
		return f.FormatTx(w, cmd, cctx, msg, resp, idx)
	}

	title := fmt.Sprintf("Message %d", num)
	fmt.Fprintln(w, Section(title))
	renderMsgJSON(w, cctx, msg)
	return nil
}

func responseForMultiMessage(resp *sdk.TxResponse) *sdk.TxResponse {
	if resp == nil || len(resp.Events) == 0 {
		return resp
	}
	filtered := *resp
	filtered.Events = make([]abci.Event, 0, len(resp.Events))
	for _, event := range resp.Events {
		for _, attribute := range event.Attributes {
			if attribute.Key == "msg_index" && normalizeTxEventValue(attribute.Value) != "" {
				filtered.Events = append(filtered.Events, event)
				break
			}
		}
	}
	return &filtered
}

// renderMsgJSON renders a message as highlighted JSON (fallback for unregistered types).
func renderMsgJSON(w io.Writer, cctx sdkclient.Context, msg sdk.Msg) {
	bz, err := cctx.Codec.MarshalJSON(msg)
	if err != nil {
		fmt.Fprintf(w, "  (cannot marshal message: %v)\n", err)
		return
	}

	_ = WriteHighlightedJSON(w, bz)
}

// decodeTxFee extracts the fee from the transaction body carried by a
// TxResponse. The bool reports whether a fee could be read at all; a decoded
// body with an empty fee is a legitimate zero-fee transaction.
func decodeTxFee(cctx sdkclient.Context, resp *sdk.TxResponse) (sdk.Coins, bool) {
	if resp == nil || resp.Tx == nil {
		return nil, false
	}

	// The registry route works whenever the body's concrete type is registered
	// as an sdk.FeeTx implementation.
	if cctx.Codec != nil {
		var feeTx sdk.FeeTx
		if err := cctx.Codec.UnpackAny(resp.Tx, &feeTx); err == nil && feeTx != nil {
			return feeTx.GetFee(), true
		}
	}

	// Otherwise decode the body as a plain cosmos.tx.v1beta1.Tx. That generated
	// type carries the fee but does not satisfy sdk.FeeTx — its FeePayer takes a
	// codec argument — so UnpackAny above can never resolve it, which is why the
	// Fee row used to go missing even on responses that did carry a body.
	if resp.Tx.TypeUrl != "/"+proto.MessageName(&txtypes.Tx{}) {
		return nil, false
	}

	var body txtypes.Tx
	if err := body.Unmarshal(resp.Tx.Value); err != nil {
		return nil, false
	}

	if body.AuthInfo == nil || body.AuthInfo.Fee == nil {
		return nil, false
	}

	return body.AuthInfo.Fee.Amount, true
}

// decodeTxMsgs extracts the sdk.Msg slice from a TxResponse.
func decodeTxMsgs(cctx sdkclient.Context, resp *sdk.TxResponse) []sdk.Msg {
	if resp.Tx == nil {
		return nil
	}

	if cctx.Codec == nil {
		return nil
	}

	var tx sdk.Tx
	if err := cctx.Codec.UnpackAny(resp.Tx, &tx); err != nil {
		return nil
	}

	return tx.GetMsgs()
}

// extractSigner pulls the first "message.sender" value from the TxResponse events.
func extractSigner(resp *sdk.TxResponse) string {
	for _, ev := range resp.Events {
		if ev.Type == "message" {
			for _, attr := range ev.Attributes {
				if attr.Key == "sender" {
					return attr.Value
				}
			}
		}
	}

	return ""
}
