package pretty

import (
	"fmt"
	"io"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

// PrintTxResult is the main dispatch function for transaction output (SPEC §10.11).
//
// It reads --output (-o) to decide format:
//   - "pretty" (default): two-section layout (common summary + message detail).
//   - "json": raw TxResponse JSON via clientCtx.PrintProto().
//   - "yaml": raw TxResponse YAML via clientCtx.PrintProto().
//
// The resp parameter is interface{} because chain-sdk's BroadcastMsgs returns
// interface{}. It is expected to be *sdk.TxResponse or proto.Message.
func PrintTxResult(cmd *cobra.Command, cctx sdkclient.Context, resp interface{}) error {
	output, _ := cmd.Flags().GetString(cflags.FlagOutput)

	// For json/yaml, delegate to the SDK printer.
	switch output {
	case cflags.OutputJSON:
		if pm, ok := resp.(proto.Message); ok {
			return cctx.WithOutputFormat("json").PrintProto(pm)
		}

		//nolint:staticcheck // SA1019: PrintObjectLegacy is the only client-context
		// printer that accepts a non-proto (amino) value, which is exactly what
		// this fallback branch has. See PrintQueryResultAny in printer.go.
		return cctx.PrintObjectLegacy(resp)
	case cflags.OutputYAML:
		if pm, ok := resp.(proto.Message); ok {
			return cctx.WithOutputFormat("text").PrintProto(pm)
		}

		//nolint:staticcheck // SA1019: PrintObjectLegacy is the only client-context
		// printer that accepts a non-proto (amino) value, which is exactly what
		// this fallback branch has. See PrintQueryResultAny in printer.go.
		return cctx.PrintObjectLegacy(resp)
	}

	// Pretty mode — render the two-section layout.
	txResp, ok := resp.(*sdk.TxResponse)
	if !ok {
		// Not a TxResponse — fall back to JSON.
		if pm, ok := resp.(proto.Message); ok {
			return cctx.WithOutputFormat("json").PrintProto(pm)
		}

		//nolint:staticcheck // SA1019: PrintObjectLegacy is the only client-context
		// printer that accepts a non-proto (amino) value, which is exactly what
		// this fallback branch has. See PrintQueryResultAny in printer.go.
		return cctx.PrintObjectLegacy(resp)
	}

	w := os.Stdout

	// Section 1: Common transaction summary.
	renderTxSummaryWithCodec(w, cctx, txResp)

	// Section 2: Message-specific detail.
	msgs := decodeTxMsgs(cctx, txResp)
	if len(msgs) == 0 {
		return nil
	}

	Newline(w)

	if len(msgs) == 1 {
		renderSingleMessage(w, cmd, cctx, msgs[0], txResp, 0)
	} else {
		for i, msg := range msgs {
			if i > 0 {
				Newline(w)
			}

			renderMultiMessage(w, cmd, cctx, msg, txResp, i, len(msgs))
		}
	}

	return nil
}

// renderTxSummaryWithCodec renders Section 1 using the provided client context
// for decoding the Tx body (signer, fee).
func renderTxSummaryWithCodec(w io.Writer, cctx sdkclient.Context, resp *sdk.TxResponse) {
	fmt.Fprintln(w, Section("Transaction"))

	KV(w, "Hash", Bold(resp.TxHash))

	// Decode the full tx for fee information.
	feeTx, _ := decodeTxFee(cctx, resp)

	// Extract signer from events (message.sender attribute).
	if signer := extractSigner(resp); signer != "" {
		KV(w, "Signer", signer)
	}

	KV(w, "Height", FormatHeight(resp.Height))
	KV(w, "Gas Used", fmt.Sprintf("%s / %s", FormatHeight(resp.GasUsed), FormatHeight(resp.GasWanted)))

	if feeTx != nil {
		fee := feeTx.GetFee()
		if len(fee) > 0 {
			KV(w, "Fee", FormatCoins(fee))
		}
	}

	if resp.Code == 0 {
		KV(w, "Status", StyleGreen.Render("success"))
	} else {
		msg := fmt.Sprintf("failed: %s", resp.RawLog)
		KV(w, "Status", StyleRed.Render(msg))
	}
}

// renderSingleMessage renders Section 2 for a single-message transaction.
func renderSingleMessage(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg sdk.Msg, resp *sdk.TxResponse, idx int) {
	f, ok := LookupTx(msg)
	if ok {
		fmt.Fprintln(w, Section(f.Title()))
		_ = f.FormatTx(w, cmd, cctx, msg, resp, idx)

		return
	}

	// No formatter registered — fall back to highlighted JSON.
	fmt.Fprintln(w, Section("Message"))
	renderMsgJSON(w, cctx, msg)
}

// renderMultiMessage renders a numbered message section for multi-msg transactions.
func renderMultiMessage(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg sdk.Msg, resp *sdk.TxResponse, idx int, total int) {
	num := idx + 1

	f, ok := LookupTx(msg)
	if ok {
		title := fmt.Sprintf("Message %d: %s", num, f.Title())
		if resp.Code != 0 {
			title += StyleRed.Render(" (failed)")
		}

		fmt.Fprintln(w, Section(title))
		_ = f.FormatTx(w, cmd, cctx, msg, resp, idx)

		return
	}

	title := fmt.Sprintf("Message %d", num)
	fmt.Fprintln(w, Section(title))
	renderMsgJSON(w, cctx, msg)
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

// decodeTxFee extracts the FeeTx interface from a TxResponse using the codec.
func decodeTxFee(cctx sdkclient.Context, resp *sdk.TxResponse) (sdk.FeeTx, error) {
	if resp.Tx == nil {
		return nil, fmt.Errorf("no tx in response")
	}

	if cctx.Codec == nil {
		return nil, fmt.Errorf("no codec in client context")
	}

	var feeTx sdk.FeeTx
	if err := cctx.Codec.UnpackAny(resp.Tx, &feeTx); err != nil {
		return nil, err
	}

	return feeTx, nil
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
