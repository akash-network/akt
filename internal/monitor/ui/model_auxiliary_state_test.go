package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	"cosmossdk.io/math"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/monitor/cache"
	"pkg.akt.dev/akt/internal/monitor/governance"
	"pkg.akt.dev/akt/internal/monitor/rpc"

	bmetypes "pkg.akt.dev/go/node/bme/v1"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/util/pubsub"
)

func TestOracleBusEventsUpdateDashboardAndRearmSubscription(t *testing.T) {
	bus := pubsub.NewBus()
	t.Cleanup(bus.Close)
	subscriber, err := bus.Subscribe()
	if err != nil {
		t.Fatal(err)
	}

	m := Model{
		runtimeContext: context.Background(),
		subscriber:     subscriber,
		oracle: OracleState{
			Prices:     make([]OraclePriceEntry, maxOracleEvents),
			Aggregated: make(map[string]*oracletypes.EventAggregatedPrice),
			Events:     make([]OracleEvent, maxOracleEvents),
		},
	}

	price := math.LegacyMustNewDecFromStr("3.45")
	priceTime := time.Date(2026, time.August, 12, 4, 0, 0, 0, time.UTC)
	updated, cmd := m.Update(busEventMsg{ok: true, event: &oracletypes.EventPriceData{
		Source:    "akash1oracle",
		Id:        oracletypes.DataID{Denom: "uakt", BaseDenom: "usd"},
		Price:     price,
		Timestamp: priceTime,
	}})
	m = lifecycleModelValue(t, updated)
	if cmd == nil {
		t.Fatal("price event did not rearm the bus subscription")
	}
	if len(m.oracle.Prices) != maxOracleEvents || len(m.oracle.Events) != maxOracleEvents {
		t.Fatalf("bounded oracle histories = %d prices, %d events", len(m.oracle.Prices), len(m.oracle.Events))
	}
	if got := m.oracle.Prices[0]; got.Denom != "uakt" || got.Price != price.String() ||
		got.Source != "akash1oracle" || !got.Timestamp.Equal(priceTime) {
		t.Fatalf("newest price entry = %#v", got)
	}
	if got := m.oracle.Events[0]; got.Type != "price" || got.Denom != "uakt" ||
		got.Detail != price.String() || got.Timestamp.IsZero() {
		t.Fatalf("newest price event = %#v", got)
	}

	aggregated := &oracletypes.EventAggregatedPrice{Price: oracletypes.AggregatedPrice{
		Denom: "uakt",
		TWAP:  math.LegacyMustNewDecFromStr("3.50"),
	}}
	updated, cmd = m.Update(busEventMsg{ok: true, event: aggregated})
	m = lifecycleModelValue(t, updated)
	if cmd == nil || m.oracle.Aggregated["uakt"] != aggregated {
		t.Fatalf("aggregated event was not stored and rearmed: cmd=%v aggregate=%p", cmd != nil, m.oracle.Aggregated["uakt"])
	}
	if got := m.oracle.Events[0]; got.Type != "aggregated" || got.Denom != "uakt" || got.Detail != aggregated.Price.TWAP.String() {
		t.Fatalf("aggregated event log entry = %#v", got)
	}

	for _, test := range []struct {
		name      string
		event     pubsub.Event
		wantType  string
		wantDenom string
	}{
		{
			name:      "stale warning",
			event:     &oracletypes.EventPriceStaleWarning{Id: oracletypes.DataID{Denom: "uakt"}},
			wantType:  "stale_warning",
			wantDenom: "uakt",
		},
		{
			name:      "staled",
			event:     &oracletypes.EventPriceStaled{Id: oracletypes.DataID{Denom: "uusdc"}},
			wantType:  "staled",
			wantDenom: "uusdc",
		},
		{
			name:      "recovered",
			event:     &oracletypes.EventPriceRecovered{Id: oracletypes.DataID{Denom: "ubtc"}},
			wantType:  "recovered",
			wantDenom: "ubtc",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			updated, cmd = m.Update(busEventMsg{ok: true, event: test.event})
			m = lifecycleModelValue(t, updated)
			if cmd == nil {
				t.Fatal("typed oracle event did not rearm the bus subscription")
			}
			if got := m.oracle.Events[0]; got.Type != test.wantType || got.Denom != test.wantDenom || got.Detail != "" {
				t.Fatalf("oracle event log entry = %#v", got)
			}
		})
	}

	updated, cmd = m.Update(busEventMsg{ok: false})
	m = lifecycleModelValue(t, updated)
	if cmd != nil || m.subscriber != nil {
		t.Fatalf("closed bus remained armed: cmd=%v subscriber=%v", cmd != nil, m.subscriber)
	}
}

