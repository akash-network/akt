package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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

	// Nine commands serve the network/oracle dashboards. The provider
	// pipeline adds one ordered load/sync command and three timer chains.
	if got, want := len(cmds), 13; got != want {
		t.Fatalf("Init() command count = %d, want %d", got, want)
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
