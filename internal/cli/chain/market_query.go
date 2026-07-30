package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

// GetQueryMarketCmds returns the transaction commands for the market module
func GetQueryMarketCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        mv1.ModuleName,
		Short:                      "Market query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetQueryMarketOrderCmd(),
		GetQueryMarketBidCmd(),
		GetQueryMarketLeaseCmd(),
		GetQueryMarketParamsCmd(),
	)

	return cmd
}

// GetQueryMarketOrderCmd returns the command to query orders.
// Accepts an optional positional ID in the form owner[/dseq[/gseq[/oseq]]].
// When all ID fields are present returns a single order.
// When partially specified, returns a filtered list.
func GetQueryMarketOrderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "order [id] [state]",
		Short: "Query orders",
		Long: `Query orders.

The optional [id] argument is [owner/]dseq[/gseq[/oseq]], a bare owner
address, or a bare state keyword (open|active|closed) per SPEC §3.8.

The optional [state] argument narrows the result: with a partial identity it
filters the list; when the identity pins down a single order it verifies the
record instead — the command fails if the order is in a different state
rather than printing it.`,
		Args:              cobra.MaximumNArgs(2),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			ofilters, err := cflags.OrderFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			defaultOwner := cl.ClientContext().GetFromAddress().String()

			if len(args) > 0 {
				af, err := cflags.OrderFiltersFromArg(args[0], defaultOwner)
				if err != nil {
					return err
				}
				if af.Owner != "" {
					ofilters.Owner = af.Owner
				}
				if af.DSeq != 0 {
					ofilters.DSeq = af.DSeq
				}
				if af.GSeq != 0 {
					ofilters.GSeq = af.GSeq
				}
				if af.OSeq != 0 {
					ofilters.OSeq = af.OSeq
				}
				// A bare state keyword may only appear once (SPEC §3.8.2).
				if af.State != "" {
					if len(args) > 1 {
						return fmt.Errorf("order filter: state keyword %q cannot be combined with a second argument %q", args[0], args[1])
					}
					ofilters.State = af.State
				}
			}

			// Optional second positional: a state keyword narrowing the
			// identity filter (SPEC §3.8), e.g.
			// `akt query market order 12345 open`.
			if len(args) > 1 {
				if ofilters.State, err = cflags.OrderStateFromArg(args[1]); err != nil {
					return err
				}
			}

			// Default owner fallback when no arg and no --owner flag.
			if ofilters.Owner == "" {
				if defaultOwner == "" {
					return requireOwnerScope("order filter")
				}
				ofilters.Owner = defaultOwner
			}

			if cflags.OrderFiltersIsID(ofilters) {
				id := mv1.OrderID{
					Owner: ofilters.Owner,
					DSeq:  ofilters.DSeq,
					GSeq:  ofilters.GSeq,
					OSeq:  ofilters.OSeq,
				}

				res, err := cl.Query().Market().Order(ctx, &mvbeta.QueryOrderRequest{ID: id})
				if err != nil {
					return err
				}

				// SPEC §3.8.3: on the get path the positional [state] is a
				// verification, not a filter — never silently ignore it.
				if err := requireStateMatch("order", fmt.Sprintf("%s/%d/%d/%d", id.Owner, id.DSeq, id.GSeq, id.OSeq),
					ofilters.State, res.Order.State.String()); err != nil {
					return err
				}

				return pretty.PrintQueryResult(cmd, cl.ClientContext(), &res.Order)
			}

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := cl.Query().Market().Orders(ctx, &mvbeta.QueryOrdersRequest{
				Filters:    ofilters,
				Pagination: pageReq,
			})
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "orders")
	cflags.AddOrderFilterFlags(cmd.Flags())

	return cmd
}

