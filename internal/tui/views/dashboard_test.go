package views_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/views"
)

func TestDashboardRendersNonEmpty(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := d.View()
	if out == "" {
		t.Error("View() returned empty string after SetSize")
	}
}

func TestDashboardDefaultWidthWhenSmall(t *testing.T) {
	d := views.NewDashboard()
	// Don't call SetSize — width stays 0, which is < 20, so View uses 80.
	out := d.View()
	if out == "" {
		t.Error("View() returned empty string with default (zero) size")
	}
}

func TestDashboardShowsShortcuts(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	shortcuts := []string{
		"navigate views",
		"drill down",
		"go back",
		"command palette",
		"help",
		"new deployment",
	}
	for _, s := range shortcuts {
		if !strings.Contains(out, s) {
			t.Errorf("View() missing shortcut text %q", s)
		}
	}
}

func TestDashboardShowsShortcutKeys(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	keys := []string{"1-6", "Enter", "Esc", ":", "?", "D"}
	for _, k := range keys {
		if !strings.Contains(out, k) {
			t.Errorf("View() missing shortcut key %q", k)
		}
	}
}

func TestDashboardShowsSectionHeadings(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	sections := []string{
		"Account",
		"Active Deployments",
		"Network",
		"Shortcuts",
	}
	for _, s := range sections {
		if !strings.Contains(out, s) {
			t.Errorf("View() missing section heading %q", s)
		}
	}
}

func TestDashboardSetContext(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetContext("my-context", "akashnet-2", "akash1abc123")

	out := ansi.Strip(d.View())

	expected := []string{"my-context", "akashnet-2", "akash1abc123"}
	for _, e := range expected {
		if !strings.Contains(out, e) {
			t.Errorf("View() missing context value %q after SetContext", e)
		}
	}
}

func TestDashboardSetContextShowsWelcome(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetContext("default", "akashnet-2", "alice")

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "welcome back, alice") {
		t.Error("View() missing welcome message with account name")
	}
}

func TestDashboardDefaultAccountShowsUnknown(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "unknown") {
		t.Error("View() should show 'unknown' when no account is set")
	}
}

func TestDashboardSetStats(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetStats(&store.StoreStats{
		Deployments:       142,
		ActiveDeployments: 57,
		ClosedDeployments: 85,
	})

	out := ansi.Strip(d.View())

	expected := []string{"142", "57", "85"}
	for _, text := range expected {
		if !strings.Contains(out, text) {
			t.Errorf("View() missing deployment count %q after SetStats", text)
		}
	}
}

func TestDashboardNoStatsShowsDash(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	// Without stats, deployment counts should show em dash (—)
	if !strings.Contains(out, "\u2014") {
		t.Error("View() should show em dash when no stats are set")
	}
}

func TestDashboardSetSyncState(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetSyncState(&store.SyncState{
		LastBlockHeight: 12345,
		LastSyncTime:    1700000000,
	})

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "12345") {
		t.Error("View() missing block height after SetSyncState")
	}
	if !strings.Contains(out, "synced") {
		t.Error("View() missing 'synced' status after SetSyncState")
	}
}

func TestDashboardNoSyncStateShowsNotSynced(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "not synced") {
		t.Error("View() should show 'not synced' when no sync state is set")
	}
	if !strings.Contains(out, "no sync") {
		t.Error("View() should show 'no sync' badge when no sync state is set")
	}
}

func TestDashboardSetActiveDeployments(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	depls := []*store.DeploymentRecord{
		{DSeq: 100, SDLPath: "deploy.yaml", Deposit: "5000000uakt"},
		{DSeq: 200, Deposit: "1000000uakt"},
	}
	d.SetActiveDeployments(depls)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "100") {
		t.Error("View() missing deployment DSeq 100")
	}
	if !strings.Contains(out, "deploy.yaml") {
		t.Error("View() missing SDL path for deployment 100")
	}
	if !strings.Contains(out, "5000000uakt") {
		t.Error("View() missing deposit for deployment 100")
	}
	if !strings.Contains(out, "200") {
		t.Error("View() missing deployment DSeq 200")
	}
}

func TestDashboardNoActiveDeploymentsShowsEmpty(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "No active deployments") {
		t.Error("View() should show 'No active deployments' when none are set")
	}
}

func TestDashboardActiveDeploymentsLimitedToMax(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	// Create 6 deployments (max displayed is 4).
	depls := make([]*store.DeploymentRecord, 6)
	for i := range depls {
		depls[i] = &store.DeploymentRecord{
			DSeq:    uint64(100 + i),
			Deposit: fmt.Sprintf("%duakt", (i+1)*1000),
		}
	}
	d.SetActiveDeployments(depls)

	out := ansi.Strip(d.View())

	// First 4 should be visible.
	for i := 0; i < 4; i++ {
		dseq := fmt.Sprintf("%d", 100+i)
		if !strings.Contains(out, dseq) {
			t.Errorf("View() missing deployment DSeq %s (within limit)", dseq)
		}
	}

	// Should show "... and 2 more".
	if !strings.Contains(out, "and 2 more") {
		t.Error("View() should show overflow count when deployments exceed max")
	}
}

func TestDashboardSyncBridgeActive(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetSyncBridgeActive(true)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "Live") {
		t.Error("View() should show 'Live' when sync bridge is active")
	}
}

func TestDashboardSyncBridgeInactive(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetSyncBridgeActive(false)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "Offline") {
		t.Error("View() should show 'Offline' when sync bridge is inactive")
	}
}

func TestDashboardShowsAktBanner(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "akt") {
		t.Error("View() should contain 'akt' banner")
	}
}

func TestDashboardDeploymentWithoutSDLPath(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetActiveDeployments([]*store.DeploymentRecord{
		{DSeq: 999},
	})

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "999") {
		t.Error("View() missing deployment DSeq 999")
	}
}

func TestDashboardDeploymentWithoutDeposit(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetActiveDeployments([]*store.DeploymentRecord{
		{DSeq: 555, Deposit: ""},
	})

	out := ansi.Strip(d.View())

	// Empty deposit should render as em dash.
	if !strings.Contains(out, "555") {
		t.Error("View() missing deployment DSeq 555")
	}
	if !strings.Contains(out, "\u2014") {
		t.Error("View() should show em dash for empty deposit")
	}
}

func TestDashboardNetworkSectionShowsChainID(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetContext("ctx", "mainnet-1", "akash1xyz")

	out := ansi.Strip(d.View())

	// Chain ID should appear in both the welcome banner and the Network section.
	count := strings.Count(out, "mainnet-1")
	if count < 2 {
		t.Errorf("View() should show chain ID in both welcome and Network section, found %d occurrences", count)
	}
}

func TestDashboardAccountSectionLabels(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	labels := []string{"address", "deployments", "active", "closed", "sync status"}
	for _, l := range labels {
		if !strings.Contains(out, l) {
			t.Errorf("View() missing Account section label %q", l)
		}
	}
}

func TestDashboardNetworkSectionLabels(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	labels := []string{"chain", "last block", "last sync"}
	for _, l := range labels {
		if !strings.Contains(out, l) {
			t.Errorf("View() missing Network section label %q", l)
		}
	}
}
