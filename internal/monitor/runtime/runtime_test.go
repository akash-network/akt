package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/gorilla/websocket"
	bolt "go.etcd.io/bbolt"

	aktevents "pkg.akt.dev/akt/internal/events"
	monitorcache "pkg.akt.dev/akt/internal/monitor/cache"
	"pkg.akt.dev/go/util/pubsub"
)

type runtimeCometClientStub struct {
	sdkclient.CometRPC
	running    bool
	startErr   error
	startCalls int
	stopCalls  int
}

func (client *runtimeCometClientStub) Start() error {
	client.startCalls++
	if client.startErr != nil {
		return client.startErr
	}
	client.running = true
	return nil
}

func (client *runtimeCometClientStub) Stop() error {
	client.stopCalls++
	client.running = false
	return nil
}

func (client *runtimeCometClientStub) IsRunning() bool { return client.running }

type runtimeEventServiceStub struct {
	shutdownCalls int
}

func (service *runtimeEventServiceStub) Shutdown() {
	service.shutdownCalls++
}

func TestRunRejectsMissingBoundaryInputs(t *testing.T) {
	if err := Run(Config{}); err == nil || err.Error() != "monitor RPC endpoint is required" {
		t.Fatalf("Run without RPC error = %v", err)
	}
	if err := Run(Config{RPCEndpoint: "http://127.0.0.1:1"}); err == nil || err.Error() != "monitor cache directory is required" {
		t.Fatalf("Run without cache error = %v", err)
	}
}

func TestBuildModelFailsClosedWhenCometClientConstructionFails(t *testing.T) {
	cacheDir := t.TempDir()
	model, cleanup, err := buildModel(Config{
		RPCEndpoint: "http://[invalid-endpoint",
		CacheDir:    cacheDir,
	})
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil || !strings.Contains(err.Error(), "create monitor CometBFT event client") {
		t.Fatalf("buildModel() = model %T, error %v; want client-construction error", model, err)
	}
	if model != nil || cleanup != nil {
		t.Fatalf("failed build returned model %T and cleanup %v", model, cleanup != nil)
	}
	requireRuntimeDatabaseReopen(t, cacheDir)
}

func TestBuildModelStartFailureReleasesAllCreatedResources(t *testing.T) {
	wantErr := errors.New("websocket start failed")
	cacheDir := t.TempDir()
	client := &runtimeCometClientStub{startErr: wantErr}
	var eventBus pubsub.Bus
	eventServiceCalls := 0

	model, cleanup, err := buildModelWithEvents(
		Config{RPCEndpoint: "http://rpc.example.test", CacheDir: cacheDir},
		monitorcache.OpenStores,
		func(string, string) (monitorCometClient, error) {
			return client, nil
		},
		func(_ context.Context, _ sdkclient.CometRPC, _ string, bus pubsub.Bus) (aktevents.Service, error) {
			eventServiceCalls++
			eventBus = bus
			return &runtimeEventServiceStub{}, nil
		},
	)
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "start monitor CometBFT event client") {
		t.Fatalf("buildModelWithEvents() = model %T, error %v; want %v", model, err, wantErr)
	}
	if model != nil || cleanup != nil {
		t.Fatalf("failed build returned model %T and cleanup %v", model, cleanup != nil)
	}
	if client.startCalls != 1 || client.stopCalls != 0 || client.running {
		t.Fatalf("failed CometBFT lifecycle = starts %d, stops %d, running %t", client.startCalls, client.stopCalls, client.running)
	}
	if eventServiceCalls != 0 || eventBus != nil {
		t.Fatalf("event service started after client failure: calls %d, bus %v", eventServiceCalls, eventBus != nil)
	}
	requireRuntimeDatabaseReopen(t, cacheDir)
}

func TestBuildModelEventServiceFailureReleasesStartedResources(t *testing.T) {
	wantErr := errors.New("subscribe enqueue failed")
	cacheDir := t.TempDir()
	client := &runtimeCometClientStub{}
	var eventBus pubsub.Bus

	model, cleanup, err := buildModelWithEvents(
		Config{RPCEndpoint: "http://rpc.example.test", CacheDir: cacheDir},
		monitorcache.OpenStores,
		func(string, string) (monitorCometClient, error) {
			return client, nil
		},
		func(_ context.Context, _ sdkclient.CometRPC, _ string, bus pubsub.Bus) (aktevents.Service, error) {
			eventBus = bus
			return nil, wantErr
		},
	)
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "start monitor event service") {
		t.Fatalf("buildModelWithEvents() = model %T, error %v; want %v", model, err, wantErr)
	}
	if model != nil || cleanup != nil {
		t.Fatalf("failed build returned model %T and cleanup %v", model, cleanup != nil)
	}
	if eventBus == nil {
		t.Fatal("event-service factory did not receive the runtime bus")
	}
	if client.startCalls != 1 || client.stopCalls != 1 || client.running {
		t.Fatalf("CometBFT lifecycle = starts %d, stops %d, running %t", client.startCalls, client.stopCalls, client.running)
	}
	select {
	case <-eventBus.Done():
	default:
		t.Fatal("event-service failure left the runtime bus active")
	}
	requireRuntimeDatabaseReopen(t, cacheDir)
}

