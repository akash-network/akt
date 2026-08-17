package cli

import (
	"fmt"
	"os"
	"path/filepath"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"cosmossdk.io/x/upgrade/plan"
	"cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	"github.com/cosmos/cosmos-sdk/x/gov/client/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

// GetTxUpgradeCmd returns the transaction commands for this module
func GetTxUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   types.ModuleName,
		Short: "Upgrade transaction subcommands",
	}

	cmd.AddCommand(
		NewCmdSubmitUpgradeProposal(),
		NewCmdSubmitCancelUpgradeProposal(),
	)

	return cmd
}

// NewCmdSubmitUpgradeProposal implements a command handler for submitting a software upgrade proposal transaction.
func NewCmdSubmitUpgradeProposal() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "software-upgrade [name] (--upgrade-height [height]) (--upgrade-info [info]) [flags]",
		Args:  cobra.ExactArgs(1),
		Short: "Submit a software upgrade proposal",
		Long: "Submit a software upgrade along with an initial deposit.\n" +
			"Please specify a unique name and height for the upgrade to take effect.\n" +
			"You may include info to reference a binary download link, in a format compatible with: https://docs.cosmos.network/main/tooling/cosmovisor",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ac := MustAddressCodecFromContext(ctx)

			cctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proposal, err := cli.ReadGovPropFlags(cctx, cmd.Flags())
			if err != nil {
				return err
			}

			name := args[0]
			p, err := parsePlan(cmd.Flags(), name)
			if err != nil {
				return err
			}

			noValidate, err := cmd.Flags().GetBool(flagdefs.FlagNoValidate)
			if err != nil {
				return err
			}

			if !noValidate {
				daemonName, err := cmd.Flags().GetString(flagdefs.FlagDaemonName)
				if err != nil {
					return err
				}

				noChecksum, err := cmd.Flags().GetBool(flagdefs.FlagNoChecksumRequired)
				if err != nil {
					return err
				}

				var planInfo *plan.Info
				if planInfo, err = plan.ParseInfo(p.Info, plan.ParseOptionEnforceChecksum(!noChecksum)); err != nil {
					return err
				}

				if err = planInfo.ValidateFull(daemonName); err != nil {
					return err
				}
			}

			authority, _ := cmd.Flags().GetString(flagdefs.FlagAuthority)
			if authority != "" {
				if _, err = ac.StringToBytes(authority); err != nil {
					return fmt.Errorf("invalid authority address: %w", err)
				}
			} else {
				authority = sdk.AccAddress(address.Module("gov")).String()
			}

			if err := proposal.SetMsgs([]sdk.Msg{
				&types.MsgSoftwareUpgrade{
					Authority: authority,
					Plan:      p,
				},
			}); err != nil {
				return fmt.Errorf("failed to create submit upgrade proposal message: %w", err)
			}

			return tx.GenerateOrBroadcastTxCLI(cctx, cmd.Flags(), proposal)
		},
	}

	cmd.Flags().Int64(flagdefs.FlagUpgradeHeight, 0, "The height at which the upgrade must happen")
	cmd.Flags().String(flagdefs.FlagUpgradeInfo, "", "Info for the upgrade plan such as new version download urls, etc.")
	cmd.Flags().Bool(flagdefs.FlagNoValidate, false, "Skip validation of the upgrade info (dangerous!)")
	cmd.Flags().Bool(flagdefs.FlagNoChecksumRequired, false, "Skip requirement of checksums for binaries in the upgrade info")
	cmd.Flags().String(flagdefs.FlagDaemonName, getDefaultDaemonName(), "The name of the executable being upgraded (for upgrade-info validation). Default is the DAEMON_NAME env var if set, or else this executable")
	cmd.Flags().String(flagdefs.FlagAuthority, "", "The address of the upgrade module authority (defaults to gov)")

	// add common proposal flags
	cflags.AddTxFlagsToCmd(cmd)
	cflags.AddGovPropFlagsToCmd(cmd)
	_ = cmd.MarkFlagRequired(flagdefs.FlagTitle)

	return cmd
}

// NewCmdSubmitCancelUpgradeProposal implements a command handler for submitting a software upgrade cancel proposal transaction.
func NewCmdSubmitCancelUpgradeProposal() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel-software-upgrade [flags]",
		Args:  cobra.ExactArgs(0),
		Short: "Cancel the current software upgrade proposal",
		Long:  "Cancel a software upgrade along with an initial deposit.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			ac := MustAddressCodecFromContext(ctx)

			cctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			proposal, err := cli.ReadGovPropFlags(cctx, cmd.Flags())
			if err != nil {
				return err
			}

			authority, _ := cmd.Flags().GetString(flagdefs.FlagAuthority)
			if authority != "" {
				if _, err = ac.StringToBytes(authority); err != nil {
					return fmt.Errorf("invalid authority address: %w", err)
				}
			} else {
				authority = sdk.AccAddress(address.Module("gov")).String()
			}

			if err := proposal.SetMsgs([]sdk.Msg{
				&types.MsgCancelUpgrade{
					Authority: authority,
				},
			}); err != nil {
				return fmt.Errorf("failed to create cancel upgrade proposal message: %w", err)
			}

			return tx.GenerateOrBroadcastTxCLI(cctx, cmd.Flags(), proposal)
		},
	}

	cmd.Flags().String(flagdefs.FlagAuthority, "", "The address of the upgrade module authority (defaults to gov)")

	// add common proposal flags
	cflags.AddTxFlagsToCmd(cmd)
	cflags.AddGovPropFlagsToCmd(cmd)
	_ = cmd.MarkFlagRequired(flagdefs.FlagTitle)

	return cmd
}

// getDefaultDaemonName gets the default name to use for the daemon.
// If a DAEMON_NAME env var is set, that is used.
// Otherwise, the last part of the currently running executable is used.
func getDefaultDaemonName() string {
	// DAEMON_NAME is specifically used here to correspond with the Cosmovisor setup env vars.
	name := os.Getenv("DAEMON_NAME")
	if len(name) == 0 {
		_, name = filepath.Split(os.Args[0])
	}
	return name
}

func parsePlan(fs *pflag.FlagSet, name string) (types.Plan, error) {
	height, err := fs.GetInt64(flagdefs.FlagUpgradeHeight)
	if err != nil {
		return types.Plan{}, err
	}

	info, err := fs.GetString(flagdefs.FlagUpgradeInfo)
	if err != nil {
		return types.Plan{}, err
	}

	return types.Plan{Name: name, Height: height, Info: info}, nil
}
