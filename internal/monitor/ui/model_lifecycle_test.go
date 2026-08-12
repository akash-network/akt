package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gorilla/websocket"

	"pkg.akt.dev/akt/internal/monitor/cache"
	"pkg.akt.dev/akt/internal/monitor/rpc"
)

type recordingProviderStore struct {
	saves int
}

func (s *recordingProviderStore) HasProviders() bool { return false }

func (s *recordingProviderStore) GetProvider(string) (*cache.CachedProvider, bool) {
	return nil, false
}

func (s *recordingProviderStore) GetAllProviders() map[string]*cache.CachedProvider {
	return nil
}

func (s *recordingProviderStore) GetOnlineProviders() []*cache.CachedProvider { return nil }

func (s *recordingProviderStore) MarkProviderOnline(
	string, string,
	uint64, uint64,
	uint64, uint64,
	uint64, uint64,
	[]string,
) {
}

func (s *recordingProviderStore) MarkProviderOffline(string) {}

func (s *recordingProviderStore) SyncWithChain([]rpc.OnChainProvider) []string { return nil }

func (s *recordingProviderStore) GetProvidersDueForCheck() []string { return nil }

func (s *recordingProviderStore) GetProvidersByPriority() []string { return nil }

func (s *recordingProviderStore) ProviderCount() int { return 0 }

func (s *recordingProviderStore) OnlineCount() int { return 0 }

func (s *recordingProviderStore) Save() error {
	s.saves++
	return nil
}

func TestModelInitStartsProviderPipeline(t *testing.T) {
	t.Parallel()

	m := Model{}
	msg := m.Init()()
	cmds, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() message = %T, want tea.BatchMsg", msg)
	}

	// Eleven commands serve the network/oracle dashboards. The provider
	// pipeline adds one ordered load/sync command and three timer chains.
	if got, want := len(cmds), 15; got != want {
		t.Fatalf("Init() command count = %d, want %d", got, want)
	}
}

func TestNewModelDefaultsRuntimeContext(t *testing.T) {
	t.Parallel()

	db, err := cache.OpenDB(t.TempDir() + "/monitor.db")
	if err != nil {
		t.Fatalf("open monitor database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close monitor database: %v", err)
		}
	})
	providerCache, monikerCache, err := cache.OpenStores(db)
	if err != nil {
		t.Fatalf("open monitor stores: %v", err)
	}

	m := NewModel(ModelConfig{Cache: providerCache, MonikerCache: monikerCache})
	if m.runtimeContext != context.Background() {
		t.Fatalf("runtime context = %T, want context.Background", m.runtimeContext)
	}
}

func TestQuittingViewScreenOwnership(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		embedded   bool
		wantScreen bool
	}{
		{name: "standalone", wantScreen: true},
		{name: "embedded", embedded: true, wantScreen: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			view := (Model{quitting: true, embedded: tc.embedded}).View()
			if view.AltScreen != tc.wantScreen {
				t.Fatalf("View().AltScreen = %t, want %t", view.AltScreen, tc.wantScreen)
			}
		})
	}
}

func TestProviderPipelineTicksRearm(t *testing.T) {
	t.Parallel()

	store := &recordingProviderStore{}
	m := Model{
		cache: store,
		loader: ProviderLoader{
			InFlight: make(map[string]bool),
		},
	}

	for _, msg := range []tea.Msg{
		providerCheckTickMsg(time.Time{}),
		chainSyncTickMsg(time.Time{}),
		cacheSaveTickMsg(time.Time{}),
	} {
		updated, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("Update(%T) did not rearm its schedule", msg)
		}
		m = updated.(Model)
	}

	if got, want := store.saves, 1; got != want {
		t.Fatalf("cache saves = %d, want %d", got, want)
	}
}

func TestProviderRefreshStartsChainSync(t *testing.T) {
	t.Parallel()

	m := Model{hubTab: HubProvider}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("provider refresh did not start a chain sync")
	}
}

