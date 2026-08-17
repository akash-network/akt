package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/governance"
	"pkg.akt.dev/akt/internal/monitor/rpc"
	"pkg.akt.dev/go/util/pubsub"
)

func TestFetchProposerReportsTransportFailureAndIdentity(t *testing.T) {
	t.Run("validator failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "validator service unavailable", http.StatusServiceUnavailable)
		}))
		t.Cleanup(server.Close)

		msg, ok := (Model{
			client:         rpc.NewClient(server.URL, server.URL),
			runtimeContext: context.Background(),
		}).fetchProposer().(proposerMsg)
		if !ok || msg.err == nil {
			t.Fatalf("fetchProposer() = %#v, want proposer error", msg)
		}
	})

	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/validators":
				_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"10"},{"address":"BB","voting_power":"20"}],"count":"2","total":"2"}}`)
			case "/consensus_state":
				_, _ = fmt.Fprint(w, `{"result":{"round_state":{"height/round/step":"42/1/6","start_time":"2026-08-11T12:00:00Z","height_vote_set":[{"round":1,"prevotes":["vote-a","vote-b"],"precommits":["vote-a","vote-b"]}],"proposer":{"address":"BB","index":1}}}}`)
			default:
				http.NotFound(w, request)
			}
		}))
		t.Cleanup(server.Close)

		msg, ok := (Model{
			client:         rpc.NewClient(server.URL, server.URL),
			runtimeContext: context.Background(),
		}).fetchProposer().(proposerMsg)
		if !ok || msg.err != nil {
			t.Fatalf("fetchProposer() = %#v, want success", msg)
		}
		if msg.height != 42 || msg.proposerIndex != 1 || msg.proposerAddr != "BB" {
			t.Fatalf("proposer identity = %#v", msg)
		}
	})
}

func TestFetchInitialSigningReportsEachReadFailure(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
	}{
		{
			name: "commit unavailable",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "commit unavailable", http.StatusServiceUnavailable)
			}),
		},
		{
			name: "historical validators unavailable",
			handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/commit":
					if request.URL.Query().Get("height") == "" {
						_, _ = fmt.Fprint(w, `{"result":{"signed_header":{"commit":{"height":"10"}}}}`)
						return
					}
					_, _ = fmt.Fprint(w, `{"result":{"signed_header":{"commit":{"height":"9","signatures":[]}}}}`)
				case "/validators":
					http.Error(w, "validators unavailable", http.StatusServiceUnavailable)
				default:
					http.NotFound(w, request)
				}
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			t.Cleanup(server.Close)
			msg, ok := (Model{
				client:         rpc.NewClient(server.URL, server.URL),
				runtimeContext: context.Background(),
			}).fetchInitialSigning().(initialSigningMsg)
			if !ok || msg.err == nil {
				t.Fatalf("fetchInitialSigning() = %#v, want error", msg)
			}
		})
	}
}

func TestCanceledAuxiliaryFetchesReportCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := Model{
		client:         rpc.NewClient("http://127.0.0.1:1", "http://127.0.0.1:1"),
		runtimeContext: ctx,
	}

	tests := []struct {
		name string
		call func() tea.Msg
		err  func(tea.Msg) error
	}{
		{name: "oracle", call: m.fetchOracleState, err: func(msg tea.Msg) error { return msg.(oracleStateMsg).err }},
		{name: "BME", call: m.fetchBMEState, err: func(msg tea.Msg) error { return msg.(bmeStateMsg).err }},
		{name: "governance parameters", call: m.fetchGovernanceParams, err: func(msg tea.Msg) error { return msg.(governanceParamsMsg).err }},
		{name: "governance proposals", call: m.fetchGovernanceProposals, err: func(msg tea.Msg) error { return msg.(governanceProposalsMsg).err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.err(test.call()); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled fetch error = %v, want context cancellation", err)
			}
		})
	}
}

func TestAuxiliaryFetchesSurfaceClientErrors(t *testing.T) {
	wantErr := errors.New("auxiliary query failed")
	m := Model{
		runtimeContext: context.Background(),
		oracleStateQuery: func(context.Context) (*rpc.OracleState, error) {
			return nil, wantErr
		},
		bmeStateQuery: func(context.Context) (*rpc.BMEState, error) {
			return nil, wantErr
		},
		governanceParamsQuery: func(context.Context) (*governance.AllParams, error) {
			return nil, wantErr
		},
	}

	for _, result := range []error{
		m.fetchOracleState().(oracleStateMsg).err,
		m.fetchBMEState().(bmeStateMsg).err,
		m.fetchGovernanceParams().(governanceParamsMsg).err,
	} {
		if !errors.Is(result, wantErr) {
			t.Fatalf("auxiliary fetch error = %v, want %v", result, wantErr)
		}
	}
}

func TestAuxiliaryFetchesReturnSuccessfulState(t *testing.T) {
	wantOracle := &rpc.OracleState{Version: "v2"}
	wantBME := &rpc.BMEState{}
	wantParams := governance.NewAllParams()
	m := Model{
		runtimeContext: context.Background(),
		oracleStateQuery: func(context.Context) (*rpc.OracleState, error) {
			return wantOracle, nil
		},
		bmeStateQuery: func(context.Context) (*rpc.BMEState, error) {
			return wantBME, nil
		},
		governanceParamsQuery: func(context.Context) (*governance.AllParams, error) {
			return wantParams, nil
		},
	}

	if msg := m.fetchOracleState().(oracleStateMsg); msg.err != nil || msg.state != wantOracle {
		t.Fatalf("oracle result = %#v", msg)
	}
	if msg := m.fetchBMEState().(bmeStateMsg); msg.err != nil || msg.state != wantBME {
		t.Fatalf("BME result = %#v", msg)
	}
	if msg := m.fetchGovernanceParams().(governanceParamsMsg); msg.err != nil || msg.params != wantParams {
		t.Fatalf("governance params result = %#v", msg)
	}
}

func TestCanceledRuntimeTicksDoNotRearmOrWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &recordingProviderStore{}
	m := Model{runtimeContext: ctx, cache: store}

	for _, msg := range []tea.Msg{
		consensusReconnectMsg{},
		providerCheckTickMsg(time.Time{}),
		chainSyncTickMsg(time.Time{}),
		cacheSaveTickMsg(time.Time{}),
		governanceProposalsTickMsg(time.Time{}),
		governanceParamsTickMsg(time.Time{}),
	} {
		updated, cmd := m.Update(msg)
		m = lifecycleModelValue(t, updated)
		if cmd != nil {
			t.Fatalf("canceled Update(%T) rearmed work", msg)
		}
	}
	if store.saves != 0 {
		t.Fatalf("canceled cache tick wrote %d times", store.saves)
	}
}

func TestWaitForSnapshotDeliversTheReceivedSnapshot(t *testing.T) {
	want := rpc.ConsensusSnapshot{State: &consensus.State{Height: 42}}
	snapshots := make(chan rpc.ConsensusSnapshot, 1)
	snapshots <- want

	msg := (Model{
		snapshotCh:     snapshots,
		runtimeContext: context.Background(),
	}).waitForSnapshot()().(consensusSnapshotMsg)
	if !msg.ok || msg.snapshot.State != want.State {
		t.Fatalf("snapshot message = %#v, want state pointer %p", msg, want.State)
	}
}

func TestWaitForSnapshotStopsWhenRuntimeIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := (Model{
		snapshotCh:     make(chan rpc.ConsensusSnapshot),
		runtimeContext: ctx,
	}).waitForSnapshot()
	if cmd == nil {
		t.Fatal("waitForSnapshot() returned nil for an active snapshot stream")
	}

	result := make(chan tea.Msg, 1)
	go func() {
		result <- cmd()
	}()

	select {
	case got := <-result:
		msg, ok := got.(consensusSnapshotMsg)
		if !ok {
			t.Fatalf("waitForSnapshot() message = %T, want consensusSnapshotMsg", got)
		}
		if msg.ok {
			t.Fatalf("canceled snapshot message = %#v, want closed-stream signal", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("waitForSnapshot() did not stop after runtime cancellation")
	}
}

func TestActiveViewPreservesScreenOwnership(t *testing.T) {
	for _, test := range []struct {
		name          string
		embedded      bool
		wantAltScreen bool
	}{
		{name: "standalone", wantAltScreen: true},
		{name: "embedded", embedded: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := (Model{
				client:   rpc.NewClient("http://rpc.example.test", "http://rest.example.test"),
				embedded: test.embedded,
			}).View()
			if view.AltScreen != test.wantAltScreen {
				t.Fatalf("active View().AltScreen = %t, want %t", view.AltScreen, test.wantAltScreen)
			}
		})
	}
}

func TestNilContextConsensusReconnectCompletes(t *testing.T) {
	m := Model{}
	cmd := m.scheduleConsensusReconnect()
	if cmd == nil {
		t.Fatal("nil runtime context disabled the standalone reconnect timer")
	}
	if _, ok := cmd().(consensusReconnectMsg); !ok {
		t.Fatalf("reconnect result = %T, want consensusReconnectMsg", cmd())
	}
}

func TestWaitForBusEventStopsForEveryLifetimeBoundary(t *testing.T) {
	t.Run("event", func(t *testing.T) {
		bus := pubsub.NewBus()
		t.Cleanup(bus.Close)
		sub, err := bus.Subscribe()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan tea.Msg, 1)
		go func() { result <- (Model{subscriber: sub, runtimeContext: ctx}).waitForBusEvent()() }()
		want := struct{ name string }{name: "oracle-event"}
		if err := bus.Publish(want); err != nil {
			t.Fatal(err)
		}
		msg := (<-result).(busEventMsg)
		if !msg.ok || msg.event != want {
			t.Fatalf("bus event = %#v, want %#v", msg, want)
		}
	})

	t.Run("subscriber closes", func(t *testing.T) {
		bus := pubsub.NewBus()
		t.Cleanup(bus.Close)
		sub, err := bus.Subscribe()
		if err != nil {
			t.Fatal(err)
		}
		result := make(chan tea.Msg, 1)
		go func() {
			result <- (Model{subscriber: sub, runtimeContext: context.Background()}).waitForBusEvent()()
		}()
		sub.Close()
		msg := (<-result).(busEventMsg)
		if msg.ok {
			t.Fatalf("closed subscriber result = %#v, want terminal result", msg)
		}
	})

	t.Run("runtime cancels", func(t *testing.T) {
		bus := pubsub.NewBus()
		t.Cleanup(bus.Close)
		sub, err := bus.Subscribe()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan tea.Msg, 1)
		go func() { result <- (Model{subscriber: sub, runtimeContext: ctx}).waitForBusEvent()() }()
		cancel()
		msg := (<-result).(busEventMsg)
		if msg.ok {
			t.Fatalf("canceled runtime result = %#v, want terminal result", msg)
		}
	})
}

func TestProviderDetailDiscardsSuccessAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := Model{
		runtimeContext: ctx,
		providerStatusGRPC: func(context.Context, string, bool) ([]rpc.ProviderNodeWithGPU, error) {
			cancel()
			return []rpc.ProviderNodeWithGPU{{Name: "stale-node"}}, nil
		},
	}

	msg := m.fetchProviderDetail("https://provider.example.test", 9)().(providerDetailMsg)
	if !errors.Is(msg.err, context.Canceled) || len(msg.nodes) != 0 {
		t.Fatalf("canceled detail result = %#v", msg)
	}
}

func TestProviderDetailReturnsCorrelatedNodes(t *testing.T) {
	want := []rpc.ProviderNodeWithGPU{{Name: "gpu-node", GPUAvailable: 1}}
	m := Model{
		runtimeContext: context.Background(),
		providerStatusGRPC: func(context.Context, string, bool) ([]rpc.ProviderNodeWithGPU, error) {
			return want, nil
		},
	}

	msg := m.fetchProviderDetail("https://provider.example.test", 10)().(providerDetailMsg)
	if msg.err != nil || msg.hostURI != "https://provider.example.test" || msg.requestID != 10 || len(msg.nodes) != 1 {
		t.Fatalf("provider detail result = %#v", msg)
	}
}

func TestSeedSigningHistoryRejectsMissingValidatorIdentity(t *testing.T) {
	m := Model{valSignHistory: make(map[int][]bool)}
	m.seedSigningHistory(map[string]bool{"AA": true}, nil)
	if len(m.valSignHistory) != 0 {
		t.Fatalf("signing history without validator set = %v", m.valSignHistory)
	}

	m.seedSigningHistory(map[string]bool{"AA": true}, []consensus.Validator{{Address: "AA"}})
	m.seedSigningHistory(map[string]bool{"BB": true}, []consensus.Validator{{Address: "BB"}})
	if history := m.valSignHistory[0]; len(history) != 1 || !history[0] {
		t.Fatalf("existing signing sample was replaced: %v", history)
	}
}
