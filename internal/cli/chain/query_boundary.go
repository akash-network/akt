package cli

import (
	"context"
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/cliutil"
	"pkg.akt.dev/akt/internal/output"
)

type queryVerboseContextKey struct{}

func rejectChangedQueryFlag(cmd *cobra.Command, flagName, reason string) error {
	if cmd.Flags().Lookup(flagName) == nil || !cmd.Flags().Changed(flagName) {
		return nil
	}

	return fmt.Errorf("--%s is not supported by %q: %s", flagName, cmd.CommandPath(), reason)
}

// localQueryPreRunE validates the query flags that still apply to a local
// derivation, and refuses transport or snapshot flags it cannot honor.
func localQueryPreRunE(cmd *cobra.Command, _ []string) error {
	cctx := GetClientContextFromCmd(cmd)
	if err := validateQueryChainID(cctx, cmd.Flags()); err != nil {
		return err
	}

	if err := rejectChangedQueryFlag(cmd, flags.FlagNode, "the result is computed locally and does not contact a node"); err != nil {
		return err
	}

	if err := rejectChangedQueryFlag(cmd, flags.FlagHeight, "the result is computed locally and does not read chain state"); err != nil {
		return err
	}

	verboseLocalQuery(cmd, cctx.ChainID)
	return nil
}

func rejectUnsupportedHeightPreRunE(cmd *cobra.Command, _ []string) error {
	return rejectChangedQueryFlag(cmd, flags.FlagHeight, "this query cannot select a historical snapshot")
}

func queryWithoutHeightPreRunE(cmd *cobra.Command, args []string) error {
	if err := rejectUnsupportedHeightPreRunE(cmd, args); err != nil {
		return err
	}

	return QueryPersistentPreRunE(cmd, args)
}

func rejectPositionalAndFlagHeightPreRunE(cmd *cobra.Command, args []string) error {
	if len(args) != 0 && cmd.Flags().Changed(flags.FlagHeight) {
		return fmt.Errorf("height cannot be supplied both positionally and with --%s", flags.FlagHeight)
	}

	return nil
}

func directQueryWithoutHeightPreRunE(cmd *cobra.Command, args []string) error {
	if err := rejectUnsupportedHeightPreRunE(cmd, args); err != nil {
		return err
	}

	return verboseDirectQueryPreRunE(cmd)
}

func directQueryHeightPreRunE(cmd *cobra.Command, args []string) error {
	if err := rejectPositionalAndFlagHeightPreRunE(cmd, args); err != nil {
		return err
	}

	return verboseDirectQueryPreRunE(cmd)
}

func verboseDirectQueryPreRunE(cmd *cobra.Command) error {
	cctx, err := GetClientQueryContext(cmd)
	if err != nil {
		return err
	}

	verboseQueryEndpoint(cmd, cctx)
	return nil
}

func verboseQueryEndpoint(cmd *cobra.Command, cctx sdkclient.Context) {
	if !markQueryVerbose(cmd) {
		return
	}
	cliutil.Verbosef(cmd, "querying %s (chain %s)", cctx.NodeURI, cctx.ChainID)
}

func verboseLocalQuery(cmd *cobra.Command, chainID string) {
	if !markQueryVerbose(cmd) {
		return
	}
	cliutil.Verbosef(cmd, "querying locally (chain %s)", chainID)
}

func markQueryVerbose(cmd *cobra.Command) bool {
	if !cliutil.IsVerbose(cmd) || cmd.Context().Value(queryVerboseContextKey{}) != nil {
		return false
	}

	cmd.SetContext(context.WithValue(cmd.Context(), queryVerboseContextKey{}, true))
	return true
}

func fileOutputQueryPreRunE(cmd *cobra.Command, args []string) error {
	switch output.FormatFromCmd(cmd) {
	case output.FormatJSON, output.FormatYAML:
		format, _ := cmd.Flags().GetString(flags.FlagOutput)
		return fmt.Errorf("--%s %s is not supported by %q: the query writes its result to the output filename", flags.FlagOutput, format, cmd.CommandPath())
	default:
		return QueryPersistentPreRunE(cmd, args)
	}
}

func printQueryScalar(cmd *cobra.Command, value string) error {
	format := output.FormatFromCmd(cmd)
	if format == output.FormatJSON || format == output.FormatYAML {
		return output.Fprint(cmd.OutOrStdout(), format, value)
	}

	_, err := fmt.Fprintln(cmd.OutOrStdout(), value)
	return err
}
