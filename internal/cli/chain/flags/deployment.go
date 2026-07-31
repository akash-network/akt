package flags

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
)

type DeploymentIDOptions struct {
	NoOwner bool
}

type DeploymentIDOption func(*DeploymentIDOptions)

// DeploymentIDOptionNoOwner do not add mark as required owner flag
func DeploymentIDOptionNoOwner(val bool) DeploymentIDOption {
	return func(opt *DeploymentIDOptions) {
		opt.NoOwner = val
	}
}

type MarketOptions struct {
	Owner    sdk.AccAddress
	Provider sdk.AccAddress
}

type MarketOption func(*MarketOptions)

func WithOwner(val sdk.AccAddress) MarketOption {
	return func(opt *MarketOptions) {
		opt.Owner = val
	}
}

func WithProvider(val sdk.AccAddress) MarketOption {
	return func(opt *MarketOptions) {
		opt.Provider = val
	}
}

// AddDeploymentIDFlags add flags for deployment except for Owner when NoOwner is set
func AddDeploymentIDFlags(flags *pflag.FlagSet, opts ...DeploymentIDOption) {
	opt := &DeploymentIDOptions{}

	for _, o := range opts {
		o(opt)
	}

	if !opt.NoOwner {
		flags.String(FlagOwner, "", "Deployment Owner")
	}

	flags.Uint64(FlagDSeq, 0, "Deployment Sequence")
}

// MarkReqDeploymentIDFlags marks flags required except for Owner when NoOwner is set
func MarkReqDeploymentIDFlags(cmd *cobra.Command, opts ...DeploymentIDOption) {
	opt := &DeploymentIDOptions{}

	for _, o := range opts {
		o(opt)
	}

	if !opt.NoOwner {
		_ = cmd.MarkFlagRequired(FlagOwner)
	}

	_ = cmd.MarkFlagRequired(FlagDSeq)
}

// DeploymentIDFromFlags returns DeploymentID with given flags, owner and error if occurred
func DeploymentIDFromFlags(flags *pflag.FlagSet, opts ...MarketOption) (dv1.DeploymentID, error) {
	var id dv1.DeploymentID
	opt := &MarketOptions{}

	for _, o := range opts {
		o(opt)
	}

	var owner string
	if flag := flags.Lookup(FlagOwner); flag != nil {
		owner = flag.Value.String()
	}

	// if --owner flag was explicitly provided, use that.
	var err error
	if owner != "" {
		opt.Owner, err = sdk.AccAddressFromBech32(owner)
		if err != nil {
			return id, err
		}
	}

	id.Owner = opt.Owner.String()

	// FEEDBACK(2026-07): several commands disabled --dseq for the
	// positional-only UX trial, so tolerate a missing flag and leave DSeq
	// zero (the positional argument is then the only source).
	if flags.Lookup(FlagDSeq) != nil {
		if id.DSeq, err = flags.GetUint64(FlagDSeq); err != nil {
			return id, err
		}
	}

	return id, nil
}

// DeploymentIDFromFlagsForOwner returns DeploymentID with given flags, owner and error if occurred
func DeploymentIDFromFlagsForOwner(flags *pflag.FlagSet, owner sdk.Address) (dv1.DeploymentID, error) {
	id := dv1.DeploymentID{
		Owner: owner.String(),
	}

	var err error
	if id.DSeq, err = flags.GetUint64(FlagDSeq); err != nil {
		return id, err
	}

	return id, nil
}

// AddGroupIDFlags add flags for Group
func AddGroupIDFlags(flags *pflag.FlagSet, opts ...DeploymentIDOption) {
	AddDeploymentIDFlags(flags, opts...)
	flags.Uint32(FlagGSeq, 1, "Group Sequence")
}

// MarkReqGroupIDFlags marks flags required for group
func MarkReqGroupIDFlags(cmd *cobra.Command, opts ...DeploymentIDOption) {
	MarkReqDeploymentIDFlags(cmd, opts...)
}

