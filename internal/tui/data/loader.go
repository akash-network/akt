package data

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/akt/internal/output/pretty"
	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/messages"
	aclient "pkg.akt.dev/go/node/client"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

// Loader implements Service using a Store and LightClient.
type Loader struct {
	Store       store.Store
	LightClient aclient.LightClient
}

// NewLoader creates a Loader. Either Store or LightClient may be nil;
// methods that require a nil dependency return an error message.
func NewLoader(s store.Store, cl aclient.LightClient) *Loader {
	return &Loader{Store: s, LightClient: cl}
}

// LoadDeployments returns a tea.Cmd that loads deployments from the store.
func (l *Loader) LoadDeployments(owner string) tea.Cmd {
	return func() tea.Msg {
		if l.Store == nil {
			return messages.DeploymentsLoadedMsg{Err: fmt.Errorf("no store available")}
		}
		depls, err := l.Store.ListDeployments(context.Background(), store.DeploymentFilter{Owner: owner})
		return messages.DeploymentsLoadedMsg{Deployments: depls, Err: err}
	}
}

// LoadLeases returns a tea.Cmd that loads all leases for an owner.
func (l *Loader) LoadLeases(owner string) tea.Cmd {
	return func() tea.Msg {
		if l.Store == nil {
			return messages.LeasesLoadedMsg{Err: fmt.Errorf("no store available")}
		}
		leases, err := l.Store.ListLeases(context.Background(), store.LeaseFilter{Owner: owner})
		return messages.LeasesLoadedMsg{Leases: leases, Err: err}
	}
}

// LoadDeploymentLeases returns a tea.Cmd that loads leases for a specific deployment.
func (l *Loader) LoadDeploymentLeases(owner string, dseq uint64) tea.Cmd {
	return func() tea.Msg {
		if l.Store == nil {
			return messages.LeasesLoadedMsg{Err: fmt.Errorf("no store available")}
		}
		leases, err := l.Store.ListLeases(context.Background(), store.LeaseFilter{Owner: owner, DSeq: dseq})
		return messages.LeasesLoadedMsg{Leases: leases, Err: err}
	}
}

// LoadBids returns a tea.Cmd that loads bids for a specific deployment.
func (l *Loader) LoadBids(owner string, dseq uint64) tea.Cmd {
	return func() tea.Msg {
		if l.Store == nil {
			return messages.BidsLoadedMsg{Err: fmt.Errorf("no store available")}
		}
		bids, err := l.Store.ListBids(context.Background(), store.BidFilter{Owner: owner, DSeq: dseq})
		return messages.BidsLoadedMsg{DSeq: dseq, Bids: bids, Err: err}
	}
}

// LoadStoreStats returns a tea.Cmd that loads aggregate store statistics.
func (l *Loader) LoadStoreStats() tea.Cmd {
	return func() tea.Msg {
		if l.Store == nil {
			return messages.StoreStatsMsg{Err: fmt.Errorf("no store available")}
		}
		stats, err := l.Store.Stats(context.Background())
		return messages.StoreStatsMsg{Stats: stats, Err: err}
	}
}

// LoadSyncState returns a tea.Cmd that loads the current sync state.
func (l *Loader) LoadSyncState() tea.Cmd {
	return func() tea.Msg {
		if l.Store == nil {
			return messages.SyncStateMsg{Err: fmt.Errorf("no store available")}
		}
		state, err := l.Store.GetSyncState(context.Background())
		return messages.SyncStateMsg{State: state, Err: err}
	}
}

