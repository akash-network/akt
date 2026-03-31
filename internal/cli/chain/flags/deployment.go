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

	if id.DSeq, err = flags.GetUint64(FlagDSeq); err != nil {
		return id, err
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

	gseq, err := flags.GetUint32(FlagGSeq)
	if err != nil {
		return id, err
	}
	return dv1.MakeGroupID(prev, gseq), nil
}

// AddDeploymentFilterFlags add flags to filter for deployment list
func AddDeploymentFilterFlags(flags *pflag.FlagSet) {
	flags.String(FlagOwner, "", "deployment owner address to filter")
	flags.String(FlagState, "", "deployment state to filter (active,closed)")
	flags.Uint64(FlagDSeq, 0, "deployment sequence to filter")
}

// DepFiltersFromFlags returns DeploymentFilters with given flags and error if occurred
func DepFiltersFromFlags(flags *pflag.FlagSet) (dv1beta.DeploymentFilters, error) {
	var dfilters dv1beta.DeploymentFilters
	owner, err := flags.GetString(FlagOwner)
	if err != nil {
		return dfilters, err
	}

	if owner != "" {
		_, err = sdk.AccAddressFromBech32(owner)
		if err != nil {
			return dfilters, err
		}
	}

	dfilters.Owner = owner

	if dfilters.State, err = flags.GetString(FlagState); err != nil {
		return dfilters, err
	}

	if dfilters.DSeq, err = flags.GetUint64(FlagDSeq); err != nil {
		return dfilters, err
	}

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

// DepFiltersFromArg parses a partial deployment path (owner[/dseq])
// into DeploymentFilters. Only the parts present in the input are populated.
func DepFiltersFromArg(arg string) (dv1beta.DeploymentFilters, error) {
	parts := strings.Split(arg, "/")
	var f dv1beta.DeploymentFilters

	if len(parts) < 1 || parts[0] == "" {
		return f, fmt.Errorf("deployment filter: owner is required")
	}

	if _, err := sdk.AccAddressFromBech32(parts[0]); err != nil {
		return f, fmt.Errorf("deployment filter: invalid owner address: %w", err)
	}
	f.Owner = parts[0]

	if len(parts) >= 2 {
		dseq, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return f, fmt.Errorf("deployment filter: invalid dseq: %w", err)
		}
		f.DSeq = dseq
	}

	if len(parts) > 2 {
		return f, fmt.Errorf("deployment filter: too many parts in %q, expected owner[/dseq]", arg)
	}

	return f, nil
}

// GroupIDFromArg parses a group path (owner/dseq[/gseq]) into a GroupID.
// When gseq is omitted it defaults to 0 (caller should treat as unset).
func GroupIDFromArg(arg string) (dv1.GroupID, bool, error) {
	parts := strings.Split(arg, "/")
	var id dv1.GroupID

	if len(parts) < 2 {
		return id, false, fmt.Errorf("group ID: expected at least owner/dseq, got %q", arg)
	}

	if _, err := sdk.AccAddressFromBech32(parts[0]); err != nil {
		return id, false, fmt.Errorf("group ID: invalid owner address: %w", err)
	}
	id.Owner = parts[0]

	dseq, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return id, false, fmt.Errorf("group ID: invalid dseq: %w", err)
	}
	id.DSeq = dseq

	fullySpecified := false
	if len(parts) >= 3 {
		gseq, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil {
			return id, false, fmt.Errorf("group ID: invalid gseq: %w", err)
		}
		id.GSeq = uint32(gseq)
		fullySpecified = true
	}

	if len(parts) > 3 {
		return id, false, fmt.Errorf("group ID: too many parts in %q, expected owner/dseq[/gseq]", arg)
	}

	return id, fullySpecified, nil
}
