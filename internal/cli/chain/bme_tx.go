package cli

import (
	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	types "pkg.akt.dev/go/node/bme/v1"
)

// GetTxBMECmd returns the transaction commands for bme module
func GetTxBMECmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "BME transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetTxBMEBurnMintCmd(),
		GetTxBMEMintACTCmd(),
		GetTxBMEBurnACTCmd(),
	)

	return cmd
}

// GetTxBMEBurnMintCmd returns the command to burn one token and mint another
func GetTxBMEBurnMintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "burn-mint [coins-to-burn] [denom-to-mint]",
		Short: "Burn tokens to mint another denomination",
		Long: `Burn tokens to mint another denomination.
This allows burning AKT to mint ACT, or burning unused ACT back to AKT.

The conversion does not happen in this transaction. A successful broadcast
means the chain accepted the request and recorded a pending ledger entry; it
burns and mints in a later block, once the oracle price is available. Until it
settles the burned amount has left your balance and the minted coins have not
arrived, and the minted amount is not known because it depends on the oracle
price at settlement. Follow the pending entry with "akt q bme ledger".`,
		Example: `  akt tx bme burn-mint 1000000uakt uact --from mykey
  akt tx bme burn-mint 500000uact uakt --from mykey

  # Then watch it settle
  akt q bme ledger --owner <your-address> --status ledger_record_status_pending`,
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			// Parse the coin to burn
			coinsToBurn, err := sdk.ParseCoinNormalized(args[0])
			if err != nil {
				return err
			}

			// Validate the denom to mint
			denomToMint := args[1]
			if err := sdk.ValidateDenom(denomToMint); err != nil {
				return err
			}

			// Get signer address from client context
			cctx, err := GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			fromAddr := cctx.GetFromAddress().String()

			msg := &types.MsgBurnMint{
				Owner:       fromAddr,
				To:          fromAddr,
				CoinsToBurn: coinsToBurn,
				DenomToMint: denomToMint,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)

	return cmd
}

// GetTxBMEMintACTCmd returns the command to burn one token and mint another
func GetTxBMEMintACTCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mint-act [coins-to-burn]",
		Short: "Mint ACT by burning AKT",
		Long: `Burn AKT to mint ACT.

The conversion does not happen in this transaction. A successful broadcast
means the chain accepted the request and recorded a pending ledger entry; it
burns and mints in a later block, once the oracle price is available. Until it
settles the burned AKT has left your balance and the ACT has not arrived, and
the minted amount is not known because it depends on the oracle price at
settlement. Follow the pending entry with "akt q bme ledger".`,
		Example: `  akt tx bme mint-act 500000uakt --from mykey

  # Then watch it settle
  akt q bme ledger --owner <your-address> --status ledger_record_status_pending`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			// Parse the coin to burn
			coinsToBurn, err := sdk.ParseCoinNormalized(args[0])
			if err != nil {
				return err
			}

			// Get signer address from client context
			cctx, err := GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			fromAddr := cctx.GetFromAddress().String()

			msg := &types.MsgMintACT{
				Owner:       fromAddr,
				To:          fromAddr,
				CoinsToBurn: coinsToBurn,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)

	return cmd
}

// GetTxBMEBurnACTCmd returns the command to burn one token and mint another
func GetTxBMEBurnACTCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "burn-act [coins-to-burn]",
		Short: "Burn ACT tokens to mint/remint AKT",
		Long: `Burn ACT to mint or remint AKT.

The conversion does not happen in this transaction. A successful broadcast
means the chain accepted the request and recorded a pending ledger entry; it
burns and mints in a later block, once the oracle price is available. Until it
settles the burned ACT has left your balance and the AKT has not arrived, and
the minted amount is not known because it depends on the oracle price at
settlement. Follow the pending entry with "akt q bme ledger".`,
		Example: `  akt tx bme burn-act 500000uact --from mykey

  # Then watch it settle
  akt q bme ledger --owner <your-address> --status ledger_record_status_pending`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			// Parse the coin to burn
			coinsToBurn, err := sdk.ParseCoinNormalized(args[0])
			if err != nil {
				return err
			}

			// Get signer address from client context
			cctx, err := GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			fromAddr := cctx.GetFromAddress().String()

			msg := &types.MsgBurnACT{
				Owner:       fromAddr,
				To:          fromAddr,
				CoinsToBurn: coinsToBurn,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)

	return cmd
}
