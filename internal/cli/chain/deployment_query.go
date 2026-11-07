package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
)

// GetQueryDeploymentCmds returns the query commands for the deployment module.
// The command itself handles list/get (unified), with group and params as subcommands.
func GetQueryDeploymentCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:   dv1.ModuleName + " [id] [state]",
		Short: "Query deployments",
		Long: `Query deployments.

Without arguments, lists every deployment on the network, one page at a
time (--limit, default 100). Continue with --page-key set to the previous
page's pagination.next_key (shown by -o json); the chain accepts --page and
--offset only together with a state.

[id] identifies what to look at, and accepts any of:

  <owner>          every deployment owned by that address
  <owner>/<dseq>   one deployment
  <dseq>           one deployment owned by the default account
  active | closed  every deployment on the network in that state

[state] narrows the result. With an owner or no identity it filters the
list; when [id] names a single deployment it checks the state instead, and
the command fails if the deployment is in a different one.`,
		Example: `  # Every deployment on the network (first page of 100)
  akt query deployment

  # ...only the active ones
  akt query deployment active

  # The next page: pagination.next_key from the previous page (-o json)
  akt query deployment --page-key <next_key>

  # Offset paging needs a state: the second page of 20 active deployments
  akt query deployment active --limit 20 --page 2

  # Every deployment owned by an address
  akt query deployment akash1zn43lm...

  # One deployment
  akt query deployment akash1zn43lm.../25354313

  # One deployment of the default account
  akt query deployment 25354313

  # Fails unless that deployment is closed
  akt query deployment akash1zn43lm.../25354313 closed`,
		Args:                       cobra.MaximumNArgs(2),
		SuggestionsMinimumDistance: 2,
		PersistentPreRunE:          QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			dfilters, err := cflags.DepFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			defaultOwner, err := defaultOwnerForQueryArg(cl.ClientContext(), args)
			if err != nil {
				return err
			}

			if len(args) > 0 {
				af, err := cflags.DepFiltersFromArg(args[0], defaultOwner)
				if err != nil {
					return err
				}
				if af.Owner != "" {
					dfilters.Owner = af.Owner
				}
				if af.DSeq != 0 {
					dfilters.DSeq = af.DSeq
				}
				// A bare state keyword may only appear once (SPEC §3.8.2).
				if af.State != "" {
					if len(args) > 1 {
						return fmt.Errorf("deployment filter: state keyword %q cannot be combined with a second argument %q", args[0], args[1])
					}
					dfilters.State = af.State
				}
			}

			// Optional second positional: a state keyword narrowing the
			// identity filter (SPEC §3.8), e.g.
			// `akt query deployment akash1owner/12345 active`.
			if len(args) > 1 {
				if dfilters.State, err = cflags.DeploymentStateFromArg(args[1]); err != nil {
					return err
				}
			}

			// An empty owner lists the whole network (SPEC §3.8.4); only the
			// numeric dseq shorthand above takes the default account.
			if cflags.DepFiltersIsID(dfilters) {
				id := dv1.DeploymentID{
					Owner: dfilters.Owner,
					DSeq:  dfilters.DSeq,
				}

				res, err := cl.Query().Deployment().Deployment(ctx, &dvbeta.QueryDeploymentRequest{ID: id})
				if err != nil {
					return err
				}

				// SPEC §3.8.3: on the get path the positional [state] is a
				// verification, not a filter — never silently ignore it.
				if err := requireStateMatch(dv1.ModuleName, fmt.Sprintf("%s/%d", id.Owner, id.DSeq),
					dfilters.State, res.Deployment.State.String()); err != nil {
					return err
				}

				return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
			}

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := cl.Query().Deployment().Deployments(ctx, &dvbeta.QueryDeploymentsRequest{
				Filters:    dfilters,
				Pagination: pageReq,
			})
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "deployments")
	cflags.AddDeploymentFilterFlags(cmd.Flags())

	cmd.AddCommand(
		GetQueryDeploymentGroupCmd(),
		GetQueryDeploymentParamsCmd(),
	)

	return cmd
}