// LoadBalance returns a tea.Cmd that loads the account balance from the chain.
func (l *Loader) LoadBalance(account string) tea.Cmd {
	return func() tea.Msg {
		if l.LightClient == nil || account == "" {
			return messages.BalanceLoadedMsg{Err: fmt.Errorf("no client or account")}
		}
		resp, err := l.LightClient.Query().Bank().AllBalances(context.Background(), &banktypes.QueryAllBalancesRequest{
			Address: account,
		})
		if err != nil {
			return messages.BalanceLoadedMsg{Err: err}
		}
		for _, coin := range resp.Balances {
			if coin.Denom == "uakt" {
				return messages.BalanceLoadedMsg{Amount: pretty.FormatCoin(coin)}
			}
		}
		return messages.BalanceLoadedMsg{Amount: "0 AKT"}
	}
}

// LoadProposals returns a tea.Cmd that loads governance proposals from the chain.
func (l *Loader) LoadProposals() tea.Cmd {
	return func() tea.Msg {
		if l.LightClient == nil {
			return messages.ProposalsLoadedMsg{Err: fmt.Errorf("no chain client available")}
		}
		res, err := l.LightClient.Query().Gov().Proposals(context.Background(), &govv1.QueryProposalsRequest{
			Pagination: &query.PageRequest{Limit: 100, Reverse: true},
		})
		if err != nil {
			return messages.ProposalsLoadedMsg{Err: err}
		}
		return messages.ProposalsLoadedMsg{Proposals: res.Proposals}
	}
}

// LoadTallies returns a tea.Cmd that loads vote tallies for proposals in voting period.
func (l *Loader) LoadTallies(proposals []*govv1.Proposal) tea.Cmd {
	return func() tea.Msg {
		if l.LightClient == nil {
			return messages.TallyLoadedMsg{Err: fmt.Errorf("no chain client")}
		}
		tallies := make(map[uint64]*govv1.TallyResult)
		for _, p := range proposals {
			if p.Status == govv1.StatusVotingPeriod {
				resp, err := l.LightClient.Query().Gov().TallyResult(context.Background(), &govv1.QueryTallyResultRequest{
					ProposalId: p.Id,
				})
				if err == nil && resp != nil {
					tallies[p.Id] = resp.Tally
				}
			}
		}
		return messages.TallyLoadedMsg{Tallies: tallies}
	}
}

// LoadValidators returns a tea.Cmd that loads validators from the chain.
func (l *Loader) LoadValidators() tea.Cmd {
	return func() tea.Msg {
		if l.LightClient == nil {
			return messages.ValidatorsLoadedMsg{Err: fmt.Errorf("no chain client available")}
		}
		res, err := l.LightClient.Query().Staking().Validators(context.Background(), &stakingtypes.QueryValidatorsRequest{
			Pagination: &query.PageRequest{Limit: 200},
		})
		if err != nil {
			return messages.ValidatorsLoadedMsg{Err: err}
		}
		return messages.ValidatorsLoadedMsg{Validators: res.Validators}
	}
}

// LoadStakingPool returns a tea.Cmd that loads the staking pool info from the chain.
func (l *Loader) LoadStakingPool() tea.Cmd {
	return func() tea.Msg {
		if l.LightClient == nil {
			return messages.StakingPoolMsg{Err: fmt.Errorf("no chain client")}
		}
		resp, err := l.LightClient.Query().Staking().Pool(context.Background(), &stakingtypes.QueryPoolRequest{})
		if err != nil {
			return messages.StakingPoolMsg{Err: err}
		}
		return messages.StakingPoolMsg{BondedTokens: resp.Pool.BondedTokens}
	}
}

// LoadProviders returns a tea.Cmd that loads on-chain providers.
func (l *Loader) LoadProviders() tea.Cmd {
	return func() tea.Msg {
		if l.LightClient == nil {
			return messages.ProvidersLoadedMsg{Err: fmt.Errorf("no chain client available")}
		}
		res, err := l.LightClient.Query().Provider().Providers(context.Background(), &ptypes.QueryProvidersRequest{
			Pagination: &query.PageRequest{Limit: 200},
		})
		if err != nil {
			return messages.ProvidersLoadedMsg{Err: err}
		}
		return messages.ProvidersLoadedMsg{Providers: res.Providers}
	}
}
