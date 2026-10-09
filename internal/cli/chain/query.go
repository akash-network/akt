package cli

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/cobra"

	ibctransfer "github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	ibccore "github.com/cosmos/ibc-go/v10/modules/core"

	"pkg.akt.dev/akt/internal/capability"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/cliutil"
	aclient "pkg.akt.dev/go/node/client/discovery"
)

func QueryPersistentPreRunE(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	if cmd.Flags().Changed(flagdefs.FlagNode) {
		rpcURI, _ := cmd.Flags().GetString(flagdefs.FlagNode)
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

	// -v answers "which endpoint did this actually talk to", the first thing
	// worth knowing when a query returns something unexpected. Until now the
	// flag was accepted on every command and produced nothing anywhere.
	cliutil.Verbosef(cmd, "querying %s (chain %s)", cctx.NodeURI, cctx.ChainID)

	return nil
}

func QueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "query [contract-address]",
		Aliases: []string{"q"},
		Short:   "Querying subcommands",
		Example: "  akt q contracts\n  akt q contract \"my contract\"\n  akt q akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			if len(args) == 1 {
				if _, err := sdk.AccAddressFromBech32(args[0]); err == nil {
					return nil
				}
			}
			return ValidateCmd(cmd, args)
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return QueryPersistentPreRunE(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return queryContract(cmd, args)
		},
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
		adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd()),
		adoptVendoredQueryCmd(ibctransfer.AppModuleBasic{}.GetQueryCmd()),
		QueryBlockCmd(),
		QueryBlocksCmd(),
		QueryBlockResultsCmd(),
		GetQueryAuthTxsByEventsCmd(),
		GetQueryAuthTxCmd(),
		GetQueryGovCmd(),
		GetQuerySlashingCmd(),
		GetQueryStakingCmd(),
		GetQueryUpgradeCmd(),
		GetQueryAuditCmd(),
		GetQueryCertCmd(),
		GetQueryContractCmd(),
		GetQueryDeploymentCmds(),
		GetQueryMarketCmds(),
		GetQueryEscrowCmd(),
		GetQueryProviderCmds(),
		GetQueryWasmCmd(),
		GetQueryOracleCmd(),
		GetQueryBMECmd(),
		GetQueryModuleNameToAddressCmd(),
	)

	cmd.PersistentFlags().String(flagdefs.FlagChainID, "", "The network chain ID")
	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}
