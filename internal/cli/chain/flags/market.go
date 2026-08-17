package flags

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	sdk "github.com/cosmos/cosmos-sdk/types"

	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

// AddOrderIDFlags add flags for order
func AddOrderIDFlags(flags *pflag.FlagSet, opts ...DeploymentIDOption) {
	AddGroupIDFlags(flags, opts...)
	flags.Uint32(flagdefs.FlagOSeq, 1, "Order Sequence")
}

// MarkReqOrderIDFlags marks flags required for order
func MarkReqOrderIDFlags(cmd *cobra.Command, opts ...DeploymentIDOption) {
	MarkReqGroupIDFlags(cmd, opts...)
}

// AddProviderFlag add provider flag to command flags set
func AddProviderFlag(flags *pflag.FlagSet) {
	flags.String(flagdefs.FlagProvider, "", "Provider")
}

// MarkReqProviderFlag marks provider flag as required
func MarkReqProviderFlag(cmd *cobra.Command) {
	_ = cmd.MarkFlagRequired(flagdefs.FlagProvider)
}

// OrderIDFromFlags returns OrderID with given flags and error if occurred
func OrderIDFromFlags(flags *pflag.FlagSet, opts ...MarketOption) (mv1.OrderID, error) {
	prev, err := GroupIDFromFlags(flags, opts...)
	if err != nil {
		return mv1.OrderID{}, err
	}
	val, err := flags.GetUint32(flagdefs.FlagOSeq)
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
		provider, err := flags.GetString(flagdefs.FlagProvider)
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
	// FEEDBACK(2026-07): --owner disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagOwner, "", "order owner address to filter")
	// FEEDBACK(2026-07): --state disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagState, "", "order state to filter (open,active,closed)")
	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint64(FlagDSeq, 0, "deployment sequence to filter")
	// FEEDBACK(2026-07): --gseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint32(FlagGSeq, 0, "group sequence to filter")
	// FEEDBACK(2026-07): --oseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint32(FlagOSeq, 0, "order sequence to filter")
	_ = flags
}

// OrderFiltersFromFlags returns OrderFilters with given flags and error if occurred
func OrderFiltersFromFlags(flags *pflag.FlagSet) (mvbeta.OrderFilters, error) {
	// FEEDBACK(2026-07): the order filter flags are disabled for the
	// positional-only UX trial (see AddOrderFilterFlags), so this returns
	// empty filters and the positional filter argument is the only source.
	// Restore by uncommenting if the flags come back.
	// dfilters, err := DepFiltersFromFlags(flags)
	// if err != nil {
	// 	return mvbeta.OrderFilters{}, err
	// }
	// ofilters := mvbeta.OrderFilters{
	// 	Owner: dfilters.Owner,
	// 	DSeq:  dfilters.DSeq,
	// 	State: dfilters.State,
	// }
	//
	// if ofilters.GSeq, err = flags.GetUint32(FlagGSeq); err != nil {
	// 	return ofilters, err
	// }
	//
	// if ofilters.OSeq, err = flags.GetUint32(FlagOSeq); err != nil {
	// 	return ofilters, err
	// }
	_ = flags

	return mvbeta.OrderFilters{}, nil
}

// AddBidFilterFlags add flags to filter for bid list
func AddBidFilterFlags(flags *pflag.FlagSet) {
	// FEEDBACK(2026-07): --owner disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagOwner, "", "bid owner address to filter")
	// FEEDBACK(2026-07): --state disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagState, "", "bid state to filter (open,active,lost,closed)")
	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint64(FlagDSeq, 0, "deployment sequence to filter")
	// FEEDBACK(2026-07): --gseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint32(FlagGSeq, 0, "group sequence to filter")
	// FEEDBACK(2026-07): --oseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint32(FlagOSeq, 0, "order sequence to filter")
	// FEEDBACK(2026-07): --provider disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagProvider, "", "bid provider address to filter")
	_ = flags
}

