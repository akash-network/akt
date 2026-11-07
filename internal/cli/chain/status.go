package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/capability"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	"pkg.akt.dev/akt/internal/output"
)

// StatusCmd queries the selected RPC without resolving a signing account.
func StatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "status",
		Short:       "Query remote node status",
		Args:        cobra.NoArgs,
		Example:     "akt status --node http://localhost:26657",
		Annotations: map[string]string{capability.AnnotationKey: string(capability.ChainQuery)},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cctx, err := GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			node, err := cctx.GetNode()
			if err != nil {
				return err
			}
			status, err := node.Status(cmd.Context())
			if err != nil {
				return err
			}
			if status == nil {
				return fmt.Errorf("missing node status")
			}
			format := output.FormatFromCmd(cmd)
			if format != output.FormatYAML {
				format = output.FormatJSON
			}
			return output.FprintJSONSemantics(cmd.OutOrStdout(), format, status)
		},
	}
	cmd.Flags().StringP(flagdefs.FlagNode, "n", "tcp://localhost:26657", "Node RPC endpoint (default: context RPC)")
	return cmd
}
