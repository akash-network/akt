package flags

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	sdk "github.com/cosmos/cosmos-sdk/types"

	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

// AddOrderIDFlags add flags for order
func AddOrderIDFlags(flags *pflag.FlagSet, opts ...DeploymentIDOption) {
	AddGroupIDFlags(flags, opts...)
	flags.Uint32(FlagOSeq, 1, "Order Sequence")
}

// MarkReqOrderIDFlags marks flags required for order
func MarkReqOrderIDFlags(cmd *cobra.Command, opts ...DeploymentIDOption) {
	MarkReqGroupIDFlags(cmd, opts...)
}

// AddProviderFlag add provider flag to command flags set
func AddProviderFlag(flags *pflag.FlagSet) {
	flags.String(FlagProvider, "", "Provider")
}

// MarkReqProviderFlag marks provider flag as required
func MarkReqProviderFlag(cmd *cobra.Command) {
	_ = cmd.MarkFlagRequired(FlagProvider)
}

// OrderIDFromFlags returns OrderID with given flags and error if occurred
func OrderIDFromFlags(flags *pflag.FlagSet, opts ...MarketOption) (mv1.OrderID, error) {
	prev, err := GroupIDFromFlags(flags, opts...)
	if err != nil {
		return mv1.OrderID{}, err
	}
	val, err := flags.GetUint32(FlagOSeq)
	if err != nil {
		return mv1.OrderID{}, err
	}
	return mv1.MakeOrderID(prev, val), nil
}

// AddBidIDFlags add flags for bid
func AddBidIDFlags(flags *pflag.FlagSet, opts ...DeploymentIDOption) {
	AddOrderIDFlags(flags, opts...)
	AddProviderFlag(flags)
}

// AddQueryBidIDFlags add flags for bid in query commands
func AddQueryBidIDFlags(flags *pflag.FlagSet) {
	AddBidIDFlags(flags)
}

// MarkReqBidIDFlags marks flags required for bid
// Used in get bid query command
func MarkReqBidIDFlags(cmd *cobra.Command, opts ...DeploymentIDOption) {
	MarkReqOrderIDFlags(cmd, opts...)
	MarkReqProviderFlag(cmd)
}

// BidIDFromFlags returns BidID with given flags and error if occurred
// Here provider value is taken from flags
func BidIDFromFlags(flags *pflag.FlagSet, opts ...MarketOption) (mv1.BidID, error) {
	prev, err := OrderIDFromFlags(flags, opts...)
	if err != nil {
		return mv1.BidID{}, err
	}

	opt := &MarketOptions{}

	for _, o := range opts {
		o(opt)
	}

	if opt.Provider.Empty() {
		provider, err := flags.GetString(FlagProvider)
		if err != nil {
			return mv1.BidID{}, err
		}

		if opt.Provider, err = sdk.AccAddressFromBech32(provider); err != nil {
			return mv1.BidID{}, err
		}
	}

	return mv1.MakeBidID(prev, opt.Provider), nil
}

func AddLeaseIDFlags(flags *pflag.FlagSet, opts ...DeploymentIDOption) {
	AddBidIDFlags(flags, opts...)
}

// MarkReqLeaseIDFlags marks flags required for bid
// Used in get bid query command
func MarkReqLeaseIDFlags(cmd *cobra.Command, opts ...DeploymentIDOption) {
	MarkReqBidIDFlags(cmd, opts...)
}

// LeaseIDFromFlags returns LeaseID with given flags and error if occurred
// Here provider value is taken from flags
func LeaseIDFromFlags(flags *pflag.FlagSet, opts ...MarketOption) (mv1.LeaseID, error) {
	bid, err := BidIDFromFlags(flags, opts...)
	if err != nil {
		return mv1.LeaseID{}, err
	}

	return bid.LeaseID(), nil
}

// AddOrderFilterFlags add flags to filter for order list
func AddOrderFilterFlags(flags *pflag.FlagSet) {
	flags.String(FlagOwner, "", "order owner address to filter")
	flags.String(FlagState, "", "order state to filter (open,matched,closed)")
	flags.Uint64(FlagDSeq, 0, "deployment sequence to filter")
	flags.Uint32(FlagGSeq, 0, "group sequence to filter")
	flags.Uint32(FlagOSeq, 0, "order sequence to filter")
}