func TestConsensusInitialFailureSchedulesReconnectAndRecovers(t *testing.T) {
	const timeout = 5 * time.Second

	var validatorRequests atomic.Int32
	var websocketConnections atomic.Int32
	secondSubscribed := make(chan struct{})
	peerErrors := make(chan error, 2)
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		if validatorRequests.Add(1) == 1 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"result":{"validators":[{"address":"AA","voting_power":"10"}],"total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			select {
			case peerErrors <- fmt.Errorf("upgrade websocket: %w", err):
			default:
			}
			return
		}
		defer conn.Close()

		connection := websocketConnections.Add(1)
		if connection == 2 {
			for range 2 {
				var request struct {
					ID int `json:"id"`
				}
				if err := conn.ReadJSON(&request); err != nil {
					peerErrors <- fmt.Errorf("read subscription: %w", err)
					return
				}
				if err := conn.WriteJSON(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"result":  map[string]any{},
				}); err != nil {
					peerErrors <- fmt.Errorf("acknowledge subscription: %w", err)
					return
				}
			}
			close(secondSubscribed)
		}

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	m := Model{
		client:         rpc.NewClient(server.URL, server.URL),
		runtimeContext: ctx,
	}

	failed := m.connectWebSocket()
	failedMsg, ok := failed.(stateMsg)
	if !ok || failedMsg.err == nil {
		t.Fatalf("first connection result = %#v, want stateMsg error", failed)
	}
	updated, retryCmd := m.Update(failedMsg)
	m = lifecycleModelValue(t, updated)
	if retryCmd == nil {
		t.Fatal("initial validator failure did not schedule reconnect")
	}
	if m.state == nil || m.state.Error == nil {
		t.Fatal("initial validator failure was not retained for rendering")
	}

	retryMsg := retryCmd()
	updated, connectCmd := m.Update(retryMsg)
	m = lifecycleModelValue(t, updated)
	if connectCmd == nil {
		t.Fatal("reconnect timer did not start another connection attempt")
	}

	connected := connectCmd()
	updated, waitCmd := m.Update(connected)
	m = lifecycleModelValue(t, updated)
	if !m.wsConnected {
		t.Fatal("transient validator failure did not recover")
	}
	if m.state == nil || m.state.Error != nil {
		t.Fatalf("visible consensus error after recovery = %v, want nil", m.state)
	}
	select {
	case <-secondSubscribed:
	case <-time.After(timeout):
		t.Fatal("second connection did not restore subscriptions")
	}
	if got := validatorRequests.Load(); got != 2 {
		t.Fatalf("validator requests = %d, want 2", got)
	}

	cancel()
	closed := make(chan tea.Msg, 1)
	go func() { closed <- waitCmd() }()
	select {
	case msg := <-closed:
		updated, retryAfterCancel := m.Update(msg)
		m = lifecycleModelValue(t, updated)
		if retryAfterCancel != nil {
			t.Fatal("consensus cancellation scheduled a reconnect")
		}
	case <-time.After(timeout):
		t.Fatal("consensus subscription did not close after cancellation")
	}
	if m.wsConnected {
		t.Fatal("model remained connected after cancellation")
	}
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestConsensusCancellationDoesNotRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := Model{runtimeContext: ctx}

	updated, cmd := m.Update(stateMsg{err: fmt.Errorf("connect: %w", context.Canceled)})
	m = lifecycleModelValue(t, updated)
	if cmd != nil {
		t.Fatal("canceled initial connection scheduled a reconnect")
	}
	if m.state == nil || !errors.Is(m.state.Error, context.Canceled) {
		t.Fatalf("visible error = %v, want context cancellation", m.state)
	}
}

func TestConsensusReconnectTimerStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tasks := NewRuntimeTaskGroup()
	m := Model{
		runtimeContext:        ctx,
		runtimeTasks:          tasks,
		consensusRetryAttempt: 10, // selects the eight-second cap
	}

	reconnect := m.scheduleConsensusReconnect()
	if reconnect == nil {
		t.Fatal("active runtime did not schedule a consensus reconnect")
	}
	cancel()

	done := make(chan tea.Msg, 1)
	go func() { done <- reconnect() }()
	select {
	case msg := <-done:
		if msg != nil {
			t.Fatalf("canceled reconnect result = %T, want nil", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("reconnect timer did not stop after context cancellation")
	}
	tasks.StopAndWait()
}

func TestInitialSigningUsesValidatorSetAtCommitHeight(t *testing.T) {
	validatorHeight := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/commit":
			height := r.URL.Query().Get("height")
			if height == "" {
				_, _ = fmt.Fprint(w, `{"result":{"signed_header":{"commit":{"height":"10"}}}}`)
				return
			}
			if height != "9" {
				http.Error(w, "unexpected commit height", http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprint(w, `{"result":{"signed_header":{"commit":{"height":"9","signatures":[{"block_id_flag":2,"validator_address":"A1B2"}]}}}}`)
		case "/validators":
			validatorHeight <- r.URL.Query().Get("height")
			_, _ = fmt.Fprint(w, `{"result":{"block_height":"9","validators":[{"address":"A1B2","voting_power":"10"}],"count":"1","total":"1"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	m := Model{
		client:         rpc.NewClient(server.URL, server.URL),
		runtimeContext: context.Background(),
		valSignHistory: make(map[int][]bool),
	}
	raw := m.fetchInitialSigning()
	msg, ok := raw.(initialSigningMsg)
	if !ok {
		t.Fatalf("initial signing result = %T, want initialSigningMsg", raw)
	}
	if msg.err != nil {
		t.Fatalf("initial signing error = %v", msg.err)
	}
	if msg.height != 9 || len(msg.validators) != 1 || msg.validators[0].Address != "A1B2" {
		t.Fatalf("initial signing result = %#v", msg)
	}
	select {
	case height := <-validatorHeight:
		if height != "9" {
			t.Fatalf("validator height = %q, want sampled commit height 9", height)
		}
	default:
		t.Fatal("initial signing did not fetch validators")
	}

	updated, _ := m.Update(msg)
	m = lifecycleModelValue(t, updated)
	if history := m.valSignHistory[0]; len(history) != 1 || !history[0] {
		t.Fatalf("seeded signing history = %v, want signed validator", history)
	}
}

func lifecycleModelValue(t *testing.T, model tea.Model) Model {
	t.Helper()
	switch value := model.(type) {
	case Model:
		return value
	case *Model:
		return *value
	default:
		t.Fatalf("updated model type = %T, want ui.Model", model)
		return Model{}
	}
}