// BidFiltersFromFlags returns BidFilters with given flags and error if occurred
func BidFiltersFromFlags(flags *pflag.FlagSet) (mvbeta.BidFilters, error) {
	// FEEDBACK(2026-07): the bid filter flags are disabled for the
	// positional-only UX trial (see AddBidFilterFlags), so this returns
	// empty filters and the positional filter argument is the only source.
	// Restore by uncommenting if the flags come back.
	// ofilters, err := OrderFiltersFromFlags(flags)
	// if err != nil {
	// 	return mvbeta.BidFilters{}, err
	// }
	// bfilters := mvbeta.BidFilters{
	// 	Owner: ofilters.Owner,
	// 	DSeq:  ofilters.DSeq,
	// 	GSeq:  ofilters.GSeq,
	// 	OSeq:  ofilters.OSeq,
	// 	State: ofilters.State,
	// }
	//
	// provider, err := flags.GetString(FlagProvider)
	// if err != nil {
	// 	return bfilters, err
	// }
	//
	// if provider != "" {
	// 	_, err = sdk.AccAddressFromBech32(provider)
	// 	if err != nil {
	// 		return bfilters, err
	// 	}
	// }
	// bfilters.Provider = provider
	_ = flags

	return mvbeta.BidFilters{}, nil
}

// AddLeaseFilterFlags add flags to filter for a lease list
func AddLeaseFilterFlags(flags *pflag.FlagSet) {
	// FEEDBACK(2026-07): --owner disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagOwner, "", "lease owner address to filter")
	// FEEDBACK(2026-07): --state disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagState, "", "lease state to filter (active,insufficient_funds,closed)")
	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint64(FlagDSeq, 0, "deployment sequence to filter")
	// FEEDBACK(2026-07): --gseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint32(FlagGSeq, 0, "group sequence to filter")
	// FEEDBACK(2026-07): --oseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.Uint32(FlagOSeq, 0, "order sequence to filter")
	// FEEDBACK(2026-07): --provider disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// flags.String(FlagProvider, "", "bid provider address to filter")
	_ = flags
}

