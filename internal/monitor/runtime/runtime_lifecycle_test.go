package runtime

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gorilla/websocket"
	bolt "go.etcd.io/bbolt"
	providerv1beta4 "pkg.akt.dev/go/node/provider/v1beta4"

	monitorcache "pkg.akt.dev/akt/internal/monitor/cache"
	monitorrpc "pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestBuildModelClosesDatabaseAfterStoreInitializationFailure(t *testing.T) {
	cacheDir := t.TempDir()
	want := errors.New("bucket initialization failed")

	_, _, err := buildModelWith(Config{
		RPCEndpoint: "http://127.0.0.1:1",
		CacheDir:    cacheDir,
	}, func(*bolt.DB) (*monitorcache.ProviderCache, *monitorcache.MonikerCache, error) {
		return nil, nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("buildModelWith() error = %v, want %v", err, want)
	}

	// Reopening the same bbolt path proves the failed boundary released its
	// file lock instead of leaking a database handle.
	db, err := monitorcache.OpenDB(filepath.Join(cacheDir, "monitor.db"))
	if err != nil {
		t.Fatalf("reopen monitor database after initialization failure: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close reopened monitor database: %v", err)
	}
}

func TestBuildModelOwnsRealCometWebSocketLifecycle(t *testing.T) {
	const timeout = 5 * time.Second

	methods := make(chan string, 4)
	peerDone := make(chan struct{})
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			peerErrors <- err
			close(peerDone)
			return
		}
		defer close(peerDone)
		defer conn.Close()
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			peerErrors <- err
			return
		}

		for {
			var rpcRequest struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Method  string          `json:"method"`
			}
			if err := conn.ReadJSON(&rpcRequest); err != nil {
				return
			}
			if rpcRequest.JSONRPC != "2.0" || len(rpcRequest.ID) == 0 || rpcRequest.Method == "" {
				peerErrors <- errors.New("CometBFT request omitted JSON-RPC identity")
				return
			}
			methods <- rpcRequest.Method
			if err := conn.WriteJSON(struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Result  struct{}        `json:"result"`
			}{JSONRPC: "2.0", ID: rpcRequest.ID}); err != nil {
				if rpcRequest.Method == "subscribe" {
					peerErrors <- err
				}
				return
			}
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	cacheDir := t.TempDir()
	model, cleanup, err := buildModel(Config{RPCEndpoint: server.URL, CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("buildModel() error = %v", err)
	}
	if model == nil || cleanup == nil {
		t.Fatal("buildModel() did not return the model and cleanup boundary")
	}
	if method := receiveRuntimeRPCMethod(t, methods, timeout); method != "subscribe" {
		t.Fatalf("first CometBFT method = %q, want subscribe", method)
	}

	cleanup()
	if method := receiveRuntimeRPCMethod(t, methods, timeout); method != "unsubscribe_all" {
		t.Fatalf("cleanup CometBFT method = %q, want unsubscribe_all", method)
	}
	select {
	case <-peerDone:
	case <-time.After(timeout):
		t.Fatal("monitor cleanup did not close the CometBFT WebSocket")
	}
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}

	// Cleanup also owns the cache database. A successful reopen proves it was
	// closed after the event service and WebSocket client.
	db, err := monitorcache.OpenDB(filepath.Join(cacheDir, "monitor.db"))
	if err != nil {
		t.Fatalf("reopen monitor database after cleanup: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close reopened monitor database: %v", err)
	}
}

