package cli

import (
	"context"

	"github.com/spf13/cobra"

	ibctransfer "github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	ibccore "github.com/cosmos/ibc-go/v10/modules/core"

	"pkg.akt.dev/akt/internal/capability"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aclient "pkg.akt.dev/go/node/client/discovery"
)

func QueryPersistentPreRunE(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	if cmd.Flags().Changed(cflags.FlagNode) {
		rpcURI, _ := cmd.Flags().GetString(cflags.FlagNode)
		ctx = context.WithValue(ctx, ContextTypeRPCURI, rpcURI)
		cmd.SetContext(ctx)
	}

	cctx, err := GetClientQueryContext(cmd)
	if err != nil {
		return err
	}

	if _, err = LightClientFromContext(ctx); err != nil {
		cl, err := aclient.DiscoverLightClient(ctx, cctx)
		if err != nil {
			return err
		}

		ctx = context.WithValue(ctx, ContextTypeQueryClient, cl)

		cmd.SetContext(ctx)
	}

	return nil
}

func QueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "query",
		Aliases: []string{"q"},
		Short:   "Querying subcommands",
		RunE:    ValidateCmd,
		// Capability gating: chain queries require a chain RPC endpoint.
		Annotations: map[string]string{capability.AnnotationKey: string(capability.ChainQuery)},
	}

	cmd.AddCommand(
		GetQueryAuthCmd(),
		GetQueryAuthzCmd(),
		GetQueryBankCmd(),
		GetQueryDistributionCmd(),
		GetQueryEvidenceCmd(),
		GetQueryFeegrantCmd(),
		GetQueryMintCmd(),
		GetQueryParamsCmd(),
		cflags.LineBreak,
		adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd()),
		adoptVendoredQueryCmd(ibctransfer.AppModuleBasic{}.GetQueryCmd()),
		cflags.LineBreak,
		QueryBlockCmd(),
		QueryBlocksCmd(),
		QueryBlockResultsCmd(),
		GetQueryAuthTxsByEventsCmd(),
		GetQueryAuthTxCmd(),
		GetQueryGovCmd(),
		GetQuerySlashingCmd(),
		GetQueryStakingCmd(),
		cflags.LineBreak,
		GetQueryAuditCmd(),
		GetQueryCertCmd(),
		GetQueryDeploymentCmds(),
		GetQueryMarketCmds(),
		GetQueryEscrowCmd(),
		GetQueryProviderCmds(),
		GetQueryWasmCmd(),
		GetQueryOracleCmd(),
		GetQueryBMECmd(),
		GetQueryModuleNameToAddressCmd(),
	)

	cmd.PersistentFlags().String(cflags.FlagChainID, "", "The network chain ID")

	return cmd
}
