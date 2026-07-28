package cli

import (
	"errors"
	"fmt"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	"pkg.akt.dev/go/node/types/constants"
	"pkg.akt.dev/go/sdl"
	cutils "pkg.akt.dev/go/util/tls"
)

var (
	errDeploymentUpdate              = errors.New("deployment update failed")
	errDeploymentUpdateGroupsChanged = fmt.Errorf("%w: groups are different than existing deployment, you cannot update groups", errDeploymentUpdate)

	// errDSeqRequired is the fail-fast guard for the positional-only UX
	// trial: with --dseq disabled, the positional [dseq] argument is the only
	// source, so a missing (or zero) dseq must be rejected before the tx
	// pipeline (connection, keyring unlock, broadcast) is ever entered.
	errDSeqRequired = errors.New("dseq is required: provide the positional [dseq] argument")
)

// GetTxDeploymentCmds returns the transaction commands for this module
func GetTxDeploymentCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        dv1.ModuleName,
		Short:                      "Deployment transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}
	cmd.AddCommand(
		GetTxDeploymentCreateCmd(),
		GetTxDeploymentUpdateCmd(),
		GetTxDeploymentCloseCmd(),
		GetTxDeploymentGroupCmds(),
	)
	return cmd
}

func GetTxDeploymentCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create [sdl-file]",
		Short:             "Create deployment",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			// first lets validate certificate exists for given account
			if _, err := cutils.LoadAndQueryCertificateForAccount(ctx, cctx, nil); err != nil {
				if os.IsNotExist(err) {
					err = fmt.Errorf("no certificate file found for account %q.\n"+
						"consider creating it as certificate required to submit manifest", cctx.FromAddress.String())
				}

				return err
			}

			sdlManifest, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}

			groups, err := sdlManifest.DeploymentGroups()
			if err != nil {
				return err
			}

			warnIfGroupVolumesExceeds(cctx, groups)

			id, err := cflags.DeploymentIDFromFlags(cmd.Flags(), cflags.WithOwner(cctx.FromAddress))
			if err != nil {
				return err
			}

			// Default DSeq to the current block height
			if id.DSeq == 0 {
				syncInfo, err := cl.Node().SyncInfo(ctx)
				if err != nil {
					return err
				}

				if syncInfo.CatchingUp {
					return fmt.Errorf("cannot generate DSEQ from last block height. node is catching up")
				}

				id.DSeq = uint64(syncInfo.LatestBlockHeight) // nolint: gosec
			}

			version, err := sdlManifest.Version()
			if err != nil {
				return err
			}

			dep, err := DetectDeposit(ctx, cmd.Flags(), cl.Query(), DetectDeploymentDeposit)
			if err != nil {
				return err
			}

			msg := &dv1beta.MsgCreateDeployment{
				ID:      id,
				Hash:    version,
				Groups:  make(dv1beta.GroupSpecs, 0, len(groups)),
				Deposit: dep,
			}

			msg.Groups = append(msg.Groups, groups...)

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
	cflags.AddDepositFlags(cmd.Flags())

	return cmd
}

func GetTxDeploymentCloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <dseq>",
		Short: "Close deployment",
		// FEEDBACK(2026-07): --dseq disabled for the positional-only UX
		// trial; the positional dseq is the only source, so validate it here
		// — before the tx pipeline (connection, keyring, broadcast) starts.
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}

			dseq, err := cflags.DSeqFromArgs(args, 0)
			if err != nil {
				return err
			}
			if dseq == 0 {
				return errDSeqRequired
			}

			return nil
		},
		Example:           `akt tx deployment close 12345`,
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			id, err := cflags.DeploymentIDFromFlags(cmd.Flags(), cflags.WithOwner(cctx.FromAddress))
			if err != nil {
				return err
			}

			// FEEDBACK(2026-07): --dseq disabled for the positional-only UX
			// trial; the positional dseq is the only source. The Args hook
			// already validated it; this guard is defense in depth so a zero
			// dseq can never reach the broadcast pipeline.
			if id.DSeq, err = cflags.DSeqFromArgs(args, 0); err != nil {
				return err
			}
			if id.DSeq == 0 {
				return errDSeqRequired
			}

			msg := &dv1beta.MsgCloseDeployment{ID: id}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)
	addDeploymentOwnerTxFlags(cmd)
	return cmd
}

func GetTxDeploymentUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <sdl-file> <dseq>",
		Short: "update deployment",
		// FEEDBACK(2026-07): --dseq disabled for the positional-only UX
		// trial; the positional dseq is the only source, so validate it here
		// — before the tx pipeline (connection, keyring, broadcast) starts
		// and before deployment 0 is ever queried.
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.RangeArgs(1, 2)(cmd, args); err != nil {
				return err
			}

			dseq, err := cflags.DSeqFromArgs(args[1:], 0)
			if err != nil {
				return err
			}
			if dseq == 0 {
				return errDSeqRequired
			}

			return nil
		},
		Example:           `akt tx deployment update deploy.yaml 12345`,
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			id, err := cflags.DeploymentIDFromFlags(cmd.Flags(), cflags.WithOwner(cctx.FromAddress))
			if err != nil {
				return err
			}

			// FEEDBACK(2026-07): --dseq disabled for the positional-only UX
			// trial; the positional dseq is the only source. The Args hook
			// already validated it; this guard is defense in depth so a zero
			// dseq can never reach the query/broadcast pipeline.
			if id.DSeq, err = cflags.DSeqFromArgs(args[1:], 0); err != nil {
				return err
			}
			if id.DSeq == 0 {
				return errDSeqRequired
			}

			sdlManifest, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}

			hash, err := sdlManifest.Version()
			if err != nil {
				return err
			}

			groups, err := sdlManifest.DeploymentGroups()
			if err != nil {
				return err
			}

			// Query the RPC node to make sure the existing groups are identical
			existingDeployment, err := cl.Query().Deployment().Deployment(ctx, &dv1beta.QueryDeploymentRequest{
				ID: id,
			})
			if err != nil {
				return err
			}

			// do not send the transaction if the groups have changed
			existingGroups := existingDeployment.GetGroups()

			if len(existingGroups) != len(groups) {
				return errDeploymentUpdateGroupsChanged
			}

			for i, existingGroup := range existingGroups {
				if !existingGroup.GroupSpec.Equal(&groups[i]) {
					return errDeploymentUpdateGroupsChanged
				}
			}

			warnIfGroupVolumesExceeds(cctx, groups)

			msg := &dv1beta.MsgUpdateDeployment{
				ID:   id,
				Hash: hash,
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)
	addDeploymentOwnerTxFlags(cmd)

	return cmd
}

// addDeploymentOwnerTxFlags registers the deployment identity flags for the
// close/update tx commands: --owner stays (it defaults to the signer and has
// no positional twin).
// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial (use
// the positional [dseq] argument instead). Restore by replacing this helper's
// body with the original registration:
// cflags.AddDeploymentIDFlags(cmd.Flags())
func addDeploymentOwnerTxFlags(cmd *cobra.Command) {
	cmd.Flags().String(cflags.FlagOwner, "", "Deployment Owner")
	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().Uint64(cflags.FlagDSeq, 0, "Deployment Sequence")
}

func GetTxDeploymentGroupCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Modify a Deployment's specific Group",
	}

	cmd.AddCommand(
		GetTxDeploymentGroupCloseCmd(),
		GetDeploymentGroupPauseCmd(),
		GetDeploymentGroupStartCmd(),
	)

	return cmd
}

func GetTxDeploymentGroupCloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close [dseq] [gseq]",
		Short: "close a Deployment's specific Group",
		Example: `akt tx deployment group close 12345 1
akt tx deployment group close 12345 1 --owner=[Account Address]`,
		Args:              cobra.MaximumNArgs(2),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			id, err := groupIDFromFlagsAndArgs(cmd, args, cctx.GetFromAddress())
			if err != nil {
				return err
			}

			msg := &dv1beta.MsgCloseGroup{
				ID: id,
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
	addGroupOwnerTxFlags(cmd)

	return cmd
}

func GetDeploymentGroupPauseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause [dseq] [gseq]",
		Short: "pause a Deployment's specific Group",
		Example: `akt tx deployment group pause 12345 1
akt tx deployment group pause 12345 1 --owner=[Account Address]`,
		Args:              cobra.MaximumNArgs(2),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			id, err := groupIDFromFlagsAndArgs(cmd, args, cctx.GetFromAddress())
			if err != nil {
				return err
			}

			msg := &dv1beta.MsgPauseGroup{
				ID: id,
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
	addGroupOwnerTxFlags(cmd)

	return cmd
}

func GetDeploymentGroupStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start [dseq] [gseq]",
		Short: "start a Deployment's specific Group",
		Example: `akt tx deployment group start 12345 1
akt tx deployment group start 12345 1 --owner=[Account Address]`,
		Args:              cobra.MaximumNArgs(2),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			id, err := groupIDFromFlagsAndArgs(cmd, args, cctx.GetFromAddress())
			if err != nil {
				return err
			}

			msg := &dv1beta.MsgStartGroup{
				ID: id,
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
	addGroupOwnerTxFlags(cmd)

	return cmd
}

// addGroupOwnerTxFlags registers the group identity flags for the group tx
// commands: --owner stays (it defaults to the signer and has no positional
// twin).
// FEEDBACK(2026-07): --dseq/--gseq disabled for the positional-only UX trial
// (use the positional [dseq] [gseq] arguments instead). Restore by replacing
// this helper's body with the original registration:
// cflags.AddGroupIDFlags(cmd.Flags())
func addGroupOwnerTxFlags(cmd *cobra.Command) {
	cmd.Flags().String(cflags.FlagOwner, "", "Deployment Owner")
	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().Uint64(cflags.FlagDSeq, 0, "Deployment Sequence")
	// FEEDBACK(2026-07): --gseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().Uint32(cflags.FlagGSeq, 1, "Group Sequence")
}

// groupIDFromFlagsAndArgs resolves a GroupID from the optional positional
// [dseq] [gseq] arguments. The owner defaults to the signer (the from
// address) and may be overridden with --owner.
func groupIDFromFlagsAndArgs(cmd *cobra.Command, args []string, owner sdk.AccAddress) (dv1.GroupID, error) {
	id, err := cflags.GroupIDFromFlags(cmd.Flags(), cflags.WithOwner(owner))
	if err != nil {
		return dv1.GroupID{}, err
	}

	// FEEDBACK(2026-07): --dseq/--gseq disabled for the positional-only UX
	// trial; the positional [dseq] [gseq] arguments are the only source
	// (dseq zero fallback; gseq keeps its old flag default of 1).
	if id.DSeq, id.GSeq, err = cflags.GroupSeqsFromArgs(args, 0, 1); err != nil {
		return dv1.GroupID{}, err
	}

	if id.DSeq == 0 {
		return dv1.GroupID{}, errDSeqRequired
	}

	return id, nil
}

func warnIfGroupVolumesExceeds(cctx sdkclient.Context, dgroups []dv1beta.GroupSpec) {
	for _, group := range dgroups {
		for _, resources := range group.GetResourceUnits() {
			if len(resources.Storage) > constants.DefaultMaxGroupVolumes {
				_ = cctx.PrintString(fmt.Sprintf("amount of volumes for service exceeds recommended value (%v > %v)\n"+
					"there may no providers on network to bid", len(resources.Storage), constants.DefaultMaxGroupVolumes))
			}
		}
	}
}