// GetQueryMarketBidCmd returns the command to query bids.
// Accepts an optional positional ID in the form [owner/]dseq[/gseq[/oseq[/provider]]].
// With --by provider: [provider/]dseq[/gseq[/oseq[/owner]]].
// When all ID fields are present returns a single bid.
// When partially specified, returns a filtered list.
func GetQueryMarketBidCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid [id] [state]",
		Short: "Query bids",
		Long: `Query bids.

The optional [id] argument is [owner/]dseq[/gseq[/oseq[/provider]]] (with
--by provider: [provider/]dseq[/gseq[/oseq[/owner]]]), a bare address, or a
bare state keyword (open|active|lost|closed) per SPEC §3.8.

The optional [state] argument narrows the result: with a partial identity it
filters the list; when the identity pins down a single bid it verifies the
record instead — the command fails if the bid is in a different state rather
than printing it.`,
		Args:              cobra.MaximumNArgs(2),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			bfilters, err := cflags.BidFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			defaultOwner := cl.ClientContext().GetFromAddress().String()
			byProvider, _ := cmd.Flags().GetString("by")
			isByProvider := byProvider == "provider"

			if len(args) > 0 {
				af, err := cflags.BidFiltersFromArg(args[0], defaultOwner, isByProvider)
				if err != nil {
					return err
				}
				if af.Owner != "" {
					bfilters.Owner = af.Owner
				}
				if af.DSeq != 0 {
					bfilters.DSeq = af.DSeq
				}
				if af.GSeq != 0 {
					bfilters.GSeq = af.GSeq
				}
				if af.OSeq != 0 {
					bfilters.OSeq = af.OSeq
				}
				if af.Provider != "" {
					bfilters.Provider = af.Provider
				}
				// A bare state keyword may only appear once (SPEC §3.8.2).
				if af.State != "" {
					if len(args) > 1 {
						return fmt.Errorf("bid filter: state keyword %q cannot be combined with a second argument %q", args[0], args[1])
					}
					bfilters.State = af.State
				}
			}

			// Optional second positional: a state keyword narrowing the
			// identity filter (SPEC §3.8), e.g.
			// `akt query market bid 12345 open`.
			if len(args) > 1 {
				if bfilters.State, err = cflags.BidStateFromArg(args[1]); err != nil {
					return err
				}
			}

			// Default owner fallback when no arg and no --owner flag (owner mode only).
			if isByProvider {
				if bfilters.Provider == "" {
					return requireProviderScope("bid filter")
				}
			} else if bfilters.Owner == "" {
				if defaultOwner == "" {
					return requireOwnerScope("bid filter")
				}
				bfilters.Owner = defaultOwner
			}

			if cflags.BidFiltersIsID(bfilters) {
				id := mv1.BidID{
					Owner:    bfilters.Owner,
					DSeq:     bfilters.DSeq,
					GSeq:     bfilters.GSeq,
					OSeq:     bfilters.OSeq,
					Provider: bfilters.Provider,
				}

				res, err := cl.Query().Market().Bid(ctx, &mvbeta.QueryBidRequest{ID: id})
				if err != nil {
					return err
				}

				// SPEC §3.8.3: on the get path the positional [state] is a
				// verification, not a filter — never silently ignore it.
				if err := requireStateMatch("bid", fmt.Sprintf("%s/%d/%d/%d/%s", id.Owner, id.DSeq, id.GSeq, id.OSeq, id.Provider),
					bfilters.State, res.Bid.State.String()); err != nil {
					return err
				}

				return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
			}

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := cl.Query().Market().Bids(ctx, &mvbeta.QueryBidsRequest{
				Filters:    bfilters,
				Pagination: pageReq,
			})
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "bids")
	cflags.AddBidFilterFlags(cmd.Flags())
	cmd.Flags().String("by", "owner", "Filter perspective: owner or provider")

	return cmd
}

