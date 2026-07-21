package cli

import (
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
		Use:               "order [id]",
		Short:             "Query orders",
		Args:              cobra.MaximumNArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			ofilters, err := cflags.OrderFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			defaultOwner := cl.ClientContext().GetFromAddress().String()

			if len(args) == 1 {
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
				// Positional state keyword wins over --state (SPEC §3.8.2).
				if af.State != "" {
					ofilters.State = af.State
				}
			}

			// Default owner fallback when no arg and no --owner flag.
			if ofilters.Owner == "" && defaultOwner != "" {
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
		Use:               "bid [id]",
		Short:             "Query bids",
		Args:              cobra.MaximumNArgs(1),
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

			if len(args) == 1 {
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
				// Positional state keyword wins over --state (SPEC §3.8.2).
				if af.State != "" {
					bfilters.State = af.State
				}
			}

			// Default owner fallback when no arg and no --owner flag (owner mode only).
			if !isByProvider && bfilters.Owner == "" && defaultOwner != "" {
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
		Use:               "lease [id]",
		Short:             "Query leases",
		Args:              cobra.MaximumNArgs(1),
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

			if len(args) == 1 {
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
				// Positional state keyword wins over --state (SPEC §3.8.2).
				if af.State != "" {
					lfilters.State = af.State
				}
			}

			// Default owner fallback when no arg and no --owner flag (owner mode only).
			if !isByProvider && lfilters.Owner == "" && defaultOwner != "" {
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
