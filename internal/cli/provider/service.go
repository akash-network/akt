package provider

import (
	"fmt"

	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	aktprovider "pkg.akt.dev/akt/internal/provider"
)

func serviceStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-status [dseq] [service]",
		Short: "Query a service's status from its provider",
		Args:  cflags.PositionalOrFlagArgs(flagdefs.FlagDSeq, flagdefs.FlagService),
		Example: `  akt provider service-status 12345 web
  akt provider service-status --dseq 12345 --service web`,
		RunE: func(cmd *cobra.Command, args []string) error {
			service, _ := cmd.Flags().GetString(flagdefs.FlagService)
			if len(args) > 1 {
				service = args[1]
				args = args[:1]
			}
			if service == "" {
				return fmt.Errorf("service is required: provide [service] or --service")
			}
			lid, providerURL, err := resolveAuthenticatedLease(cmd, args)
			if err != nil {
				return err
			}
			cl, err := gatewayClientFromCmd(cmd, providerURL)
			if err != nil {
				return err
			}
			status, err := cl.ServiceStatus(cmd.Context(), lid, service)
			if err != nil {
				return aktprovider.GatewayError("query service status", err)
			}
			return printJSON(cmd, status)
		},
	}
	addLeaseFlags(cmd)
	cmd.Flags().String(flagdefs.FlagService, "", "Service name")
	return cmd
}
