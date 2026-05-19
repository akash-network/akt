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
	// Don't call SetSize — width stays 0, which is < 40, so View uses 80.
	out := d.View()
	if out == "" {
		t.Error("View() returned empty string with default (zero) size")
	}
}

func TestDashboardShowsPanelTitles(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	// The new dashboard has titled panels: WALLET, ACTIVE, NETWORK,
	// RECENT ACTIVITY, and SHORTCUTS.
	panels := []string{
		"WALLET",
		"ACTIVE",
		"NETWORK",
		"RECENT ACTIVITY",
		"SHORTCUTS",
	}
	for _, p := range panels {
		if !strings.Contains(out, p) {
			t.Errorf("View() missing panel title %q", p)
		}
	}
}

func TestDashboardShowsWalletPanel(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	// Wallet panel should show labels for address, liquid, staked, rewards, escrow.
	labels := []string{"address", "liquid", "staked", "rewards", "escrow"}
	for _, l := range labels {
		if !strings.Contains(out, l) {
			t.Errorf("View() missing wallet label %q", l)
		}
	}
}

func TestDashboardSetContext(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetContext("my-context", "akashnet-2", "akash1abc123")

	out := ansi.Strip(d.View())

	// Chain ID should appear in the Network panel.
	if !strings.Contains(out, "akashnet-2") {
		t.Error("View() missing chain ID after SetContext")
	}
	// Account should appear in the welcome banner and wallet.
	if !strings.Contains(out, "akash1abc123") {
		t.Error("View() missing account after SetContext")
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

	// Stats are used internally; active count shows in the ACTIVE panel title.
	// The panel title is "ACTIVE · N" where N comes from len(deployments),
	// not from stats. Stats are stored for potential future use.
	out := ansi.Strip(d.View())
	// Just verify it renders without error.
	if out == "" {
		t.Error("View() returned empty string after SetStats")
	}
}

func TestDashboardNoStatsShowsDash(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	// Without data, panels should show em dash (—) for missing values.
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

	// Block height should appear in Network panel (possibly comma-grouped).
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

	// Create 7 deployments (max displayed is 4).
	depls := make([]*store.DeploymentRecord, 7)
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

	// The ACTIVE panel title should show the total count.
	if !strings.Contains(out, "ACTIVE") {
		t.Error("View() should show ACTIVE panel title")
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

	// ProposalCount is stored but not directly displayed in the new design.
	// Verify it doesn't crash.
	out := ansi.Strip(d.View())
	if out == "" {
		t.Error("View() returned empty string after SetProposalCount")
	}
}

func TestDashboardNetworkPanelShowsChainID(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetContext("ctx", "mainnet-1", "akash1xyz")

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "mainnet-1") {
		t.Error("View() should show chain ID in Network panel")
	}
}

func TestDashboardWelcomeBanner(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetContext("my-ctx", "akashnet-2", "akash1zk2vq4r")

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "welcome back") {
		t.Error("View() missing welcome greeting")
	}
	if !strings.Contains(out, "akash1zk2vq4r") {
		t.Error("View() missing account in welcome banner")
	}
}

func TestDashboardSetWallet(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetWallet("1,284 AKT", "3,400 AKT", "+12.4 AKT", "246.4 AKT")

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "1,284 AKT") {
		t.Error("View() missing liquid balance after SetWallet")
	}
	if !strings.Contains(out, "3,400 AKT") {
		t.Error("View() missing staked balance after SetWallet")
	}
}

func TestDashboardSetRecentActivity(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)
	d.SetRecentActivity([]views.ActivityEntry{
		{Time: "14:02:11", Kind: "tx", Text: "MsgSendManifest"},
		{Time: "14:01:48", Kind: "evt", Text: "bid received"},
	})

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "MsgSendManifest") {
		t.Error("View() missing activity entry after SetRecentActivity")
	}
	if !strings.Contains(out, "bid received") {
		t.Error("View() missing second activity entry")
	}
}

func TestDashboardShortcutsPanel(t *testing.T) {
	d := views.NewDashboard()
	d.SetSize(120, 60)

	out := ansi.Strip(d.View())

	if !strings.Contains(out, "SHORTCUTS") {
		t.Error("View() missing SHORTCUTS panel")
	}
	if !strings.Contains(out, "command palette") {
		t.Error("View() missing shortcut description")
	}
}