func TestOracleStateMessagesSeedAndPreserveDashboardState(t *testing.T) {
	priceTime := time.Date(2026, time.August, 12, 5, 0, 0, 0, time.UTC)
	prices := make([]oracletypes.PriceData, maxOracleEvents+1)
	for i := range prices {
		prices[i] = oracletypes.PriceData{
			ID: oracletypes.PriceDataRecordID{
				Source:    uint32(i + 1),
				Denom:     "uakt",
				BaseDenom: "usd",
				Timestamp: priceTime.Add(time.Duration(i) * time.Second),
			},
			State: oracletypes.PriceDataState{Price: math.LegacyNewDec(int64(i + 1))},
		}
	}
	aggregated := &oracletypes.QueryAggregatedPriceResponse{AggregatedPrice: oracletypes.AggregatedPrice{
		Denom: "uakt",
		TWAP:  math.LegacyMustNewDecFromStr("3.50"),
	}}
	m := Model{
		runtimeContext: context.Background(),
		oracle: OracleState{
			Aggregated: make(map[string]*oracletypes.EventAggregatedPrice),
		},
	}

	updated, cmd := m.Update(oracleStateMsg{state: &rpc.OracleState{
		Version:    "v2",
		Prices:     &oracletypes.QueryPricesResponse{Prices: prices},
		Aggregated: map[string]*oracletypes.QueryAggregatedPriceResponse{"uakt": aggregated},
	}})
	m = lifecycleModelValue(t, updated)
	if cmd == nil {
		t.Fatal("successful oracle refresh did not schedule its next refresh")
	}
	if m.oracle.Version != "v2" || len(m.oracle.Prices) != maxOracleEvents {
		t.Fatalf("seeded oracle state = version %q, prices %d", m.oracle.Version, len(m.oracle.Prices))
	}
	if first, last := m.oracle.Prices[0], m.oracle.Prices[maxOracleEvents-1]; first.Source != "1" || last.Source != "100" || first.Price != "1.000000000000000000" {
		t.Fatalf("seeded bounded prices = first %#v, last %#v", first, last)
	}
	if got := m.oracle.Aggregated["uakt"]; got == nil || got.Price.Denom != "uakt" ||
		got.Price.TWAP.String() != aggregated.AggregatedPrice.TWAP.String() {
		t.Fatalf("seeded aggregate = %#v", got)
	}

	firstPrices := append([]OraclePriceEntry(nil), m.oracle.Prices...)
	uusdc := &oracletypes.QueryAggregatedPriceResponse{AggregatedPrice: oracletypes.AggregatedPrice{
		Denom: "uusdc",
		TWAP:  math.LegacyMustNewDecFromStr("1.00"),
	}}
	updated, cmd = m.Update(oracleStateMsg{state: &rpc.OracleState{
		Prices: &oracletypes.QueryPricesResponse{Prices: []oracletypes.PriceData{{
			ID: oracletypes.PriceDataRecordID{Source: 999, Denom: "replacement"},
		}}},
		Aggregated: map[string]*oracletypes.QueryAggregatedPriceResponse{"uusdc": uusdc},
	}})
	m = lifecycleModelValue(t, updated)
	if cmd == nil || m.oracle.Version != "v2" || !reflect.DeepEqual(m.oracle.Prices, firstPrices) {
		t.Fatalf("periodic oracle refresh replaced stable identity: version=%q prices_changed=%t", m.oracle.Version, !reflect.DeepEqual(m.oracle.Prices, firstPrices))
	}
	if got := m.oracle.Aggregated["uusdc"]; got == nil || got.Price.TWAP.String() != uusdc.AggregatedPrice.TWAP.String() {
		t.Fatalf("periodic aggregate = %#v", got)
	}

	for _, msg := range []oracleStateMsg{
		{err: errors.New("oracle unavailable")},
		{},
	} {
		updated, cmd = m.Update(msg)
		m = lifecycleModelValue(t, updated)
		if cmd == nil || m.oracle.Version != "v2" || !reflect.DeepEqual(m.oracle.Prices, firstPrices) {
			t.Fatalf("failed oracle refresh changed last good state: %#v", m.oracle)
		}
	}
}

