package sync

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mv1beta "pkg.akt.dev/go/node/market/v1beta5"

	aktprovider "pkg.akt.dev/akt/internal/provider"
	"pkg.akt.dev/akt/internal/store"
)

const querierOwner = "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu"

// TestDeploymentRecordsCarryChainState pins the proto -> record conversion.
// These records are what `akt store status` and `akt store export` show, so a
// field silently dropped here is a field the user never sees.
func TestDeploymentRecordsCarryChainState(t *testing.T) {
	res := &dv1beta.QueryDeploymentsResponse{
		Deployments: dv1beta.DeploymentResponses{
			{
				Deployment: dv1.Deployment{
					ID:        dv1.DeploymentID{Owner: querierOwner, DSeq: 4649141},
					State:     dv1.DeploymentActive,
					Hash:      []byte{0xde, 0xad},
					CreatedAt: 4649140,
				},
				EscrowAccount: etypes.Account{
					State: etypes.AccountState{
						Funds: []etypes.Balance{
							{Denom: "uakt", Amount: math.LegacyNewDec(500000)},
						},
						Transferred: sdk.DecCoins{sdk.NewDecCoin("uakt", math.NewInt(1200))},
					},
				},
			},
			{
				Deployment: dv1.Deployment{
					ID:    dv1.DeploymentID{Owner: querierOwner, DSeq: 42},
					State: dv1.DeploymentClosed,
				},
			},
		},
	}

	got := deploymentRecords(res, 1700000000)
	if len(got) != 2 {
		t.Fatalf("records = %d, want 2", len(got))
	}

	active := got[0]
	if active.Owner != querierOwner || active.DSeq != 4649141 {
		t.Errorf("identity = %s/%d", active.Owner, active.DSeq)
	}
	if active.State != "active" {
		t.Errorf("state = %q, want active", active.State)
	}
	if string(active.Version) != string([]byte{0xde, 0xad}) {
		t.Errorf("version = %x, want the deployment hash", active.Version)
	}
	// CreatedHeight is a block height; CreatedAt is a wall clock and must not
	// be invented from it.
	if active.CreatedHeight != 4649140 {
		t.Errorf("created height = %d, want 4649140", active.CreatedHeight)
	}
	if active.CreatedAt != 0 {
		t.Errorf("created at = %d, want 0 (the chain reports a height, not a timestamp)", active.CreatedAt)
	}
	if active.UpdatedAt != 1700000000 {
		t.Errorf("updated at = %d, want the supplied now", active.UpdatedAt)
	}
	if active.EscrowBalance == "" || active.Transferred == "" {
		t.Errorf("escrow figures dropped: balance=%q transferred=%q", active.EscrowBalance, active.Transferred)
	}

	if got[1].State != "closed" {
		t.Errorf("closed deployment state = %q", got[1].State)
	}
	// An escrow account with no funds yields no coin string, not "0".
	if got[1].EscrowBalance != "" || got[1].Transferred != "" {
		t.Errorf("empty escrow rendered as %q/%q, want empty", got[1].EscrowBalance, got[1].Transferred)
	}
}

func TestLeaseRecordsCarryChainState(t *testing.T) {
	res := &mv1beta.QueryLeasesResponse{
		Leases: []mv1beta.QueryLeaseResponse{
			{
				Lease: mv1.Lease{
					ID: mv1.LeaseID{
						Owner:    querierOwner,
						DSeq:     4649141,
						GSeq:     1,
						OSeq:     1,
						Provider: "akash1provider",
					},
					State: mv1.LeaseActive,
					Price: sdk.NewDecCoin("uakt", math.NewInt(25)),
				},
			},
			{
				Lease: mv1.Lease{
					ID:    mv1.LeaseID{Owner: querierOwner, DSeq: 7, GSeq: 1, OSeq: 1, Provider: "akash1gone"},
					State: mv1.LeaseInsufficientFunds,
					Price: sdk.NewDecCoin("uakt", math.NewInt(3)),
				},
			},
		},
	}

	got := leaseRecords(res)
	if len(got) != 2 {
		t.Fatalf("records = %d, want 2", len(got))
	}
	if got[0].ID.Provider != "akash1provider" || got[0].ID.GSeq != 1 || got[0].ID.OSeq != 1 {
		t.Errorf("lease identity = %+v", got[0].ID)
	}
	if got[0].State != "active" {
		t.Errorf("state = %q, want active", got[0].State)
	}
	if got[0].Price == "" {
		t.Error("lease price dropped")
	}
	if got[1].State != "insufficient_funds" {
		t.Errorf("state = %q, want insufficient_funds", got[1].State)
	}
}