// GetQueryMarketLeaseCmd returns the command to query leases.
// Accepts an optional positional ID in the form [owner/]dseq[/gseq[/oseq[/provider]]].
// With --by provider: [provider/]dseq[/gseq[/oseq[/owner]]].
// When all ID fields are present returns a single lease.
// When partially specified, returns a filtered list.
func GetQueryMarketLeaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease [id] [state]",
		Short: "Query leases",
		Long: `Query leases.

The optional [id] argument is [owner/]dseq[/gseq[/oseq[/provider]]] (with
--by provider: [provider/]dseq[/gseq[/oseq[/owner]]]), a bare address, or a
bare state keyword (active|insufficient_funds|closed) per SPEC §3.8.

The optional [state] argument narrows the result: with a partial identity it
filters the list; when the identity pins down a single lease it verifies the
record instead — the command fails if the lease is in a different state
rather than printing it.`,
		Args:              cobra.MaximumNArgs(2),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			lfilters, err := cflags.LeaseFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			defaultOwner := cl.ClientContext().GetFromAddress().String()
			byProvider, _ := cmd.Flags().GetString("by")
			isByProvider := byProvider == "provider"

			if len(args) > 0 {
				af, err := cflags.LeaseFiltersFromArg(args[0], defaultOwner, isByProvider)
				if err != nil {
					return err
				}
				if af.Owner != "" {
					lfilters.Owner = af.Owner
				}
				if af.DSeq != 0 {
					lfilters.DSeq = af.DSeq
				}
				if af.GSeq != 0 {
					lfilters.GSeq = af.GSeq
				}
				if af.OSeq != 0 {
					lfilters.OSeq = af.OSeq
				}
				if af.Provider != "" {
					lfilters.Provider = af.Provider
				}
				// A bare state keyword may only appear once (SPEC §3.8.2).
				if af.State != "" {
					if len(args) > 1 {
						return fmt.Errorf("lease filter: state keyword %q cannot be combined with a second argument %q", args[0], args[1])
					}
					lfilters.State = af.State
				}
			}

			// Optional second positional: a state keyword narrowing the
			// identity filter (SPEC §3.8), e.g.
			// `akt query market lease 12345 active`.
			if len(args) > 1 {
				if lfilters.State, err = cflags.LeaseStateFromArg(args[1]); err != nil {
					return err
				}
			}

			// Default owner fallback when no arg and no --owner flag (owner mode only).
			if isByProvider {
				if lfilters.Provider == "" {
					return requireProviderScope("lease filter")
				}
			} else if lfilters.Owner == "" {
				if defaultOwner == "" {
					return requireOwnerScope("lease filter")
				}
				lfilters.Owner = defaultOwner
			}

			if cflags.LeaseFiltersIsID(lfilters) {
				id := mv1.LeaseID{
					Owner:    lfilters.Owner,
					DSeq:     lfilters.DSeq,
					GSeq:     lfilters.GSeq,
					OSeq:     lfilters.OSeq,
					Provider: lfilters.Provider,
				}

				res, err := cl.Query().Market().Lease(ctx, &mvbeta.QueryLeaseRequest{ID: id})
				if err != nil {
					return err
				}

				// SPEC §3.8.3: on the get path the positional [state] is a
				// verification, not a filter — never silently ignore it.
				if err := requireStateMatch("lease", fmt.Sprintf("%s/%d/%d/%d/%s", id.Owner, id.DSeq, id.GSeq, id.OSeq, id.Provider),
					lfilters.State, res.Lease.State.String()); err != nil {
					return err
				}

				return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
			}

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := cl.Query().Market().Leases(ctx, &mvbeta.QueryLeasesRequest{
				Filters:    lfilters,
				Pagination: pageReq,
			})
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "leases")
	cflags.AddLeaseFilterFlags(cmd.Flags())
	cmd.Flags().String("by", "owner", "Filter perspective: owner or provider")

	return cmd
}

func GetQueryMarketParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "params",
		Short:             "Query the current market parameters",
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			req := &mvbeta.QueryParamsRequest{}

			res, err := cl.Query().Market().Params(ctx, req)
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}
