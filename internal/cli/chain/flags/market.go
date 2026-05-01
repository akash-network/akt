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

// OrderFiltersFromArg parses a partial order path with smart type detection.
//
// Formats (SPEC §3.8):
//   - [owner/]dseq[/gseq[/oseq]]
//   - If the first component is a number, it is dseq and defaultOwner is used.
//   - If the first component is a bech32 address, it is the owner.
func OrderFiltersFromArg(arg string, defaultOwner string) (mvbeta.OrderFilters, error) {
	parts := strings.Split(arg, "/")
	var f mvbeta.OrderFilters

	if len(parts) < 1 || parts[0] == "" {
		return f, fmt.Errorf("order filter: argument is required")
	}

	idx := 0

	// Smart type detection on the first component.
	if _, err := sdk.AccAddressFromBech32(parts[0]); err == nil {
		f.Owner = parts[0]
		idx = 1
	} else {
		if defaultOwner == "" {
			return f, fmt.Errorf("order filter: no default account set; provide owner address or configure default-account")
		}
		f.Owner = defaultOwner
	}

	if idx < len(parts) {
		dseq, err := strconv.ParseUint(parts[idx], 10, 64)
		if err != nil {
			return f, fmt.Errorf("order filter: invalid dseq %q: %w", parts[idx], err)
		}
		f.DSeq = dseq
		idx++
	}

	if idx < len(parts) {
		gseq, err := strconv.ParseUint(parts[idx], 10, 32)
		if err != nil {
			return f, fmt.Errorf("order filter: invalid gseq %q: %w", parts[idx], err)
		}
		f.GSeq = uint32(gseq)
		idx++
	}

	if idx < len(parts) {
		oseq, err := strconv.ParseUint(parts[idx], 10, 32)
		if err != nil {
			return f, fmt.Errorf("order filter: invalid oseq %q: %w", parts[idx], err)
		}
		f.OSeq = uint32(oseq)
		idx++
	}

	if idx < len(parts) {
		return f, fmt.Errorf("order filter: too many parts in %q", arg)
	}

	return f, nil
}

// BidFiltersFromArg parses a partial bid/lease path with smart type detection.
//
// Owner perspective (default): [owner/]dseq[/gseq[/oseq[/provider]]]
// Provider perspective (--by provider): [provider/]dseq[/gseq[/oseq[/owner]]]
//
// When byProvider is true, the leading address is the provider and the trailing
// address is the owner. Otherwise the leading address is the owner and the
// trailing address is the provider.
func BidFiltersFromArg(arg string, defaultOwner string, byProvider bool) (mvbeta.BidFilters, error) {
	parts := strings.Split(arg, "/")
	var f mvbeta.BidFilters

	if len(parts) < 1 || parts[0] == "" {
		return f, fmt.Errorf("bid filter: argument is required")
	}

	idx := 0

	// Smart type detection on the first component.
	if _, err := sdk.AccAddressFromBech32(parts[0]); err == nil {
		if byProvider {
			f.Provider = parts[0]
		} else {
			f.Owner = parts[0]
		}
		idx = 1
	} else if _, err := strconv.ParseUint(parts[0], 10, 64); err == nil {
		// First component is a number — use default for the leading address.
		if byProvider {
			return f, fmt.Errorf("bid filter: provider address is required with --by provider")
		}
		if defaultOwner == "" {
			return f, fmt.Errorf("bid filter: no default account set; provide owner address or configure default-account")
		}
		f.Owner = defaultOwner
	} else {
		return f, fmt.Errorf("bid filter: %q is not a valid address or dseq number", parts[0])
	}

	if idx < len(parts) {
		dseq, err := strconv.ParseUint(parts[idx], 10, 64)
		if err != nil {
			return f, fmt.Errorf("bid filter: invalid dseq %q: %w", parts[idx], err)
		}
		f.DSeq = dseq
		idx++
	}

	if idx < len(parts) {
		gseq, err := strconv.ParseUint(parts[idx], 10, 32)
		if err != nil {
			return f, fmt.Errorf("bid filter: invalid gseq %q: %w", parts[idx], err)
		}
		f.GSeq = uint32(gseq)
		idx++
	}

	if idx < len(parts) {
		oseq, err := strconv.ParseUint(parts[idx], 10, 32)
		if err != nil {
			return f, fmt.Errorf("bid filter: invalid oseq %q: %w", parts[idx], err)
		}
		f.OSeq = uint32(oseq)
		idx++
	}

	// Trailing address (opposite of the leading one).
	if idx < len(parts) {
		if _, err := sdk.AccAddressFromBech32(parts[idx]); err != nil {
			return f, fmt.Errorf("bid filter: invalid address %q: %w", parts[idx], err)
		}
		if byProvider {
			f.Owner = parts[idx]
		} else {
			f.Provider = parts[idx]
		}
		idx++
	}

	if idx < len(parts) {
		return f, fmt.Errorf("bid filter: too many parts in %q", arg)
	}

	return f, nil
}

// LeaseFiltersFromArg parses a partial lease path into LeaseFilters.
// See BidFiltersFromArg for format details.
func LeaseFiltersFromArg(arg string, defaultOwner string, byProvider bool) (mv1.LeaseFilters, error) {
	bf, err := BidFiltersFromArg(arg, defaultOwner, byProvider)
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