// GroupIDFromFlags returns GroupID with given flags and error if occurred
func GroupIDFromFlags(flags *pflag.FlagSet, opts ...MarketOption) (dv1.GroupID, error) {
	var id dv1.GroupID
	prev, err := DeploymentIDFromFlags(flags, opts...)
	if err != nil {
		return id, err
	}

	// FEEDBACK(2026-07): several commands disabled --gseq for the
	// positional-only UX trial, so tolerate a missing flag and leave GSeq
	// zero (the positional argument is then the only source).
	var gseq uint32
	if flags.Lookup(FlagGSeq) != nil {
		if gseq, err = flags.GetUint32(FlagGSeq); err != nil {
			return id, err
		}
	}
	return dv1.MakeGroupID(prev, gseq), nil
}

// AddDeploymentFilterFlags add flags to filter for deployment list
func AddDeploymentFilterFlags(flags *pflag.FlagSet) {
	// FEEDBACK(2026-07): --owner disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagOwner, "", "deployment owner address to filter")
	// FEEDBACK(2026-07): --state disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagState, "", "deployment state to filter (active,closed)")
	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint64(FlagDSeq, 0, "deployment sequence to filter")
	_ = flags
}

// DepFiltersFromFlags returns DeploymentFilters with given flags and error if occurred
func DepFiltersFromFlags(flags *pflag.FlagSet) (dv1beta.DeploymentFilters, error) {
	var dfilters dv1beta.DeploymentFilters

	// FEEDBACK(2026-07): the --owner/--state/--dseq filter flags are disabled
	// for the positional-only UX trial (see AddDeploymentFilterFlags), so this
	// returns empty filters and the positional filter argument is the only
	// source. Restore by uncommenting if the flags come back.
	// owner, err := flags.GetString(FlagOwner)
	// if err != nil {
	// 	return dfilters, err
	// }
	//
	// if owner != "" {
	// 	_, err = sdk.AccAddressFromBech32(owner)
	// 	if err != nil {
	// 		return dfilters, err
	// 	}
	// }
	//
	// dfilters.Owner = owner
	//
	// if dfilters.State, err = flags.GetString(FlagState); err != nil {
	// 	return dfilters, err
	// }
	//
	// if dfilters.DSeq, err = flags.GetUint64(FlagDSeq); err != nil {
	// 	return dfilters, err
	// }
	_ = flags

	return dfilters, nil
}

func AddDepositSourcesFlags(flags *pflag.FlagSet) {
	flags.StringSlice(FlagDepositSources, []string{"grant", "balance"}, "Comma separated list of deposit sources. allowed values grant|balance")
}

func AddDepositFlags(flags *pflag.FlagSet) {
	flags.String(FlagDeposit, "", "Deposit amount")
	flags.StringSlice(FlagDepositSources, []string{"grant", "balance"}, "Comma separated list of deposit sources. allowed values grant|balance")
}

// DepFiltersIsID returns true when the deployment filters specify a single
// deployment (owner and dseq are both set).
func DepFiltersIsID(f dv1beta.DeploymentFilters) bool {
	return f.Owner != "" && f.DSeq != 0
}