func TestRunCancelsModelConsensusSubscription(t *testing.T) {
	const timeout = 5 * time.Second

	consensusReady := make(chan struct{}, 1)
	consensusClosed := make(chan struct{}, 1)
	peerErrors := make(chan error, 2)
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, request *http.Request) {
		if page := request.URL.Query().Get("page"); page != "1" {
			reportConsensusPeerError(peerErrors, "validator page = %q, want 1", page)
			http.Error(w, "unexpected validator page", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"1","validators":[{"address":"AA","pub_key":{"type":"tendermint/PubKeyEd25519","value":"key-a"},"voting_power":"10"}],"count":"1","total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			reportConsensusPeerError(peerErrors, "upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		consensusConnection := false
		consensusSubscriptions := 0
		for {
			var call struct {
				ID     json.RawMessage   `json:"id"`
				Method string            `json:"method"`
				Params map[string]string `json:"params"`
			}
			if err := conn.ReadJSON(&call); err != nil {
				if consensusConnection {
					consensusClosed <- struct{}{}
				}
				return
			}

			if call.Method == "subscribe" {
				switch call.Params["query"] {
				case "tm.event='Vote'", "tm.event='NewRoundStep'":
					consensusConnection = true
					consensusSubscriptions++
					if consensusSubscriptions == 2 {
						consensusReady <- struct{}{}
					}
				}
			}
			if err := writeEventRPCResult(conn, call.ID); err != nil {
				if call.Method != "unsubscribe_all" {
					reportConsensusPeerError(peerErrors, "write %s result: %v", call.Method, err)
				}
				return
			}
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	err := run(Config{
		RPCEndpoint:  server.URL,
		RESTEndpoint: server.URL,
		CacheDir:     t.TempDir(),
	}, func(model tea.Model) error {
		init := model.Init()
		if init == nil {
			t.Fatal("monitor model Init returned no command")
		}
		initMsg := init()
		batch, ok := initMsg.(tea.BatchMsg)
		if !ok || len(batch) == 0 {
			t.Fatalf("monitor model Init result = %T, want non-empty tea.BatchMsg", initMsg)
		}

		// The first Init command establishes the model's consensus stream. Run
		// it synchronously so returning from this callback precisely models a
		// completed program with an active subscription.
		if msg := batch[0](); msg == nil {
			t.Fatal("consensus connection command returned no result")
		}
		select {
		case <-consensusReady:
		case <-time.After(timeout):
			t.Fatal("monitor model did not subscribe to consensus events")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	select {
	case <-consensusClosed:
	case <-time.After(timeout):
		t.Fatal("monitor run left its consensus WebSocket open")
	}
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestRunCancelsAndDrainsBlockedChainRefreshBeforeCacheClose(t *testing.T) {
	const timeout = 5 * time.Second

	cacheDir := t.TempDir()
	dbPath := filepath.Join(cacheDir, "monitor.db")
	db, err := monitorcache.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("open monitor database: %v", err)
	}
	providerCache, _, err := monitorcache.OpenStores(db)
	if err != nil {
		_ = db.Close()
		t.Fatalf("open monitor stores: %v", err)
	}
	providerCache.SyncWithChain([]monitorrpc.OnChainProvider{{
		Owner:   "akash1existing",
		HostURI: "https://existing.example.test:8443",
	}})
	if err := db.Close(); err != nil {
		t.Fatalf("close primed monitor database: %v", err)
	}

	providers := providerv1beta4.QueryProvidersResponse{
		Providers: providerv1beta4.Providers{{
			Owner:   "akash1new",
			HostURI: "https://new.example.test:8443",
		}},
	}
	encodedProviders, err := providers.Marshal()
	if err != nil {
		t.Fatalf("marshal provider response: %v", err)
	}
	leaseRequestStarted := make(chan struct{})
	leaseRequestCanceled := make(chan struct{})
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		for {
			var call eventRPCRequest
			if err := connection.ReadJSON(&call); err != nil {
				return
			}
			if err := writeEventRPCResult(connection, call.ID); err != nil {
				return
			}
		}
	})
	mux.HandleFunc("/abci_query", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"result":{"response":{"code":0,"value":%q}}}`,
			base64.StdEncoding.EncodeToString(encodedProviders))
	})
	mux.HandleFunc("/akash/market/v1beta5/leases/list", func(_ http.ResponseWriter, request *http.Request) {
		close(leaseRequestStarted)
		<-request.Context().Done()
		close(leaseRequestCanceled)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	commandReturned := make(chan struct{})
	err = run(Config{
		RPCEndpoint:      server.URL,
		RESTEndpoint:     server.URL,
		CacheDir:         cacheDir,
		InitialDashboard: "provider",
	}, func(model tea.Model) error {
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		if cmd == nil {
			t.Fatal("provider refresh did not return a chain command")
		}
		go func() {
			_ = cmd()
			close(commandReturned)
		}()
		select {
		case <-leaseRequestStarted:
			return nil
		case <-time.After(timeout):
			return errors.New("active lease query did not start")
		}
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	select {
	case <-leaseRequestCanceled:
	case <-time.After(timeout):
		t.Fatal("runtime cleanup did not cancel the blocked chain request")
	}
	select {
	case <-commandReturned:
	case <-time.After(timeout):
		t.Fatal("runtime cleanup did not drain the chain command")
	}

	db, err = monitorcache.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("reopen monitor database after runtime cleanup: %v", err)
	}
	providerCache, _, err = monitorcache.OpenStores(db)
	if err != nil {
		_ = db.Close()
		t.Fatalf("reopen monitor stores: %v", err)
	}
	if _, exists := providerCache.GetProvider("akash1new"); exists {
		_ = db.Close()
		t.Fatal("canceled chain refresh wrote its provider after shutdown began")
	}
	if _, exists := providerCache.GetProvider("akash1existing"); !exists {
		_ = db.Close()
		t.Fatal("runtime cleanup lost the existing provider cache")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close reopened monitor database: %v", err)
	}
}

func receiveRuntimeRPCMethod(t *testing.T, methods <-chan string, timeout time.Duration) string {
	t.Helper()
	select {
	case method := <-methods:
		return method
	case <-time.After(timeout):
		t.Fatal("timed out waiting for CometBFT JSON-RPC request")
		return ""
	}
}

func reportConsensusPeerError(errors chan<- error, format string, args ...any) {
	select {
	case errors <- fmt.Errorf(format, args...):
	default:
	}
}