// OrderFiltersFromFlags returns OrderFilters with given flags and error if occurred
func OrderFiltersFromFlags(flags *pflag.FlagSet) (mvbeta.OrderFilters, error) {
	dfilters, err := DepFiltersFromFlags(flags)
	if err != nil {
		return mvbeta.OrderFilters{}, err
	}
	ofilters := mvbeta.OrderFilters{
		Owner: dfilters.Owner,
		DSeq:  dfilters.DSeq,
		State: dfilters.State,
	}

	if ofilters.GSeq, err = flags.GetUint32(FlagGSeq); err != nil {
		return ofilters, err
	}

	if ofilters.OSeq, err = flags.GetUint32(FlagOSeq); err != nil {
		return ofilters, err
	}

	return ofilters, nil
}

// AddBidFilterFlags add flags to filter for bid list
func AddBidFilterFlags(flags *pflag.FlagSet) {
	flags.String(FlagOwner, "", "bid owner address to filter")
	flags.String(FlagState, "", "bid state to filter (open,matched,lost,closed)")
	flags.Uint64(FlagDSeq, 0, "deployment sequence to filter")
	flags.Uint32(FlagGSeq, 0, "group sequence to filter")
	flags.Uint32(FlagOSeq, 0, "order sequence to filter")
	flags.String(FlagProvider, "", "bid provider address to filter")
}

// BidFiltersFromFlags returns BidFilters with given flags and error if occurred
func BidFiltersFromFlags(flags *pflag.FlagSet) (mvbeta.BidFilters, error) {
	ofilters, err := OrderFiltersFromFlags(flags)
	if err != nil {
		return mvbeta.BidFilters{}, err
	}
	bfilters := mvbeta.BidFilters{
		Owner: ofilters.Owner,
		DSeq:  ofilters.DSeq,
		GSeq:  ofilters.GSeq,
		OSeq:  ofilters.OSeq,
		State: ofilters.State,
	}

	provider, err := flags.GetString(FlagProvider)
	if err != nil {
		return bfilters, err
	}

	if provider != "" {
		_, err = sdk.AccAddressFromBech32(provider)
		if err != nil {
			return bfilters, err
		}
	}
	bfilters.Provider = provider

	return bfilters, nil
}

// AddLeaseFilterFlags add flags to filter for a lease list
func AddLeaseFilterFlags(flags *pflag.FlagSet) {
	flags.String(FlagOwner, "", "lease owner address to filter")
	flags.String(FlagState, "", "lease state to filter (active,insufficient_funds,closed)")
	flags.Uint64(FlagDSeq, 0, "deployment sequence to filter")
	flags.Uint32(FlagGSeq, 0, "group sequence to filter")
	flags.Uint32(FlagOSeq, 0, "order sequence to filter")
	flags.String(FlagProvider, "", "bid provider address to filter")
}

// LeaseFiltersFromFlags returns LeaseFilters with given flags and error if occurred
func LeaseFiltersFromFlags(flags *pflag.FlagSet) (mv1.LeaseFilters, error) {
	bfilters, err := BidFiltersFromFlags(flags)
	if err != nil {
		return mv1.LeaseFilters{}, err
	}
	return mv1.LeaseFilters(bfilters), nil
}

// OrderFiltersIsID returns true when all ID fields in the order filters are set,
// indicating the user wants a single record rather than a filtered list.
func OrderFiltersIsID(f mvbeta.OrderFilters) bool {
	return f.Owner != "" && f.DSeq != 0 && f.GSeq != 0 && f.OSeq != 0
}

// BidFiltersIsID returns true when all ID fields in the bid filters are set,
// indicating the user wants a single record rather than a filtered list.
func BidFiltersIsID(f mvbeta.BidFilters) bool {
	return f.Owner != "" && f.DSeq != 0 && f.GSeq != 0 && f.OSeq != 0 && f.Provider != ""
}

// LeaseFiltersIsID returns true when all ID fields in the lease filters are set,
// indicating the user wants a single record rather than a filtered list.
func LeaseFiltersIsID(f mv1.LeaseFilters) bool {
	return BidFiltersIsID(mvbeta.BidFilters(f))
}