// LeaseFiltersFromFlags returns LeaseFilters with given flags and error if occurred.
// FEEDBACK(2026-07): the lease filter flags are disabled for the positional-only
// UX trial (see AddLeaseFilterFlags), so this returns empty filters via the
// (also gutted) BidFiltersFromFlags.
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
//   - If the arg is a bare state keyword (open|active|closed), it is a state
//     filter. State keywords do not combine with identity paths inside one
//     argument; pass the state as the optional second positional instead.
func OrderFiltersFromArg(arg string, defaultOwner string) (mvbeta.OrderFilters, error) {
	parts := strings.Split(arg, "/")
	var f mvbeta.OrderFilters

	if len(parts) < 1 || parts[0] == "" {
		return f, fmt.Errorf("order filter: argument is required")
	}

	// A bare state keyword as the sole argument selects a state filter (SPEC §3.8.2).
	if val, exists := mvbeta.Order_State_value[parts[0]]; exists && mvbeta.Order_State(val) != mvbeta.OrderStateInvalid {
		if len(parts) > 1 {
			return f, fmt.Errorf("order filter: state keyword %q cannot be combined with identity path %q; pass the state as a separate second argument instead", parts[0], arg)
		}
		f.State = parts[0]

		return f, nil
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
		dseq, err := parseSeq(parts[idx], 64)
		if err != nil {
			return f, fmt.Errorf("order filter: invalid dseq %q: %w", parts[idx], err)
		}
		f.DSeq = dseq
		idx++
	}

	if idx < len(parts) {
		gseq, err := parseSeq(parts[idx], 32)
		if err != nil {
			return f, fmt.Errorf("order filter: invalid gseq %q: %w", parts[idx], err)
		}
		f.GSeq = uint32(gseq)
		idx++
	}

	if idx < len(parts) {
		oseq, err := parseSeq(parts[idx], 32)
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
//
// A bare state keyword (open|active|lost|closed) as the sole argument is a
// state filter. State keywords do not combine with identity paths inside one
// argument; pass the state as the optional second positional instead.
// BidFiltersFromArg parses the shared [owner/]dseq[/gseq[/oseq[/provider]]]
// grammar for bids.
func BidFiltersFromArg(arg string, defaultOwner string, byProvider bool) (mvbeta.BidFilters, error) {
	return bidShapedFiltersFromArg(arg, defaultOwner, byProvider, "bid filter")
}

// bidShapedFiltersFromArg is the parser behind both the bid and lease filters.
// resource names the caller's resource: lease queries delegate here, and every
// diagnostic used to say "bid filter" regardless of which command was run.
func bidShapedFiltersFromArg(arg string, defaultOwner string, byProvider bool, resource string) (mvbeta.BidFilters, error) {
	parts := strings.Split(arg, "/")
	var f mvbeta.BidFilters

	if len(parts) < 1 || parts[0] == "" {
		return f, fmt.Errorf("%s: argument is required", resource)
	}

	// A bare state keyword as the sole argument selects a state filter (SPEC §3.8.2).
	if val, exists := mvbeta.Bid_State_value[parts[0]]; exists && mvbeta.Bid_State(val) != mvbeta.BidStateInvalid {
		if len(parts) > 1 {
			return f, fmt.Errorf("%s: state keyword %q cannot be combined with identity path %q; pass the state as a separate second argument instead", resource, parts[0], arg)
		}
		f.State = parts[0]

		return f, nil
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
			return f, fmt.Errorf("%s: provider address is required with --by provider", resource)
		}
		if defaultOwner == "" {
			return f, fmt.Errorf("%s: no default account set; provide owner address or configure default-account", resource)
		}
		f.Owner = defaultOwner
	} else {
		return f, fmt.Errorf("%s: %q is not a valid address or dseq number", resource, parts[0])
	}

	if idx < len(parts) {
		dseq, err := parseSeq(parts[idx], 64)
		if err != nil {
			return f, fmt.Errorf("%s: invalid dseq %q: %w", resource, parts[idx], err)
		}
		f.DSeq = dseq
		idx++
	}

	if idx < len(parts) {
		gseq, err := parseSeq(parts[idx], 32)
		if err != nil {
			return f, fmt.Errorf("%s: invalid gseq %q: %w", resource, parts[idx], err)
		}
		f.GSeq = uint32(gseq)
		idx++
	}

	if idx < len(parts) {
		oseq, err := parseSeq(parts[idx], 32)
		if err != nil {
			return f, fmt.Errorf("%s: invalid oseq %q: %w", resource, parts[idx], err)
		}
		f.OSeq = uint32(oseq)
		idx++
	}

	// Trailing address (opposite of the leading one).
	if idx < len(parts) {
		if _, err := sdk.AccAddressFromBech32(parts[idx]); err != nil {
			return f, fmt.Errorf("%s: invalid address %q: %w", resource, parts[idx], err)
		}
		if byProvider {
			f.Owner = parts[idx]
		} else {
			f.Provider = parts[idx]
		}
		idx++
	}

	if idx < len(parts) {
		return f, fmt.Errorf("%s: too many parts in %q", resource, arg)
	}

	return f, nil
}

// LeaseFiltersFromArg parses a partial lease path into LeaseFilters.
// See BidFiltersFromArg for format details.
//
// A bare state keyword (active|insufficient_funds|closed) as the sole argument
// is a state filter. The lease state vocabulary differs
// from the bid vocabulary, so it is handled here before delegating identity
// parsing to BidFiltersFromArg.
func LeaseFiltersFromArg(arg string, defaultOwner string, byProvider bool) (mv1.LeaseFilters, error) {
	parts := strings.Split(arg, "/")

	// A bare state keyword as the sole argument selects a state filter (SPEC §3.8.2).
	if val, exists := mv1.Lease_State_value[parts[0]]; exists && mv1.Lease_State(val) != mv1.LeaseStateInvalid {
		if len(parts) > 1 {
			return mv1.LeaseFilters{}, fmt.Errorf("lease filter: state keyword %q cannot be combined with identity path %q; pass the state as a separate second argument instead", parts[0], arg)
		}

		return mv1.LeaseFilters{State: parts[0]}, nil
	}

	bf, err := bidShapedFiltersFromArg(arg, defaultOwner, byProvider, "lease filter")
	if err != nil {
		return mv1.LeaseFilters{}, err
	}

	// BidFiltersFromArg recognizes bid state keywords; reject those that are
	// not valid lease states (e.g. "open", "lost").
	if bf.State != "" {
		if _, exists := mv1.Lease_State_value[bf.State]; !exists {
			return mv1.LeaseFilters{}, fmt.Errorf("lease filter: %q is not a valid lease state (%s)", bf.State, stateKeywords(mv1.Lease_State_value))
		}
	}

	return mv1.LeaseFilters(bf), nil
}

// OrderStateFromArg validates a positional order state keyword against the
// order state vocabulary (open|active|closed). It backs the optional second
// positional state argument of `query market order` (SPEC §3.8).
func OrderStateFromArg(arg string) (string, error) {
	return stateFromArg("order", arg, mvbeta.Order_State_value)
}

// BidStateFromArg validates a positional bid state keyword against the bid
// state vocabulary (open|active|lost|closed). It backs the optional second
// positional state argument of `query market bid` (SPEC §3.8).
func BidStateFromArg(arg string) (string, error) {
	return stateFromArg("bid", arg, mvbeta.Bid_State_value)
}

// LeaseStateFromArg validates a positional lease state keyword against the
// lease state vocabulary (active|insufficient_funds|closed). It backs the
// optional second positional state argument of `query market lease`
// (SPEC §3.8).
func LeaseStateFromArg(arg string) (string, error) {
	return stateFromArg("lease", arg, mv1.Lease_State_value)
}

// stateFromArg validates a positional state keyword against a resource's
// protobuf State_value vocabulary — the same check the *FiltersFromArg
// parsers apply to a bare-keyword first argument. The zero enum value is the
// per-resource "invalid" placeholder and is never a valid keyword.
func stateFromArg(resource, arg string, values map[string]int32) (string, error) {
	if val, exists := values[arg]; exists && val != 0 {
		return arg, nil
	}

	return "", fmt.Errorf("%s filter: %q is not a valid state (%s)", resource, arg, stateKeywords(values))
}

// stateKeywords returns the valid state keywords of a protobuf State_value
// enum map joined with "|", ordered by enum value and excluding the zero
// (invalid) placeholder.
func stateKeywords(values map[string]int32) string {
	names := make([]string, 0, len(values))
	for name, val := range values {
		if val == 0 {
			continue
		}
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool { return values[names[i]] < values[names[j]] })

	return strings.Join(names, "|")
}

// AddBidClosedReasonFlag add the reason flag when the provider initiates lease close
func AddBidClosedReasonFlag(flags *pflag.FlagSet) {
	flags.Int32(flagdefs.FlagClosedReason, int32(mv1.LeaseClosedReasonUnspecified), "Numeric reason for closing the bid (10000=unstable, 10001=decommission, 10002=unspecified, 10003=manifest_timeout)")
}

// BidClosedReasonFromFlags returns LeaseClosedReason from flags or returns the default value if not set
func BidClosedReasonFromFlags(flags *pflag.FlagSet) (mv1.LeaseClosedReason, error) {
	val, err := flags.GetInt32(flagdefs.FlagClosedReason)
	if err != nil {
		return mv1.LeaseClosedReasonInvalid, err
	}

	reason := mv1.LeaseClosedReason(val)
	if !reason.IsRange(mv1.LeaseClosedReasonRangeProvider) {
		return mv1.LeaseClosedReasonInvalid, fmt.Errorf("invalid --reason value. expected range 10000..19999")
	}

	return reason, nil
}