func TestBuildModelCleanupIsIdempotent(t *testing.T) {
	cacheDir := t.TempDir()
	client := &runtimeCometClientStub{}
	service := &runtimeEventServiceStub{}
	var eventBus pubsub.Bus

	model, cleanup, err := buildModelWithEvents(
		Config{RPCEndpoint: "http://rpc.example.test", CacheDir: cacheDir},
		monitorcache.OpenStores,
		func(string, string) (monitorCometClient, error) {
			return client, nil
		},
		func(_ context.Context, _ sdkclient.CometRPC, _ string, bus pubsub.Bus) (aktevents.Service, error) {
			eventBus = bus
			return service, nil
		},
	)
	if err != nil || model == nil || cleanup == nil {
		t.Fatalf("buildModelWithEvents() = model %T, cleanup %v, error %v", model, cleanup != nil, err)
	}

	cleanup()
	cleanup()

	if service.shutdownCalls != 1 {
		t.Fatalf("idempotent cleanup = service shutdowns %d, want 1", service.shutdownCalls)
	}
	if client.startCalls != 1 || client.stopCalls != 1 || client.running {
		t.Fatalf("idempotent CometBFT cleanup = starts %d, stops %d, running %t", client.startCalls, client.stopCalls, client.running)
	}
	select {
	case <-eventBus.Done():
	default:
		t.Fatal("cleanup left the runtime bus active")
	}
	requireRuntimeDatabaseReopen(t, cacheDir)
}

func requireRuntimeDatabaseReopen(t *testing.T, cacheDir string) {
	t.Helper()

	db, err := monitorcache.OpenDB(filepath.Join(cacheDir, "monitor.db"))
	if err != nil {
		t.Fatalf("monitor startup failure retained database lock: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close reopened monitor database: %v", err)
	}
}

func TestRunBuildsActiveMonitorAndCleansUp(t *testing.T) {
	peerClosed := make(chan struct{})
	peerErrors := make(chan error, 1)
	server := newRuntimeEventServer(t, peerClosed, peerErrors)

	cacheDir := t.TempDir()
	want := errors.New("program stopped")
	called := false
	err := run(Config{
		RPCEndpoint:      server.URL,
		RESTEndpoint:     server.URL,
		CacheDir:         cacheDir,
		Insecure:         true,
		InitialDashboard: "provider",
	}, func(model tea.Model) error {
		called = true
		if model == nil {
			t.Fatal("run supplied a nil monitor model")
		}
		if !model.View().AltScreen {
			t.Fatal("standalone monitor did not claim the alternate screen")
		}
		return want
	})
	if !called || !errors.Is(err, want) {
		t.Fatalf("run called = %t, error = %v", called, err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "monitor.db")); err != nil {
		t.Fatalf("monitor cache was not created: %v", err)
	}
	select {
	case <-peerClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor run did not close its event WebSocket")
	}
	select {
	case err := <-peerErrors:
		t.Fatal(err)
	default:
	}
}

