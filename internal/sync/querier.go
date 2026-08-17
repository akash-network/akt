package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/types/query"

	sdk "github.com/cosmos/cosmos-sdk/types"

	aclient "pkg.akt.dev/go/node/client"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mv1beta "pkg.akt.dev/go/node/market/v1beta5"

	aktprovider "pkg.akt.dev/akt/internal/provider"
	"pkg.akt.dev/akt/internal/store"
)

// queryPageLimit is the page size used when walking paginated chain queries.
// Reconciliation must see every record, not the first default-sized page: an
// account with more deployments than one page would otherwise be silently
// truncated in the local store.
const queryPageLimit = 100

// chainQuerier implements Querier against a live Akash node.
type chainQuerier struct {
	cl aclient.LightClient
}

// NewChainQuerier returns the Querier that reconciliation (SPEC §6.4) uses to
// read on-chain state through an Akash node client. Only queries are needed,
// so a LightClient suffices and no keyring or signer is involved.
func NewChainQuerier(cl aclient.LightClient) Querier {
	return &chainQuerier{cl: cl}
}

// CurrentHeight returns the node's latest block height.
//
// A node that is still catching up is rejected rather than used: reconciling
// against it would write stale records and then record a height that makes the
// next reconciliation think it is up to date.
func (q *chainQuerier) CurrentHeight(ctx context.Context) (int64, error) {
	info, err := q.cl.Node().SyncInfo(ctx)
	if err != nil {
		return 0, fmt.Errorf("query node sync info: %w", err)
	}

	if info.CatchingUp {
		return 0, fmt.Errorf("node is catching up (height %d); it cannot report the current chain state", info.LatestBlockHeight)
	}

	return info.LatestBlockHeight, nil
}

// Deployments returns every deployment owned by owner as store records.
func (q *chainQuerier) Deployments(ctx context.Context, owner string) ([]*store.DeploymentRecord, error) {
	now := time.Now().Unix()

	var out []*store.DeploymentRecord
	var nextKey []byte
	seenPageKeys := make(map[string]struct{})

	for {
		res, err := q.cl.Query().Deployment().Deployments(ctx, &dv1beta.QueryDeploymentsRequest{
			Filters:    dv1beta.DeploymentFilters{Owner: owner},
			Pagination: &query.PageRequest{Key: nextKey, Limit: queryPageLimit},
		})
		if err != nil {
			return nil, fmt.Errorf("query deployments for %s: %w", owner, err)
		}

		out = append(out, deploymentRecords(res, now)...)

		nextKey, err = unseenNextPageKey(res.GetPagination(), seenPageKeys)
		if err != nil {
			return nil, fmt.Errorf("query deployments for %s: %w", owner, err)
		}
		if len(nextKey) == 0 {
			return out, nil
		}
	}
}

// Leases returns every lease on the given deployment as store records.
func (q *chainQuerier) Leases(ctx context.Context, owner string, dseq uint64) ([]*store.LeaseRecord, error) {
	var out []*store.LeaseRecord
	var nextKey []byte
	seenPageKeys := make(map[string]struct{})

	for {
		res, err := q.cl.Query().Market().Leases(ctx, &mv1beta.QueryLeasesRequest{
			Filters:    mv1.LeaseFilters{Owner: owner, DSeq: dseq},
			Pagination: &query.PageRequest{Key: nextKey, Limit: queryPageLimit},
		})
		if err != nil {
			return nil, fmt.Errorf("query leases for %s/%d: %w", owner, dseq, err)
		}

		out = append(out, leaseRecords(res)...)

		nextKey, err = unseenNextPageKey(res.GetPagination(), seenPageKeys)
		if err != nil {
			return nil, fmt.Errorf("query leases for %s/%d: %w", owner, dseq, err)
		}
		if len(nextKey) == 0 {
			return out, nil
		}
	}
}

// Bids returns every bid on the given deployment as store records.
func (q *chainQuerier) Bids(ctx context.Context, owner string, dseq uint64) ([]*store.BidRecord, error) {
	var out []*store.BidRecord
	var nextKey []byte
	seenPageKeys := make(map[string]struct{})

	for {
		res, err := q.cl.Query().Market().Bids(ctx, &mv1beta.QueryBidsRequest{
			Filters:    mv1beta.BidFilters{Owner: owner, DSeq: dseq},
			Pagination: &query.PageRequest{Key: nextKey, Limit: queryPageLimit},
		})
		if err != nil {
			return nil, fmt.Errorf("query bids for %s/%d: %w", owner, dseq, err)
		}

		out = append(out, bidRecords(res)...)

		nextKey, err = unseenNextPageKey(res.GetPagination(), seenPageKeys)
		if err != nil {
			return nil, fmt.Errorf("query bids for %s/%d: %w", owner, dseq, err)
		}
		if len(nextKey) == 0 {
			break
		}
	}

	providers := make([]string, 0, len(out))
	for _, bid := range out {
		providers = append(providers, bid.ID.Provider)
	}
	applyBidMetadata(out, aktprovider.FetchChainMetadata(ctx, q.cl.Query(), providers))

	return out, nil
}

// nextPageKey returns the continuation key of a paginated response, or nil
// when the response was the last page.
func nextPageKey(p *query.PageResponse) []byte {
	if p == nil {
		return nil
	}

	return p.NextKey
}

