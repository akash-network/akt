package pretty

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	clioutput "pkg.akt.dev/akt/internal/output"
)

// PrintQueryResult is the main dispatch function for query output.
//
// It reads --output (-o) to decide format:
//   - "pretty" (default): look up a registered PrettyFormatter for the message type.
//     If one exists, render pretty output. Otherwise, fall back to JSON.
//   - "json": raw Cosmos SDK JSON output via clientCtx.PrintProto().
//   - "yaml": raw Cosmos SDK YAML output via clientCtx.PrintProto().
func PrintQueryResult(cmd *cobra.Command, cctx sdkclient.Context, msg proto.Message) error {
	output, _ := cmd.Flags().GetString(cflags.FlagOutput)
	checked := clioutput.NewCheckedWriter(cmd.OutOrStdout())
	cctx = cctx.WithOutput(checked)

	switch output {
	case cflags.OutputJSON:
		return checked.Complete(cctx.WithOutputFormat("json").PrintProto(msg))
	case cflags.OutputYAML:
		return checked.Complete(cctx.WithOutputFormat("text").PrintProto(msg))
	default:
		// "pretty" or unset — use registered formatter if available.
		f, ok := Lookup(msg)
		if !ok {
			// No formatter registered — fall back to JSON.
			return checked.Complete(cctx.WithOutputFormat("json").PrintProto(msg))
		}

		return checked.Complete(f.Format(clioutput.TerminalAwareWriter(checked), cmd, cctx, msg))
	}
}

// PrintQueryResultAny is like PrintQueryResult but accepts interface{} to
// support call sites that pass sub-fields (e.g., &res.Params).
// If msg implements proto.Message, it dispatches through the formatter registry.
// Otherwise, it falls back to the client context's legacy print methods.
func PrintQueryResultAny(cmd *cobra.Command, cctx sdkclient.Context, msg interface{}) error {
	if pm, ok := msg.(proto.Message); ok {
		return PrintQueryResult(cmd, cctx, pm)
	}

	// Not a proto.Message — use legacy print (e.g., for amino-encoded types).
	checked := clioutput.NewCheckedWriter(cmd.OutOrStdout())
	//nolint:staticcheck // SA1019: PrintObjectLegacy is the only client-context
	// printer that handles non-proto (amino) values. The proto-only replacements
	// cannot encode this input; revisit when chain-sdk stops returning interface{}.
	return checked.Complete(cctx.WithOutput(checked).PrintObjectLegacy(msg))
}