func newRuntimeEventServer(t *testing.T, peerClosed chan<- struct{}, peerErrors chan<- error) *httptest.Server {
	t.Helper()

	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			reportEventPeerError(peerErrors, "upgrade websocket: %v", err)
			close(peerClosed)
			return
		}
		defer close(peerClosed)
		defer connection.Close()

		for {
			var call eventRPCRequest
			if err := connection.ReadJSON(&call); err != nil {
				return
			}
			if call.Method != "subscribe" && call.Method != "unsubscribe_all" {
				reportEventPeerError(peerErrors, "unexpected RPC method %q", call.Method)
				return
			}
			if err := writeEventRPCResult(connection, call.ID); err != nil {
				if call.Method == "subscribe" {
					reportEventPeerError(peerErrors, "write subscribe result: %v", err)
				}
				return
			}
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestBuildModelReportsCacheFailures(t *testing.T) {
	parent := t.TempDir()
	file := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(file, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := buildModel(Config{
		RPCEndpoint: "http://127.0.0.1:1",
		CacheDir:    filepath.Join(file, "cache"),
	})
	if err == nil {
		t.Fatal("buildModel accepted an uncreatable cache directory")
	}

	legacyDir := t.TempDir()
	legacy := filepath.Join(legacyDir, "top.db")
	if err := os.WriteFile(legacy, []byte("not bbolt"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = buildModel(Config{RPCEndpoint: "http://127.0.0.1:1", CacheDir: legacyDir})
	if err == nil {
		t.Fatal("buildModel accepted an invalid migrated cache")
	}
	if _, statErr := os.Stat(filepath.Join(legacyDir, "monitor.db")); statErr != nil {
		t.Fatalf("legacy cache was not migrated before validation: %v", statErr)
	}
}

func TestBuildModelReleasesDBWhenStoreInitializationFails(t *testing.T) {
	cacheDir := t.TempDir()
	want := errors.New("store initialization failed")
	_, _, err := buildModelWith(
		Config{RPCEndpoint: "http://127.0.0.1:1", CacheDir: cacheDir},
		func(*bolt.DB) (*monitorcache.ProviderCache, *monitorcache.MonikerCache, error) {
			return nil, nil, want
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("buildModelWith() error = %v, want %v", err, want)
	}

	db, err := monitorcache.OpenDB(filepath.Join(cacheDir, "monitor.db"))
	if err != nil {
		t.Fatalf("monitor database remained locked after initialization failure: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildModelStartsAndStopsRealEventService(t *testing.T) {
	const timeout = 5 * time.Second
	subscribed := make(chan string, 1)
	unsubscribed := make(chan struct{}, 1)
	peerClosed := make(chan struct{}, 1)
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			reportEventPeerError(peerErrors, "upgrade websocket: %v", err)
			return
		}
		defer func() {
			_ = connection.Close()
			peerClosed <- struct{}{}
		}()

		for {
			var call eventRPCRequest
			if err := connection.ReadJSON(&call); err != nil {
				return
			}
			switch call.Method {
			case "subscribe":
				query := call.Params["query"]
				if query == "" {
					reportEventPeerError(peerErrors, "subscribe query is empty")
					return
				}
				if err := writeEventRPCResult(connection, call.ID); err != nil {
					reportEventPeerError(peerErrors, "write subscribe result: %v", err)
					return
				}
				subscribed <- query
			case "unsubscribe_all":
				select {
				case unsubscribed <- struct{}{}:
				default:
				}
				// Cleanup only promises that unsubscribe_all crossed the write
				// boundary before the client closes. Its response can race with
				// that close, and the flush marker is itself idempotent.
				_ = writeEventRPCResult(connection, call.ID)
			default:
				reportEventPeerError(peerErrors, "unexpected RPC method %q", call.Method)
				return
			}
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	model, cleanup, err := buildModel(Config{
		RPCEndpoint:  server.URL,
		RESTEndpoint: server.URL,
		CacheDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			cleanup()
		}
	})
	if model == nil {
		t.Fatal("buildModel returned a nil model")
	}
	select {
	case query := <-subscribed:
		if query == "" {
			t.Fatal("event service sent an empty subscription")
		}
	case <-time.After(timeout):
		t.Fatal("event service did not subscribe")
	}

	cleanup()
	cleaned = true
	for name, signal := range map[string]<-chan struct{}{
		"unsubscribe": unsubscribed,
		"peer close":  peerClosed,
	} {
		select {
		case <-signal:
		case <-time.After(timeout):
			t.Fatalf("timed out waiting for %s", name)
		}
	}
	select {
	case err := <-peerErrors:
		t.Fatal(err)
	default:
	}
}

type eventRPCRequest struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params map[string]string `json:"params"`
}

func writeEventRPCResult(connection *websocket.Conn, id json.RawMessage) error {
	return connection.WriteJSON(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  map[string]any  `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  map[string]any{},
	})
}

func reportEventPeerError(errors chan<- error, format string, args ...any) {
	select {
	case errors <- fmt.Errorf(format, args...):
	default:
	}
}

type quitModel struct{}

func (quitModel) Init() tea.Cmd { return tea.Quit }

func (model quitModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return model, tea.Quit }

func (quitModel) View() tea.View { return tea.NewView("") }

func TestRunProgramPropagatesProgramCompletion(t *testing.T) {
	input, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create program input pipe: %v", err)
	}
	if err := inputWriter.Close(); err != nil {
		_ = input.Close()
		t.Fatalf("close program input writer: %v", err)
	}
	previousInput := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = previousInput
		_ = input.Close()
	})

	if err := runProgram(quitModel{}); err != nil {
		t.Fatalf("runProgram: %v", err)
	}
}