// OrderFiltersFromArg parses a partial order path (owner[/dseq[/gseq[/oseq]]])
// into OrderFilters. Only the parts present in the input are populated.
func OrderFiltersFromArg(arg string) (mvbeta.OrderFilters, error) {
	parts := strings.Split(arg, "/")
	var f mvbeta.OrderFilters

	if len(parts) < 1 || parts[0] == "" {
		return f, fmt.Errorf("order filter: owner is required")
	}

	if _, err := sdk.AccAddressFromBech32(parts[0]); err != nil {
		return f, fmt.Errorf("order filter: invalid owner address: %w", err)
	}
	f.Owner = parts[0]

	if len(parts) >= 2 {
		dseq, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return f, fmt.Errorf("order filter: invalid dseq: %w", err)
		}
		f.DSeq = dseq
	}

	if len(parts) >= 3 {
		gseq, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil {
			return f, fmt.Errorf("order filter: invalid gseq: %w", err)
		}
		f.GSeq = uint32(gseq)
	}

	if len(parts) >= 4 {
		oseq, err := strconv.ParseUint(parts[3], 10, 32)
		if err != nil {
			return f, fmt.Errorf("order filter: invalid oseq: %w", err)
		}
		f.OSeq = uint32(oseq)
	}

	if len(parts) > 4 {
		return f, fmt.Errorf("order filter: too many parts in %q, expected owner/dseq/gseq/oseq", arg)
	}

	return f, nil
}

// BidFiltersFromArg parses a partial bid path (owner[/dseq[/gseq[/oseq[/provider]]]])
// into BidFilters. Only the parts present in the input are populated.
func BidFiltersFromArg(arg string) (mvbeta.BidFilters, error) {
	parts := strings.Split(arg, "/")

	// Parse the order portion (up to 4 parts).
	orderArg := arg
	if len(parts) > 4 {
		orderArg = strings.Join(parts[:4], "/")
	}
	of, err := OrderFiltersFromArg(orderArg)
	if err != nil {
		return mvbeta.BidFilters{}, err
	}

	f := mvbeta.BidFilters{
		Owner: of.Owner,
		DSeq:  of.DSeq,
		GSeq:  of.GSeq,
		OSeq:  of.OSeq,
		State: of.State,
	}

	if len(parts) >= 5 {
		if _, err := sdk.AccAddressFromBech32(parts[4]); err != nil {
			return f, fmt.Errorf("bid filter: invalid provider address: %w", err)
		}
		f.Provider = parts[4]
	}

	if len(parts) > 5 {
		return f, fmt.Errorf("bid filter: too many parts in %q, expected owner/dseq/gseq/oseq/provider", arg)
	}

	return f, nil
}

// LeaseFiltersFromArg parses a partial lease path into LeaseFilters.
// The format is owner[/dseq[/gseq[/oseq[/provider]]]].
func LeaseFiltersFromArg(arg string) (mv1.LeaseFilters, error) {
	bf, err := BidFiltersFromArg(arg)
	if err != nil {
		return mv1.LeaseFilters{}, err
	}
	return mv1.LeaseFilters(bf), nil
}

// AddBidClosedReasonFlag add the reason flag when the provider initiates lease close
func AddBidClosedReasonFlag(flags *pflag.FlagSet) {
	flags.Int32(FlagClosedReason, int32(mv1.LeaseClosedReasonUnspecified), "Numeric reason for closing the bid (10000=unstable, 10001=decommission, 10002=unspecified, 10003=manifest_timeout)")
}

// BidClosedReasonFromFlags returns LeaseClosedReason from flags or returns the default value if not set
func BidClosedReasonFromFlags(flags *pflag.FlagSet) (mv1.LeaseClosedReason, error) {
	val, err := flags.GetInt32(FlagClosedReason)
	if err != nil {
		return mv1.LeaseClosedReasonInvalid, err
	}

	reason := mv1.LeaseClosedReason(val)
	if !reason.IsRange(mv1.LeaseClosedReasonRangeProvider) {
		return mv1.LeaseClosedReasonInvalid, fmt.Errorf("invalid --reason value. expected range 10000..19999")
	}

	return reason, nil
}