// GetQueryDeploymentGroupCmd returns the command to query deployment groups.
// Accepts an optional positional ID in the form owner[/dseq[/gseq]].
// When gseq is provided, returns a single group.
// When only owner/dseq are given, fetches the deployment and shows all its groups.
// Without a dseq, shows the groups of one page of deployments (SPEC §3.8.5).
func GetQueryDeploymentGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group [id]",
		Short: "Query deployment groups",
		Long: `Query deployment groups.

Without arguments, lists the groups of every deployment on the network. No
chain query lists groups, so this fetches one page of deployments (--limit,
default 100) and prints their groups. Continue with --page-key set to the
pagination.next_key that -o json prints; --page and --offset need a state
filter, which a group listing cannot take.

[id] identifies what to look at, and accepts any of:

  <owner>                 the groups of that owner's deployments (one page)
  <owner>/<dseq>[/gseq]   every group of one deployment, or one group
  <dseq>[/gseq]           the same, owned by the default account`,
		Example: `  # Groups of every deployment on the network (first page of deployments)
  akt query deployment group

  # Groups of one owner's deployments
  akt query deployment group akash1zn43lm...

  # Every group of one deployment
  akt query deployment group akash1zn43lm.../25354313

  # Group 1 of a deployment of the default account
  akt query deployment group 25354313/1`,
		Args:              cobra.MaximumNArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			var (
				owner string
				dseq  uint64
				gseq  uint32
			)

			defaultOwner, err := defaultOwnerForQueryArg(cl.ClientContext(), args)
			if err != nil {
				return err
			}

			if len(args) == 1 {
				// A bare owner address lists that owner's groups; anything
				// longer is a group path.
				if addr, aerr := sdk.AccAddressFromBech32(args[0]); aerr == nil {
					owner = addr.String()
				} else {
					parsed, fullySpecified, err := cflags.GroupIDFromArg(args[0], defaultOwner)
					if err != nil {
						return err
					}
					owner = parsed.Owner
					dseq = parsed.DSeq
					if fullySpecified {
						gseq = parsed.GSeq
					}
				}
			}
			// FEEDBACK(2026-07): the --owner/--dseq/--gseq flag path is
			// disabled for the positional-only UX trial; the positional
			// [owner/]dseq[/gseq] argument is the only source. Restore by
			// uncommenting if users ask for the flag form back.
			// } else {
			// 	// Read from flags
			// 	id, err := cflags.DeploymentIDFromFlags(cmd.Flags())
			// 	if err != nil {
			// 		return err
			// 	}
			// 	owner = id.Owner
			// 	dseq = id.DSeq
			//
			// 	if cmd.Flags().Changed(flagdefs.FlagGSeq) {
			// 		gseq, _ = cmd.Flags().GetUint32(flagdefs.FlagGSeq)
			// 	}
			// }

			// Without a dseq there is no single deployment to read, and no
			// chain query lists groups: list one page of deployments (the
			// whole network when owner is empty) and show their groups.
			if dseq == 0 {
				if err := rejectOffsetPaging(cmd.Flags(), "deployment group"); err != nil {
					return err
				}

				pageReq, err := ReadPageRequest(cmd.Flags())
				if err != nil {
					return err
				}

				res, err := cl.Query().Deployment().Deployments(ctx, &dvbeta.QueryDeploymentsRequest{
					Filters:    dvbeta.DeploymentFilters{Owner: owner},
					Pagination: pageReq,
				})
				if err != nil {
					return err
				}

				var groups dvbeta.Groups
				for _, d := range res.Deployments {
					groups = append(groups, d.Groups...)
				}

				return pretty.PrintGroupsPage(cmd, cl.ClientContext(), groups, res.Pagination)
			}

			// If gseq is specified, query a single group.
			if gseq != 0 {
				id := dv1.MakeGroupID(dv1.DeploymentID{Owner: owner, DSeq: dseq}, gseq)

				res, err := cl.Query().Deployment().Group(ctx, &dvbeta.QueryGroupRequest{ID: id})
				if err != nil {
					return err
				}

				return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
			}

			// Otherwise, fetch the deployment and show all groups.
			depID := dv1.DeploymentID{Owner: owner, DSeq: dseq}

			res, err := cl.Query().Deployment().Deployment(ctx, &dvbeta.QueryDeploymentRequest{ID: depID})
			if err != nil {
				return err
			}

			return pretty.PrintGroupsList(cmd, cl.ClientContext(), res.Groups)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "deployments")
	// FEEDBACK(2026-07): --owner/--dseq/--gseq disabled for the
	// positional-only UX trial (use the positional owner/dseq[/gseq] form
	// instead). Restore by uncommenting if users ask for the flag form back.
	// cflags.AddGroupIDFlags(cmd.Flags())

	return cmd
}

func GetQueryDeploymentParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "params",
		Args:              cobra.NoArgs,
		Short:             "Query the current deployment parameters",
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			req := &dvbeta.QueryParamsRequest{}

			res, err := cl.Query().Deployment().Params(ctx, req)
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}
