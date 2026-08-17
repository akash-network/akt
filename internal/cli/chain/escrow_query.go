package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	bme "pkg.akt.dev/go/node/bme/v1"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	"pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	oracle "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/node/utils"
	"pkg.akt.dev/go/sdkutil"
	netutil "pkg.akt.dev/go/util/network"
)

var errNoLeaseMatches = errors.New("leases for deployment do not exist")

const (
	authzDepositScopeDeployment = "deployment"
	authzDepositScopeBid        = "bid"
)

func GetQueryEscrowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        module.ModuleName,
		Short:                      "Escrow query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetQueryEscrowAccountsCmd(),
		GetQueryEscrowPaymentsCmd(),
		GetQueryEscrowBlocksRemainingCmd(),
	)

	return cmd
}

func parseXID(xid string) ([]string, error) { //nolint: unparam
	xid = strings.TrimPrefix(xid, "/")
	xid = strings.TrimSuffix(xid, "/")

	parts := strings.Split(xid, "/")

	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid xid format")
	}

	switch parts[0] {
	case authzDepositScopeDeployment, authzDepositScopeBid:
	default:
		return nil, fmt.Errorf("invalid xid scope prefix. allowed values - deployment|bid")
	}

	return parts, nil
}

func validateEscrowState(state string) (string, error) {
	switch state {
	case "open", "closed", "overdrawn":
	default:
		return "", fmt.Errorf("invalid account state. allowed values - open|closed|overdrawn")
	}

	return state, nil
}

func GetQueryEscrowAccountsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accounts <state> <xid>",
		Short: "Query for escrow account(s)",
		Long: strings.TrimSpace(
			fmt.Sprintf(`Query escrow accounts.
Arguments are optional. XID cannot be provided without state
<state> - allowed values are open|closed|overdrawn
<xid> - format must follow template - [scope]</xid...>
        allowed scope values are deployment|bid
        xid examples:
        - deployment
        - deployment/akash1...
        - deployment/akash1.../dseq
Examples (pagination limits apply to all examples below):
1. Return all accounts
$ %[1]s query %[2]s accounts
2. Return accounts in open state
$ %[1]s query %[2]s accounts open
3. Return accounts in open state for deployment scope
$ %[1]s query %[2]s accounts open deployment
3. Return accounts in open state for deployment scope
$ %[1]s query %[2]s accounts open deployment/akash1...
`,
				version.AppName, module.ModuleName)),
		Args:              cobra.RangeArgs(0, 2),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			if err := rejectUnsupportedPaging(cmd.Flags(), "escrow"); err != nil {
				return err
			}

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			var state string
			var xid string

			if len(args) > 0 {
				state, err = validateEscrowState(args[0])
				if err != nil {
					return err
				}
			}

			if len(args) > 1 {
				xid = args[1]
				_, err = parseXID(xid)
				if err != nil {
					return err
				}
			}
			req := &etypes.QueryAccountsRequest{
				State:      state,
				XID:        xid,
				Pagination: pageReq,
			}

			res, err := cl.Query().Escrow().Accounts(ctx, req)
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "escrow")

	return cmd
}

func GetQueryEscrowPaymentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "payments <state> <xid>",
		Short: "Query escrow payments",
		Long: strings.TrimSpace(
			fmt.Sprintf(`Query escrow payments.
Arguments are optional. XID cannot be provided without state
<state> - allowed values are open|closed|overdrawn
<xid> - format must follow template - [scope]</xid...>
        allowed scope values are deployment|bid
        xid examples:
        - deployment
        - deployment/akash1...
        - deployment/akash1.../dseq
Examples (pagination limits apply to all examples below):
1. Return all payments
$ %[1]s query %[2]s payments
2. Return payments in open state
$ %[1]s query %[2]s payments open
3. Return payments in open state for deployment scope
$ %[1]s query %[2]s payments open deployment
4. Return payments in open state for one deployment
$ %[1]s query %[2]s payments open deployment/akash1.../12345
`,
				version.AppName, module.ModuleName)),
		Example: `  akt query escrow payments
  akt query escrow payments open
  akt query escrow payments open deployment
  akt query escrow payments open deployment/akash1.../12345`,
		Args:              cobra.RangeArgs(0, 2),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			if err := rejectUnsupportedPaging(cmd.Flags(), "escrow"); err != nil {
				return err
			}

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			var state string
			var xid string

			if len(args) > 0 {
				state, err = validateEscrowState(args[0])
				if err != nil {
					return err
				}
			}

			if len(args) > 1 {
				xid = args[1]
				_, err = parseXID(args[1])
				if err != nil {
					return err
				}
			}
			req := &etypes.QueryPaymentsRequest{
				State:      state,
				XID:        xid,
				Pagination: pageReq,
			}

			res, err := cl.Query().Escrow().Payments(ctx, req)
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "escrow")

	return cmd
}

func GetQueryEscrowBlocksRemainingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocks-remaining [filter]",
		Short: "Compute the number of blocks remaining for an escrow account",
		Long: `Compute the number of blocks remaining for an escrow account.
The filter argument takes the form <owner>/<dseq>, or a bare <dseq> to use
the context's default account.`,
		Example: `akt query escrow blocks-remaining 12345
akt query escrow blocks-remaining akash1owner.../12345`,
		Args:              cobra.MaximumNArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			// FEEDBACK(2026-07): --owner/--dseq disabled for the
			// positional-only UX trial; start from a zero ID so the
			// positional [owner/]dseq filter is the only source. Restore by
			// uncommenting if users ask for the flag form back.
			// id, err := cflags.DeploymentIDFromFlags(cmd.Flags())
			// if err != nil {
			// 	return err
			// }
			var id dv1.DeploymentID

			// Positional [owner/]dseq; owner defaults to the context
			// default account (SPEC §3.8).
			defaultOwner, err := defaultOwnerForQueryArg(cl.ClientContext(), args)
			if err != nil {
				return err
			}
			id, err = cflags.DeploymentIDFromArgs(args, id, defaultOwner)
			if err != nil {
				return err
			}

			if id.Owner == "" {
				return errors.New("owner is required: pass the filter positionally as [owner/]dseq")
			}

			if id.DSeq == 0 {
				return errors.New("dseq is required: pass the filter positionally as [owner/]dseq")
			}

			// Fetch leases matching owner & dseq
			leaseRequest := mvbeta.QueryLeasesRequest{
				Filters: mv1.LeaseFilters{
					Owner:    id.Owner,
					DSeq:     id.DSeq,
					GSeq:     0,
					OSeq:     0,
					Provider: "",
					State:    mv1.LeaseActive.String(),
				},
				Pagination: nil,
			}

			bmeStatus, err := cl.Query().BME().Status(ctx, &bme.QueryStatusRequest{})
			if err != nil {
				return err
			}

			aktPrice, err := cl.Query().Oracle().AggregatedPrice(ctx, &oracle.QueryAggregatedPriceRequest{Denom: sdkutil.DenomAkt})
			if err != nil {
				return err
			}

			leasesResponse, err := cl.Query().Market().Leases(ctx, &leaseRequest)
			if err != nil {
				return err
			}

			if len(leasesResponse.Leases) == 0 {
				return errNoLeaseMatches
			}

			// Fetch the balance of the escrow account
			totalLeaseRate := leasesResponse.TotalPriceAmount()
			blockchainHeight, err := cl.Node().CurrentBlockHeight(ctx)
			if err != nil {
				return err
			}

			res, err := cl.Query().Deployment().Deployment(cmd.Context(), &dv1beta.QueryDeploymentRequest{
				ID: dv1.DeploymentID{Owner: id.Owner, DSeq: id.DSeq},
			})
			if err != nil {
				return err
			}

			balanceRemaining := sdk.NewInt64DecCoin(sdkutil.DenomUact, 0)

			for _, funds := range res.EscrowAccount.State.Funds {
				if funds.Amount.IsNegative() {
					continue
				}

				if funds.Denom == sdkutil.DenomUact {
					balanceRemaining.Amount.AddMut(funds.Amount)
				} else if funds.Denom == sdkutil.DenomUakt &&
					(bmeStatus.Status >= bme.MintStatusHaltCR) &&
					aktPrice.PriceHealth.IsHealthy {
					// Account for uAKT only if the BME circuit breaker is active.
					swappedRate := funds.Amount.Mul(aktPrice.AggregatedPrice.TWAP)
					balanceRemaining.Amount.AddMut(swappedRate)
				}
			}

			balanceRemaining = utils.LeaseCalcBalanceRemain(balanceRemaining.Amount, blockchainHeight, res.EscrowAccount.State.SettledAt, sdk.NewDecCoinFromDec(sdkutil.DenomUact, totalLeaseRate))
			blocksRemaining := utils.LeaseCalcBlocksRemain(balanceRemaining.Amount, totalLeaseRate)

			output := struct {
				BalanceRemain          sdk.DecCoin   `json:"balance_remaining" yaml:"balance_remaining"`
				BlocksRemain           int64         `json:"blocks_remaining" yaml:"blocks_remaining"`
				EstimatedTimeRemaining time.Duration `json:"estimated_time_remaining" yaml:"estimated_time_remaining"`
			}{
				BalanceRemain:          balanceRemaining,
				BlocksRemain:           blocksRemaining,
				EstimatedTimeRemaining: netutil.AverageBlockTime * time.Duration(blocksRemaining),
			}

			outputType, err := cmd.Flags().GetString(flagdefs.FlagOutput)
			if err != nil {
				return err
			}

			var data []byte
			if outputType == "json" {
				data, err = json.MarshalIndent(output, " ", "\t")
			} else {
				data, err = yaml.Marshal(output)
			}

			if err != nil {
				return err
			}

			return cl.ClientContext().PrintBytes(data)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	// FEEDBACK(2026-07): --owner/--dseq disabled for the positional-only UX
	// trial (use the positional [owner/]dseq filter instead). Restore by
	// uncommenting if users ask for the flag form back.
	// cflags.AddDeploymentIDFlags(cmd.Flags())

	return cmd
}
