package cli

import (
	"context"
	"errors"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	ibctransfer "github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	ibccore "github.com/cosmos/ibc-go/v10/modules/core"

	"pkg.akt.dev/akt/internal/capability"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aclient "pkg.akt.dev/go/node/client/discovery"
)

func TxPersistentPreRunE(cmd *cobra.Command, args []string) error {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return nil
		}
	}

	ctx := cmd.Context()

	if cmd.Flags().Changed(flagdefs.FlagNode) {
		rpcURI, _ := cmd.Flags().GetString(flagdefs.FlagNode)
		ctx = context.WithValue(ctx, ContextTypeRPCURI, rpcURI)
		cmd.SetContext(ctx)
	}

	cctx, err := GetClientTxContext(cmd)
	if err != nil {
		return err
	}
	if commandDerivesFees(cmd) {
		if err := reconcileGasPricesWithNode(ctx, cctx, cmd.Flags()); err != nil {
			return err
		}
	}
	// Persist the fully resolved context for SDK-owned handlers that call the
	// Cosmos client package directly. Without this, their second flag read sees
	// a nil RPC client and recreates it from the SDK localhost default.
	if err := SetCmdClientContext(cmd, cctx); err != nil {
		return err
	}

	if cctx.Codec == nil {
		return errors.New("codec is not initialized")
	}

	if cctx.LegacyAmino == nil {
		return errors.New("legacy amino codec is not initialized")
	}

	if _, err = ClientFromContext(ctx); err != nil {
		opts, err := cflags.ClientOptionsFromFlags(cmd.Flags())
		if err != nil {
			return err
		}

		cl, err := aclient.DiscoverClient(ctx, clientContextForTxClient(cctx), opts...)
		if err != nil {
			return err
		}

		// Record every broadcast in the per-context action log (SPEC §5.6).
		wrapped := WithActionLog(ctx, cl)

		ctx = context.WithValue(ctx, ContextTypeClient, wrapped)

		cmd.SetContext(ctx)
	}

	return nil
}

// clientContextForTxClient prevents the downstream transaction client from
// interpreting an already parsed address as a key name. Unsigned construction
// needs the address for messages but does not sign.
func clientContextForTxClient(cctx sdkclient.Context) sdkclient.Context {
	addressOnly := cctx.FromName == "" && len(cctx.FromAddress) != 0
	if addressOnly && cctx.GenerateOnly {
		return cctx.WithFrom("")
	}

	return cctx
}

func TxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx",
		RunE:  ValidateCmd,
		Short: "Transactions subcommands",
		// Capability gating: broadcasting requires a chain RPC endpoint.
		Annotations: map[string]string{capability.AnnotationKey: string(capability.ChainTx)},
	}

	cmd.AddCommand(
		GetTxAuthzCmd(),
		GetTxBankCmd(),
		getTxDistributionCmd(),
		GetTxEscrowCmd(),
		GetTxFeegrantCmd(),
		GetSignCommand(),
		GetSignBatchCommand(),
		GetAuthMultiSignCmd(),
		GetValidateSignaturesCommand(),
		GetBroadcastCommand(),
		GetEncodeCommand(),
		GetDecodeCommand(),
		GetTxVestingCmd(),
		adoptVendoredTxCmd(withoutEmptyVendoredGroup(ibccore.AppModuleBasic{}.GetTxCmd(), "channelv2")),
		adoptVendoredTxCmd(ibctransfer.AppModuleBasic{}.GetTxCmd()),
		GetTxAuditCmd(),
		GetTxCertCmd(),
		GetTxDeploymentCmds(),
		GetTxMarketCmds(),
		GetTxProviderCmd(),
		GetTxGovCmd(
			[]*cobra.Command{
				GetTxParamsSubmitParamChangeProposalCmd(),
			},
		),
		GetTxSlashingCmd(),
		GetTxStakingCmd(),
		adoptVendoredTxCmd(GetTxUpgradeCmd()),
		GetTxWasmCmd(),
		GetTxOracleCmd(),
		GetTxBMECmd(),
	)

	cmd.PersistentFlags().String(flagdefs.FlagChainID, "", "The network chain ID")

	return cmd
}
