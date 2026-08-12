// Package runtime owns the shipped standalone akt monitor lifecycle.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	cmthttp "github.com/cometbft/cometbft/rpc/client/http"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	bolt "go.etcd.io/bbolt"

	aktevents "pkg.akt.dev/akt/internal/events"
	monitorcache "pkg.akt.dev/akt/internal/monitor/cache"
	monitorrpc "pkg.akt.dev/akt/internal/monitor/rpc"
	monitorui "pkg.akt.dev/akt/internal/monitor/ui"
	"pkg.akt.dev/go/util/pubsub"
)

type monitorCometClient interface {
	sdkclient.CometRPC
	Start() error
	Stop() error
	IsRunning() bool
}

type flushingMonitorCometClient struct {
	*cmthttp.HTTP
}

// FlushUnsubscribe is a write barrier for CometBFT's asynchronous WebSocket
// client. Its unbuffered queue cannot accept this second idempotent request
// until the writer has completed the preceding unsubscribe_all WriteJSON.
func (client *flushingMonitorCometClient) FlushUnsubscribe(ctx context.Context) error {
	const flushTimeout = time.Second
	flushContext, cancel := context.WithTimeout(ctx, flushTimeout)
	defer cancel()

	return client.UnsubscribeAll(flushContext, "")
}

// Config holds the inputs for the shipped standalone monitor command.
type Config struct {
	RPCEndpoint      string
	RESTEndpoint     string
	CacheDir         string
	Insecure         bool
	InitialDashboard string
}

// Run launches the standalone monitor and owns all of its cache, event, and
// Bubble Tea resources until the program exits.
func Run(cfg Config) error {
	return run(cfg, runProgram)
}

func run(cfg Config, execute func(tea.Model) error) error {
	model, cleanup, err := buildModel(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	return execute(model)
}

func runProgram(model tea.Model) error {
	// The CLI already verifies an interactive terminal before reaching this
	// boundary. Supplying stdin explicitly also keeps program execution
	// deterministic under tests instead of making Bubble Tea reopen /dev/tty.
	_, err := tea.NewProgram(model, tea.WithInput(os.Stdin)).Run()
	return err
}

func buildModel(cfg Config) (tea.Model, func(), error) {
	return buildModelWith(cfg, monitorcache.OpenStores)
}

func buildModelWith(
	cfg Config,
	initializeStores func(*bolt.DB) (*monitorcache.ProviderCache, *monitorcache.MonikerCache, error),
) (tea.Model, func(), error) {
	return buildModelWithEvents(
		cfg,
		initializeStores,
		func(remote, endpoint string) (monitorCometClient, error) {
			client, err := cmthttp.New(remote, endpoint)
			if err != nil {
				return nil, err
			}

			return &flushingMonitorCometClient{HTTP: client}, nil
		},
		aktevents.NewService,
	)
}

func buildModelWithEvents(
	cfg Config,
	initializeStores func(*bolt.DB) (*monitorcache.ProviderCache, *monitorcache.MonikerCache, error),
	newCometClient func(string, string) (monitorCometClient, error),
	newEventService func(context.Context, sdkclient.CometRPC, string, pubsub.Bus) (aktevents.Service, error),
) (tea.Model, func(), error) {
	if cfg.RPCEndpoint == "" {
		return nil, nil, errors.New("monitor RPC endpoint is required")
	}
	if cfg.CacheDir == "" {
		return nil, nil, errors.New("monitor cache directory is required")
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create monitor cache directory %s: %w", cfg.CacheDir, err)
	}

	dbPath := filepath.Join(cfg.CacheDir, "monitor.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		legacyPath := filepath.Join(cfg.CacheDir, "top.db")
		if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
			_ = os.Rename(legacyPath, dbPath)
		}
	}

	db, err := monitorcache.OpenDB(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open monitor cache %s: %w", dbPath, err)
	}

	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	var runtimeTasks *monitorui.RuntimeTaskGroup
	var bus pubsub.Bus
	var evtSvc aktevents.Service
	var cometClient monitorCometClient
	cleanup := sync.OnceFunc(func() {
		cancelRuntime()
		if runtimeTasks != nil {
			runtimeTasks.StopAndWait()
		}
		if evtSvc != nil {
			evtSvc.Shutdown()
		}
		if cometClient != nil && cometClient.IsRunning() {
			_ = cometClient.Stop()
		}
		if bus != nil {
			bus.Close()
		}
		_ = db.Close()
	})
	fail := func(err error) (tea.Model, func(), error) {
		cleanup()
		return nil, nil, err
	}

	provCache, monCache, err := initializeStores(db)
	if err != nil {
		return fail(fmt.Errorf("initialize monitor caches: %w", err))
	}

	runtimeTasks = monitorui.NewRuntimeTaskGroup()
	bus = pubsub.NewBus()
	createdClient, err := newCometClient(cfg.RPCEndpoint, "/websocket")
	if err != nil {
		return fail(fmt.Errorf("create monitor CometBFT event client: %w", err))
	}
	cometClient = createdClient
	if err := cometClient.Start(); err != nil {
		return fail(fmt.Errorf("start monitor CometBFT event client: %w", err))
	}

	// The event service owns the CometBFT subscription and gracefully
	// unsubscribes before cleanup stops the client. Model network commands use
	// runtimeContext and drain first in cleanup.
	evtSvc, err = newEventService(context.Background(), cometClient, "akt-monitor", bus)
	if err != nil {
		return fail(fmt.Errorf("start monitor event service: %w", err))
	}

	model := monitorui.NewModel(monitorui.ModelConfig{
		Client:             monitorrpc.NewClient(cfg.RPCEndpoint, cfg.RESTEndpoint),
		RPCClient:          monitorrpc.NewRPCProviderClient(cfg.RPCEndpoint),
		RuntimeContext:     runtimeContext,
		RuntimeTasks:       runtimeTasks,
		Cache:              provCache,
		MonikerCache:       monCache,
		InsecureSkipVerify: cfg.Insecure,
		Embedded:           false,
		InitialDashboard:   cfg.InitialDashboard,
		Bus:                bus,
	})
	return model, cleanup, nil
}
