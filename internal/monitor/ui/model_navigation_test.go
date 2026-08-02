package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestProviderVersionNavigationKeys(t *testing.T) {
	providers := newTestProviders(2)
	tests := map[string]struct {
		key       rune
		wantIndex int
	}{
		"previous wraps": {key: 'h', wantIndex: 2},
		"next":           {key: 'l', wantIndex: 1},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := Model{
				hubTab:        HubProvider,
				providerTable: newTestProviderTableModel(providers),
				providers: ProviderList{
					Items:      providers,
					Versions:   []string{"0.6.4", "0.6.3", "0.6.2"},
					VersionIdx: 0,
					Version:    "0.6.4",
				},
			}

			updated, _ := m.Update(tea.KeyPressMsg{Code: tc.key})
			got := updated.(*Model)
			if got.providers.VersionIdx != tc.wantIndex {
				t.Fatalf("version index = %d, want %d", got.providers.VersionIdx, tc.wantIndex)
			}
		})
	}
}

func TestNetworkNumberKeysSelectGovernanceViews(t *testing.T) {
	tests := map[string]struct {
		key  rune
		want Tab
	}{
		"governance proposals":  {key: '3', want: TabGovernance},
		"governance parameters": {key: '4', want: TabParameters},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := Model{hubTab: HubNetwork, activeTab: TabOverview}
			updated, _ := m.Update(tea.KeyPressMsg{Code: tc.key})
			got := updated.(*Model)
			if got.activeTab != tc.want {
				t.Fatalf("active tab = %v, want %v", got.activeTab, tc.want)
			}
		})
	}
}

func TestProviderVersionNavigationReconcilesStaleIndex(t *testing.T) {
	tests := map[string]struct {
		versions  []string
		version   string
		index     int
		key       rune
		want      string
		wantIndex int
	}{
		"selected version moved": {
			versions:  []string{"0.7.0", "0.6.4"},
			version:   "0.6.4",
			index:     0,
			key:       'l',
			want:      "0.7.0",
			wantIndex: 0,
		},
		"selected version disappeared": {
			versions:  []string{"0.7.0"},
			version:   "0.6.2",
			index:     2,
			key:       'h',
			want:      "0.7.0",
			wantIndex: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := Model{
				hubTab:        HubProvider,
				providerTable: newTestProviderTableModel(nil),
				providers: ProviderList{
					Versions:   tc.versions,
					Version:    tc.version,
					VersionIdx: tc.index,
				},
			}

			updated, _ := m.Update(tea.KeyPressMsg{Code: tc.key})
			got := updated.(*Model)
			if got.providers.Version != tc.want || got.providers.VersionIdx != tc.wantIndex {
				t.Fatalf("selection = (%q, %d), want (%q, %d)",
					got.providers.Version, got.providers.VersionIdx, tc.want, tc.wantIndex)
			}
		})
	}
}

func TestProviderTableFiltersSelectedVersion(t *testing.T) {
	providers := newTestProviders(3)
	m := Model{
		providerTable: newTestProviderTableModel(providers),
		providers: ProviderList{
			Items:    providers,
			Versions: []string{"0.6.4", "0.6.3", "0.6.2"},
			Version:  "0.6.3",
		},
	}

	m.rebuildProviderTableRows()
	if got := len(m.providerTable.Rows()); got != 1 {
		t.Fatalf("provider table rows = %d, want 1 selected-version row", got)
	}
	filtered := m.getFilteredProviders()
	if len(filtered) != 1 || filtered[0].AkashVersion != "0.6.3" {
		t.Fatalf("detail provider mapping = %#v, want only version 0.6.3", filtered)
	}
}

func TestProviderDetailResponseIsDiscardedForStaleSelection(t *testing.T) {
	m := Model{
		detailRequestID: 2,
		detail: ProviderDetail{
			Showing:  true,
			Provider: &rpc.Provider{HostURI: "https://other.example.com:8443"},
		},
		nodeTable: newTestNodeTableModel(nil),
	}

	updated, _ := m.handleProviderDetailMsg(providerDetailMsg{
		hostURI:   "https://stale.example.com:8443",
		requestID: 1,
		nodes:     newTestNodes(1),
	})
	got := updated.(*Model)
	if len(got.detail.Nodes) != 0 {
		t.Fatalf("stale detail response populated %d nodes", len(got.detail.Nodes))
	}
}

func TestProviderDetailResponseIsDiscardedAfterLeavingDetail(t *testing.T) {
	hostURI := "https://selected.example.com:8443"
	m := Model{
		detailRequestID: 2,
		detail: ProviderDetail{
			Showing:  false,
			Provider: &rpc.Provider{HostURI: hostURI},
		},
		nodeTable: newTestNodeTableModel(nil),
	}

	updated, _ := m.handleProviderDetailMsg(providerDetailMsg{
		hostURI:   hostURI,
		requestID: 2,
		nodes:     newTestNodes(1),
	})
	got := updated.(*Model)
	if len(got.detail.Nodes) != 0 {
		t.Fatalf("detail response populated %d nodes after the view closed", len(got.detail.Nodes))
	}
}

func TestMatchingProviderDetailResponseIsApplied(t *testing.T) {
	hostURI := "https://selected.example.com:8443"
	m := Model{
		detailRequestID: 3,
		detail: ProviderDetail{
			Showing:  true,
			Provider: &rpc.Provider{HostURI: hostURI},
		},
		nodeTable: newTestNodeTableModel(nil),
	}
	nodes := newTestNodes(1)

	updated, _ := m.handleProviderDetailMsg(providerDetailMsg{
		hostURI:   hostURI,
		requestID: 3,
		nodes:     nodes,
	})
	got := updated.(*Model)
	if len(got.detail.Nodes) != 1 || got.detail.Nodes[0].Name != nodes[0].Name {
		t.Fatalf("matching detail response = %#v, want selected provider nodes", got.detail.Nodes)
	}
}

func TestProviderDetailDashboardNavigation(t *testing.T) {
	providers := newTestProviders(1)
	m := Model{
		hubTab:    HubProvider,
		detail:    ProviderDetail{Showing: true},
		nodeTable: newTestNodeTableModel(newTestNodes(1)),
		providers: ProviderList{Items: providers},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	got := updated.(*Model)
	if got.detail.Showing {
		t.Fatal("Shift-Tab left provider detail open")
	}
	if got.hubTab != HubNetwork {
		t.Fatalf("hub tab = %v, want Network", got.hubTab)
	}
}

func TestProviderDetailIgnoresNetworkNumberKeys(t *testing.T) {
	for _, code := range []rune{'1', '2', '3'} {
		t.Run(string(code), func(t *testing.T) {
			m := Model{
				hubTab:    HubProvider,
				detail:    ProviderDetail{Showing: true},
				nodeTable: newTestNodeTableModel(newTestNodes(1)),
			}

			updated, _ := m.Update(tea.KeyPressMsg{Code: code})
			got := updated.(*Model)
			if !got.detail.Showing {
				t.Fatalf("%c closed provider detail, but number navigation is Network-only", code)
			}
		})
	}
}