func TestBMEStateMessagesApplyPartialDataAndPreserveLastGoodState(t *testing.T) {
	status := &bmetypes.QueryStatusResponse{Status: bmetypes.MintStatusHaltCR}
	ledger := []bmetypes.QueryLedgerRecordEntry{{}, {}}
	m := Model{runtimeContext: context.Background()}

	updated, cmd := m.Update(bmeStateMsg{state: &rpc.BMEState{
		Status: status,
		Ledger: &bmetypes.QueryLedgerRecordsResponse{Records: ledger},
	}})
	m = lifecycleModelValue(t, updated)
	if cmd == nil || m.oracle.BMEStatus != status || !reflect.DeepEqual(m.oracle.BMELedger, ledger) {
		t.Fatalf("BME state = status %p, ledger %#v, scheduled %t", m.oracle.BMEStatus, m.oracle.BMELedger, cmd != nil)
	}

	updated, cmd = m.Update(bmeStateMsg{state: &rpc.BMEState{}})
	m = lifecycleModelValue(t, updated)
	if cmd == nil || m.oracle.BMEStatus != status || !reflect.DeepEqual(m.oracle.BMELedger, ledger) {
		t.Fatalf("partial BME refresh discarded last good data: %#v", m.oracle)
	}

	for _, msg := range []bmeStateMsg{
		{err: errors.New("BME unavailable")},
		{},
	} {
		updated, cmd = m.Update(msg)
		m = lifecycleModelValue(t, updated)
		if cmd == nil || m.oracle.BMEStatus != status || !reflect.DeepEqual(m.oracle.BMELedger, ledger) {
			t.Fatalf("failed BME refresh changed last good state: %#v", m.oracle)
		}
	}
}

func TestGovernanceMessagesRenderAndPreserveLastGoodState(t *testing.T) {
	params := governance.NewAllParams()
	params.Modules["gov"] = &governance.ModuleParams{RawJSON: []byte(`{
		"params": {
			"voting_period": "172800s",
			"min_deposit": [{"denom":"uakt","amount":"5000000"}],
			"max_deposit_period": "1209600s",
			"quorum": "0.334",
			"threshold": "0.5",
			"veto_threshold": "0.334"
		}
	}`)}
	m := Model{
		govModuleIdx:    0,
		govParamView:    viewport.New(viewport.WithWidth(80), viewport.WithHeight(20)),
		govProposalView: viewport.New(viewport.WithWidth(120), viewport.WithHeight(20)),
	}

	updated, cmd := m.Update(governanceParamsMsg{params: params})
	m = lifecycleModelValue(t, updated)
	if cmd != nil || m.governanceParams != params {
		t.Fatalf("governance params result = params %p, command %v", m.governanceParams, cmd != nil)
	}
	if content := stripANSI(m.govParamView.GetContent()); !strings.Contains(content, "Governance Parameters") || !strings.Contains(content, "Voting Period") {
		t.Fatalf("rendered governance parameters = %q", content)
	}
	lastGoodContent := m.govParamView.GetContent()
	lastGoodParams := m.governanceParams
	updated, cmd = m.Update(governanceParamsMsg{params: governance.NewAllParams(), err: errors.New("params unavailable")})
	m = lifecycleModelValue(t, updated)
	if cmd != nil || m.governanceParams != lastGoodParams || m.govParamView.GetContent() != lastGoodContent {
		t.Fatal("failed governance parameter refresh discarded the last good view")
	}

	m.govModuleIdx = -1
	m.govParamView.SetContent("stale")
	m.updateGovParamView()
	if content := m.govParamView.GetContent(); content != "" {
		t.Fatalf("invalid module selection content = %q, want empty", content)
	}
	m.govModuleIdx = 0
	m.governanceParams = governance.NewAllParams()
	m.updateGovParamView()
	if content := m.govParamView.GetContent(); content != "(no data)" {
		t.Fatalf("missing module content = %q", content)
	}
	m.governanceParams.Modules["gov"] = &governance.ModuleParams{Error: errors.New("module unavailable")}
	m.updateGovParamView()
	if content := m.govParamView.GetContent(); content != "Error: module unavailable" {
		t.Fatalf("module error content = %q", content)
	}

	oldProposals := &govv1.QueryProposalsResponse{Proposals: []*govv1.Proposal{{Id: 1, Title: "last good"}}}
	m.governanceProposals = oldProposals
	m.govProposalView.SetContent("last good proposals")
	wantErr := errors.New("proposals unavailable")
	updated, cmd = m.Update(governanceProposalsMsg{err: wantErr})
	m = lifecycleModelValue(t, updated)
	if cmd != nil || !errors.Is(m.governanceProposalsErr, wantErr) || m.governanceProposals != oldProposals ||
		m.govProposalView.GetContent() != "last good proposals" {
		t.Fatal("failed proposal refresh discarded the last good view")
	}

	proposals := &govv1.QueryProposalsResponse{Proposals: []*govv1.Proposal{{
		Id:     7,
		Title:  "raise provider minimum",
		Status: govv1.StatusVotingPeriod,
	}}}
	updated, cmd = m.Update(governanceProposalsMsg{proposals: proposals})
	m = lifecycleModelValue(t, updated)
	if cmd != nil || m.governanceProposalsErr != nil || m.governanceProposals != proposals ||
		!strings.Contains(stripANSI(m.govProposalView.GetContent()), "raise provider minimum") {
		t.Fatalf("successful proposal refresh = error %v, proposals %p, content %q", m.governanceProposalsErr, m.governanceProposals, m.govProposalView.GetContent())
	}
}