// DepFiltersFromArg parses a partial deployment path into DeploymentFilters.
//
// Smart type detection (SPEC §3.8.2):
//   - If the first component is a bech32 address, it is the owner.
//   - If the first component is a number, it is the dseq and defaultOwner is used.
//   - If the arg is a bare state keyword (active|closed), it is a state
//     filter. State keywords do not combine with identity paths inside one
//     argument; pass the state as the optional second positional instead.
//   - When the arg is a bare bech32 address with no "/", it lists all deployments for that owner.
//
// Format: [owner/]dseq  or  owner  or  state
func DepFiltersFromArg(arg string, defaultOwner string) (dv1beta.DeploymentFilters, error) {
	parts := strings.Split(arg, "/")
	var f dv1beta.DeploymentFilters

	if len(parts) < 1 || parts[0] == "" {
		return f, fmt.Errorf("deployment filter: argument is required")
	}

	// A bare state keyword as the sole argument selects a state filter (SPEC §3.8.2).
	if val, exists := dv1.Deployment_State_value[parts[0]]; exists && dv1.Deployment_State(val) != dv1.DeploymentStateInvalid {
		if len(parts) > 1 {
			return f, fmt.Errorf("deployment filter: state keyword %q cannot be combined with identity path %q; pass the state as a separate second argument instead", parts[0], arg)
		}
		f.State = parts[0]

		return f, nil
	}

	// Smart type detection on the first component.
	if _, err := sdk.AccAddressFromBech32(parts[0]); err == nil {
		// First component is a bech32 address.
		f.Owner = parts[0]

		if len(parts) >= 2 {
			dseq, err := parseSeq(parts[1], 64)
			if err != nil {
				return f, fmt.Errorf("deployment filter: invalid dseq %q: %w", parts[1], err)
			}
			f.DSeq = dseq
		}

		if len(parts) > 2 {
			return f, fmt.Errorf("deployment filter: too many parts in %q, expected owner[/dseq]", arg)
		}
	} else if dseq, err := strconv.ParseUint(parts[0], 10, 64); err == nil {
		// First component is a number — treat as dseq, use default owner.
		if defaultOwner == "" {
			return f, fmt.Errorf("deployment filter: no default account set; provide owner address or configure default-account")
		}
		f.Owner = defaultOwner
		f.DSeq = dseq

		if len(parts) > 1 {
			return f, fmt.Errorf("deployment filter: too many parts in %q when using dseq shorthand", arg)
		}
	} else {
		return f, fmt.Errorf("deployment filter: %q is not a valid address or dseq number", parts[0])
	}

	return f, nil
}

// DeploymentStateFromArg validates a positional deployment state keyword
// against the deployment state vocabulary (active|closed) — the same
// vocabulary DepFiltersFromArg accepts for the bare-keyword form. It backs
// the optional second positional state argument of `query deployment`
// (SPEC §3.8): `akt query deployment akash1owner/12345 active`.
func DeploymentStateFromArg(arg string) (string, error) {
	return stateFromArg("deployment", arg, dv1.Deployment_State_value)
}

// GroupIDFromArg parses a group path into a GroupID with smart type detection.
//
// Formats:
//   - owner/dseq[/gseq]  (explicit owner)
//   - dseq[/gseq]        (owner defaults to defaultOwner)
//
// When gseq is omitted it defaults to 0 (caller should treat as unset).
func GroupIDFromArg(arg string, defaultOwner string) (dv1.GroupID, bool, error) {
	parts := strings.Split(arg, "/")
	var id dv1.GroupID

	if len(parts) < 1 || parts[0] == "" {
		return id, false, fmt.Errorf("group ID: argument is required")
	}

	idx := 0

	// Smart type detection on the first component.
	if _, err := sdk.AccAddressFromBech32(parts[0]); err == nil {
		id.Owner = parts[0]
		idx = 1
		if len(parts) < 2 {
			return id, false, fmt.Errorf("group ID: expected at least owner/dseq, got %q", arg)
		}
	} else {
		// First component is not a bech32 — must be dseq with default owner.
		if defaultOwner == "" {
			return id, false, fmt.Errorf("group ID: no default account set; provide owner address or configure default-account")
		}
		id.Owner = defaultOwner
	}

	dseq, err := parseSeq(parts[idx], 64)
	if err != nil {
		return id, false, fmt.Errorf("group ID: invalid dseq %q: %w", parts[idx], err)
	}
	id.DSeq = dseq
	idx++

	fullySpecified := false
	if idx < len(parts) {
		gseq, err := parseSeq(parts[idx], 32)
		if err != nil {
			return id, false, fmt.Errorf("group ID: invalid gseq %q: %w", parts[idx], err)
		}
		id.GSeq = uint32(gseq)
		fullySpecified = true
		idx++
	}

	if idx < len(parts) {
		return id, false, fmt.Errorf("group ID: too many parts in %q", arg)
	}

	return id, fullySpecified, nil
}