// unseenNextPageKey prevents a malformed or inconsistent node response from
// trapping reconciliation in an unbounded pagination loop. A repeated key
// cannot advance the query, so returning an error is safer than presenting a
// partial local-store snapshot as current.
func unseenNextPageKey(p *query.PageResponse, seen map[string]struct{}) ([]byte, error) {
	nextKey := nextPageKey(p)
	if len(nextKey) == 0 {
		return nil, nil
	}

	key := string(nextKey)
	if _, ok := seen[key]; ok {
		return nil, fmt.Errorf("repeated pagination key %x", nextKey)
	}
	seen[key] = struct{}{}

	return nextKey, nil
}

// deploymentRecords converts a deployments query response into store records.
//
// now is the wall-clock timestamp recorded as the record's last local update.
// CreatedAt is *not* derived from it: the chain reports creation as a block
// height, which is what CreatedHeight carries, and inventing a wall-clock
// creation time from a height would be a guess.
func deploymentRecords(res *dv1beta.QueryDeploymentsResponse, now int64) []*store.DeploymentRecord {
	if res == nil {
		return nil
	}

	out := make([]*store.DeploymentRecord, 0, len(res.Deployments))
	for _, d := range res.Deployments {
		rec := &store.DeploymentRecord{
			Owner:         d.Deployment.ID.Owner,
			DSeq:          d.Deployment.ID.DSeq,
			State:         deploymentState(d.Deployment.State),
			Version:       d.Deployment.Hash,
			CreatedHeight: d.Deployment.CreatedAt,
			UpdatedAt:     now,
			EscrowBalance: balancesString(d.EscrowAccount.State.Funds),
			Transferred:   decCoinsString(d.EscrowAccount.State.Transferred),
		}

		out = append(out, rec)
	}

	return out
}

// leaseRecords converts a leases query response into store records.
func leaseRecords(res *mv1beta.QueryLeasesResponse) []*store.LeaseRecord {
	if res == nil {
		return nil
	}

	out := make([]*store.LeaseRecord, 0, len(res.Leases))
	for _, l := range res.Leases {
		out = append(out, &store.LeaseRecord{
			ID: store.LeaseID{
				Owner:    l.Lease.ID.Owner,
				DSeq:     l.Lease.ID.DSeq,
				GSeq:     l.Lease.ID.GSeq,
				OSeq:     l.Lease.ID.OSeq,
				Provider: l.Lease.ID.Provider,
			},
			State: leaseState(l.Lease.State),
			Price: l.Lease.Price.String(),
		})
	}

	return out
}

// bidRecords converts a bids query response into store records.
func bidRecords(res *mv1beta.QueryBidsResponse) []*store.BidRecord {
	if res == nil {
		return nil
	}

	out := make([]*store.BidRecord, 0, len(res.Bids))
	for _, b := range res.Bids {
		out = append(out, &store.BidRecord{
			ID: store.BidID{
				Owner:    b.Bid.ID.Owner,
				DSeq:     b.Bid.ID.DSeq,
				GSeq:     b.Bid.ID.GSeq,
				OSeq:     b.Bid.ID.OSeq,
				Provider: b.Bid.ID.Provider,
			},
			State: bidState(b.Bid.State),
			Price: b.Bid.Price.String(),
		})
	}

	return out
}

func applyBidMetadata(records []*store.BidRecord, metadata map[string]aktprovider.Metadata) {
	for _, record := range records {
		if record == nil {
			continue
		}
		providerMetadata, ok := metadata[record.ID.Provider]
		if !ok {
			continue
		}

		record.ProviderAttributes = make(map[string]string, len(providerMetadata.Attributes))
		for key, value := range providerMetadata.Attributes {
			record.ProviderAttributes[key] = value
		}
		record.ProviderAudited = providerMetadata.Audited
	}
}

// deploymentState maps the chain deployment state onto the store vocabulary.
func deploymentState(s dv1.Deployment_State) string {
	switch s {
	case dv1.DeploymentActive:
		return "active"
	case dv1.DeploymentClosed:
		return "closed"
	default:
		return ""
	}
}

// leaseState maps the chain lease state onto the store vocabulary.
func leaseState(s mv1.Lease_State) string {
	switch s {
	case mv1.LeaseActive:
		return "active"
	case mv1.LeaseInsufficientFunds:
		return "insufficient_funds"
	case mv1.LeaseClosed:
		return "closed"
	default:
		return ""
	}
}

// bidState maps the chain bid state onto the store vocabulary. The chain calls
// a bid that won a lease "active"; the store (and the event handlers above)
// call it "matched".
func bidState(s mv1beta.Bid_State) string {
	switch s {
	case mv1beta.BidOpen:
		return "open"
	case mv1beta.BidActive:
		return "matched"
	case mv1beta.BidLost:
		return "lost"
	case mv1beta.BidClosed:
		return "closed"
	default:
		return ""
	}
}

// balancesString renders escrow funds as a coin string. Balances are built
// into DecCoin values field-by-field rather than through the sdk constructors,
// which panic on denominations the chain may legitimately introduce later.
func balancesString(funds []etypes.Balance) string {
	if len(funds) == 0 {
		return ""
	}

	coins := make(sdk.DecCoins, 0, len(funds))
	for _, f := range funds {
		coins = append(coins, sdk.DecCoin{Denom: f.Denom, Amount: f.Amount})
	}

	return decCoinsString(coins)
}

// decCoinsString renders coins as a string, mapping "no coins" to the empty
// string rather than the SDK's "" / "0" ambiguity.
func decCoinsString(coins sdk.DecCoins) string {
	if len(coins) == 0 {
		return ""
	}

	return coins.String()
}