func TestSuccessfulChainSyncPrioritizesLeasedProvidersAndSchedulesChecks(t *testing.T) {
	now := time.Now()
	store := &monitorProviderStore{
		providers: map[string]*cache.CachedProvider{
			"regular": {
				HostURI:        "https://regular.example.test:8443",
				IsOnline:       true,
				Version:        "0.7.0",
				LastSeenOnline: now,
			},
			"leased": {
				HostURI:        "https://leased.example.test:8443",
				IsOnline:       true,
				Version:        "0.7.1",
				LastSeenOnline: now,
			},
		},
		priority: []string{"regular", "leased"},
	}
	m := Model{
		runtimeContext: context.Background(),
		cache:          store,
		providerTable:  newTestProviderTableModel(nil),
		loader: ProviderLoader{
			FirstRun: true,
			InFlight: make(map[string]bool),
		},
	}
	before := time.Now()

	updated, cmd := m.Update(chainSyncMsg{activeLeaseProviders: map[string]bool{"leased": true}})
	m = lifecycleModelValue(t, updated)
	if cmd == nil {
		t.Fatal("successful chain sync did not schedule provider checks")
	}
	if m.loader.LastSync.Before(before) || m.loader.Total != 2 || m.loader.Checked != 0 || !m.loader.Loading {
		t.Fatalf("chain sync loader state = %#v", m.loader)
	}
	if !reflect.DeepEqual(m.loader.Queue, []string{"leased", "regular"}) || !m.loader.InFlight["leased"] || !m.loader.InFlight["regular"] {
		t.Fatalf("leased-provider priority pipeline = queue %v, in-flight %v", m.loader.Queue, m.loader.InFlight)
	}
	if len(m.providers.Items) != 2 || m.providers.Items[0].Owner != "leased" {
		t.Fatalf("rebuilt provider dashboard = %#v", m.providers.Items)
	}

	emptyStore := &monitorProviderStore{providers: make(map[string]*cache.CachedProvider)}
	empty := Model{
		runtimeContext: context.Background(),
		cache:          emptyStore,
		providerTable:  newTestProviderTableModel(nil),
		loader:         ProviderLoader{InFlight: make(map[string]bool)},
	}
	updated, cmd = empty.Update(chainSyncMsg{activeLeaseProviders: map[string]bool{}})
	empty = lifecycleModelValue(t, updated)
	if cmd != nil || empty.loader.Loading || empty.loader.Total != 0 || empty.loader.LastSync.IsZero() {
		t.Fatalf("empty chain sync pipeline = %#v, command %v", empty.loader, cmd != nil)
	}
}