// TestBidRecordsTranslateChainStateVocabulary covers the one place the chain
// and the store disagree on a word: a bid that won is "active" on chain and
// "matched" in the store (the vocabulary the event handlers already use).
func TestBidRecordsTranslateChainStateVocabulary(t *testing.T) {
	states := []mv1beta.Bid_State{
		mv1beta.BidOpen,
		mv1beta.BidActive,
		mv1beta.BidLost,
		mv1beta.BidClosed,
	}
	want := []string{"open", "matched", "lost", "closed"}

	res := &mv1beta.QueryBidsResponse{}
	for i, st := range states {
		res.Bids = append(res.Bids, mv1beta.QueryBidResponse{
			Bid: mv1beta.Bid{
				ID: mv1.BidID{
					Owner:    querierOwner,
					DSeq:     4649141,
					GSeq:     1,
					OSeq:     uint32(i + 1), //nolint:gosec // small test index
					Provider: "akash1provider",
				},
				State: st,
				Price: sdk.NewDecCoin("uakt", math.NewInt(10)),
			},
		})
	}

	got := bidRecords(res)
	if len(got) != len(want) {
		t.Fatalf("records = %d, want %d", len(got), len(want))
	}
	for i, rec := range got {
		if rec.State != want[i] {
			t.Errorf("bid %d state = %q, want %q", i, rec.State, want[i])
		}
		if rec.ID.Owner != querierOwner || rec.ID.DSeq != 4649141 {
			t.Errorf("bid %d identity = %+v", i, rec.ID)
		}
	}
}

func TestApplyBidMetadataPopulatesKnownValues(t *testing.T) {
	records := []*store.BidRecord{{ID: store.BidID{Provider: "akash1provider"}}}
	applyBidMetadata(records, map[string]aktprovider.Metadata{
		"akash1provider": {
			Attributes: map[string]string{"region": "us-west"},
			Audited:    true,
		},
	})

	if records[0].ProviderAttributes["region"] != "us-west" || !records[0].ProviderAudited {
		t.Fatalf("bid metadata = %#v audited=%v", records[0].ProviderAttributes, records[0].ProviderAudited)
	}
}

func TestMergeBidPreservesMetadataOnlyWhenRefreshWasUnavailable(t *testing.T) {
	existing := &store.BidRecord{
		ProviderAttributes: map[string]string{"region": "us-west"},
		ProviderAudited:    true,
	}

	unavailable := mergeBid(existing, &store.BidRecord{}, 1)
	if unavailable.ProviderAttributes["region"] != "us-west" || !unavailable.ProviderAudited {
		t.Fatalf("unavailable refresh lost metadata: %#v", unavailable)
	}

	knownUnaudited := mergeBid(existing, &store.BidRecord{ProviderAttributes: map[string]string{}}, 1)
	if knownUnaudited.ProviderAttributes == nil || knownUnaudited.ProviderAudited {
		t.Fatalf("known unaudited refresh kept stale metadata: %#v", knownUnaudited)
	}
}

// TestRecordConvertersToleratePartialResponses guards the nil/empty arms: a
// query that returns nothing must yield no records, never a panic in the
// middle of a reconciliation.
func TestRecordConvertersToleratePartialResponses(t *testing.T) {
	if got := deploymentRecords(nil, 1); got != nil {
		t.Errorf("nil deployments response = %+v, want nil", got)
	}
	if got := leaseRecords(nil); got != nil {
		t.Errorf("nil leases response = %+v, want nil", got)
	}
	if got := bidRecords(nil); got != nil {
		t.Errorf("nil bids response = %+v, want nil", got)
	}
	if got := deploymentRecords(&dv1beta.QueryDeploymentsResponse{}, 1); len(got) != 0 {
		t.Errorf("empty deployments response = %+v, want none", got)
	}
}

// TestUnknownStatesRenderEmpty pins the default arms: an enum value this
// binary does not know must not be written as a made-up state string.
func TestUnknownStatesRenderEmpty(t *testing.T) {
	if got := deploymentState(dv1.Deployment_State(99)); got != "" {
		t.Errorf("unknown deployment state = %q, want empty", got)
	}
	if got := leaseState(mv1.Lease_State(99)); got != "" {
		t.Errorf("unknown lease state = %q, want empty", got)
	}
	if got := bidState(mv1beta.Bid_State(99)); got != "" {
		t.Errorf("unknown bid state = %q, want empty", got)
	}
}

func TestNextPageKeyStopsOnLastPage(t *testing.T) {
	if got := nextPageKey(nil); got != nil {
		t.Errorf("nil pagination = %v, want nil", got)
	}
}
