package cli

import (
	"fmt"

	"github.com/spf13/cobra"

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

Without arguments, lists the deployments owned by the context's default
account. The optional [id] argument is [owner/]dseq, a bare owner address,
or a bare state keyword (active|closed) per SPEC §3.8.

The optional [state] argument narrows the result: with a partial identity it
filters the list; when the identity pins down a single deployment it verifies
the record instead — the command fails if the deployment is in a different
state rather than printing it.`,
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

			defaultOwner := cl.ClientContext().GetFromAddress().String()

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

			// Default owner fallback when no arg and no --owner flag.
			if dfilters.Owner == "" {
				if defaultOwner == "" {
					return requireOwnerScope("deployment filter")
				}
				dfilters.Owner = defaultOwner
			}

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
// Accepts an optional positional ID in the form owner/dseq[/gseq].
// When gseq is provided, returns a single group.
// When only owner/dseq are given, fetches the deployment and shows all its groups.
func GetQueryDeploymentGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "group [id]",
		Short:             "Query deployment groups",
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

			defaultOwner := cl.ClientContext().GetFromAddress().String()

			if len(args) == 1 {
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
			// 	if cmd.Flags().Changed(cflags.FlagGSeq) {
			// 		gseq, _ = cmd.Flags().GetUint32(cflags.FlagGSeq)
			// 	}
			// }

			if owner == "" || dseq == 0 {
				return fmt.Errorf("owner and dseq are required (provide as owner/dseq[/gseq])")
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
	// FEEDBACK(2026-07): --owner/--dseq/--gseq disabled for the
	// positional-only UX trial (use the positional owner/dseq[/gseq] form
	// instead). Restore by uncommenting if users ask for the flag form back.
	// cflags.AddGroupIDFlags(cmd.Flags())

	return cmd
}

func GetQueryDeploymentParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "params",
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
