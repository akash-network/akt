package cli

import (
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	eid "pkg.akt.dev/go/node/escrow/id/v1"
	emodule "pkg.akt.dev/go/node/escrow/module"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
)

func GetTxEscrowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        emodule.ModuleName,
		Short:                      "Escrow transaction commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}
	cmd.AddCommand(
		GetTxEscrowDeposit(),
	)

	return cmd
}

func GetTxEscrowDeposit() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deposit deployment <amount> --dseq <dseq>",
		Short: "Add funds to a deployment's escrow account",
		Long: `Add funds to a deployment's escrow account.

The first argument is the escrow scope and must be the literal word
"deployment" -- it is the only scope this command supports. The deployment
itself is chosen with --dseq.

Deposited funds are locked in escrow and drawn down per block by the
deployment's leases. Whatever is left is returned when the deployment is
closed.`,
		Example: `  # Add 5 AKT to deployment 12345
  akt tx escrow deposit deployment 5000000uakt --dseq 12345 --from mykey`,
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			var aid eid.Account

			switch args[0] {
			case "deployment":
				id, err := cflags.DeploymentIDFromFlags(cmd.Flags(), cflags.WithOwner(cctx.FromAddress))
				if err != nil {
					return err
				}
				if id.DSeq == 0 {
					return errDSeqRequired
				}
				aid = id.ToEscrowAccountID()
			default:
				return fmt.Errorf("invalid account scope. allowed values deployment")
			}

			amount, err := sdk.ParseCoinNormalized(args[1])
			if err != nil {
				return err
			}

			sources, err := DepositSources(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &ev1.MsgAccountDeposit{
				ID:     aid,
				Signer: cctx.FromAddress.String(),
				Deposit: deposit.Deposit{
					Amount:  amount,
					Sources: sources,
				},
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
	cflags.AddDeploymentIDFlags(cmd.Flags())
	cflags.AddDepositSourcesFlags(cmd.Flags())

	return cmd
}
