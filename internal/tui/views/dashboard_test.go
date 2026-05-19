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

func TestDashboardShowsSectionHeadings(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	// The new dashboard has "Recent Deployments" and "Network" sections.
	sections := []string{
		"Recent Deployments",
		"Network",
	}
	for _, s := range sections {
		if !strings.Contains(out, s) {
			t.Errorf("View() missing section heading %q", s)
		}
	}
}

func TestDashboardShowsSummaryCards(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	// Summary cards should show labels for Balance, Deployments, Leases.
	cards := []string{"Balance", "Deployments", "Leases"}
	for _, c := range cards {
		if !strings.Contains(out, c) {
			t.Errorf("View() missing summary card label %q", c)
		}
	}
}

func TestDashboardSetContext(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetContext("my-context", "akashnet-2", "akash1abc123")

	out := ansi.Strip(d.View())

	// Chain ID should appear in the Network section.
	if !strings.Contains(out, "akashnet-2") {
		t.Error("View() missing chain ID after SetContext")
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

	// Active count should appear in the Deployments card.
	if !strings.Contains(out, "57") {
		t.Error("View() missing active deployment count after SetStats")
	}
	// Total count should appear.
	if !strings.Contains(out, "142") {
		t.Error("View() missing total deployment count after SetStats")
	}
}

func TestDashboardNoStatsShowsDash(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	// Without stats, the cards should show em dash (—) or "0".
	if !strings.Contains(out, "\u2014") && !strings.Contains(out, "0") {
		t.Error("View() should show dash or zero when no stats are set")
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

	// Block height should appear in Network section (possibly comma-grouped).
	if !strings.Contains(out, "12,345") && !strings.Contains(out, "12345") {
		t.Error("View() missing block height after SetSyncState")
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
	if !strings.Contains(out, "200") {
		t.Error("View() missing deployment DSeq 200")
	}
}

func TestDashboardNoActiveDeploymentsShowsEmpty(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "No deployments") {
		t.Error("View() should show empty state when no deployments are set")
	}
}

func TestDashboardActiveDeploymentsLimitedToMax(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	// Create 7 deployments (max displayed is 5).
	depls := make([]*store.DeploymentRecord, 7)
	for i := range depls {
		depls[i] = &store.DeploymentRecord{
			DSeq:    uint64(100 + i),
			Deposit: fmt.Sprintf("%duakt", (i+1)*1000),
		}
	}
	d.SetActiveDeployments(depls)

	out := ansi.Strip(d.View())

	// First 5 should be visible.
	for i := 0; i < 5; i++ {
		dseq := fmt.Sprintf("%d", 100+i)
		if !strings.Contains(out, dseq) {
			t.Errorf("View() missing deployment DSeq %s (within limit)", dseq)
		}
	}

	// Should show "Press 2 to see all 7 deployments".
	if !strings.Contains(out, "see all") {
		t.Error("View() should show 'see all' message when deployments exceed max")
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

func TestDashboardSetBalance(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetBalance("148.52 AKT")

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "148.52 AKT") {
		t.Error("View() missing balance after SetBalance")
	}
}

func TestDashboardSetValidatorCount(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetValidatorCount(100)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "100") {
		t.Error("View() missing validator count after SetValidatorCount")
	}
}

func TestDashboardSetProposalCount(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetProposalCount(2)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "2") {
		t.Error("View() missing proposal count after SetProposalCount")
	}
}

func TestDashboardNetworkSectionShowsChainID(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetContext("ctx", "mainnet-1", "akash1xyz")

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "mainnet-1") {
		t.Error("View() should show chain ID in Network section")
	}
}
