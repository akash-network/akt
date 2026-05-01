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
		Use:                        dv1.ModuleName + " [id]",
		Short:                      "Query deployments",
		Args:                       cobra.MaximumNArgs(1),
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

			if len(args) == 1 {
				af, err := cflags.DepFiltersFromArg(args[0], defaultOwner)
				if err != nil {
					return err
				}
				dfilters.Owner = af.Owner
				if af.DSeq != 0 {
					dfilters.DSeq = af.DSeq
				}
			}

			// Default owner fallback when no arg and no --owner flag.
			if dfilters.Owner == "" && defaultOwner != "" {
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
// When gseq is provided (via arg or --gseq flag), returns a single group.
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
			} else {
				// Read from flags
				id, err := cflags.DeploymentIDFromFlags(cmd.Flags())
				if err != nil {
					return err
				}
				owner = id.Owner
				dseq = id.DSeq

				if cmd.Flags().Changed(cflags.FlagGSeq) {
					gseq, _ = cmd.Flags().GetUint32(cflags.FlagGSeq)
				}
			}

			if owner == "" || dseq == 0 {
				return fmt.Errorf("owner and dseq are required (provide as owner/dseq[/gseq] or via --owner and --dseq flags)")
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
	cflags.AddGroupIDFlags(cmd.Flags())

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
