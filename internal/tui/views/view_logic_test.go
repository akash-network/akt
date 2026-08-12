package views

import (
	"strings"
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/keys"
)

func TestFormatTokensUsesCanonicalCoinFormatting(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		amount math.Int
		want   string
	}{
		{name: "base with trailing zero removed", amount: math.NewInt(5_300_000), want: "5.3 AKT"},
		{name: "milli", amount: math.NewInt(3_000), want: "3 mAKT"},
		{name: "micro", amount: math.NewInt(500), want: "500 uAKT"},
		{name: "sparse zero", amount: math.Int{}, want: "0 AKT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := formatTokens(tc.amount); got != tc.want {
				t.Fatalf("formatTokens result = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStoredCoinStringsUseCanonicalFormattingAcrossTUIViews(t *testing.T) {
	t.Parallel()

	deployment := &store.DeploymentRecord{
		DSeq:          42,
		Deposit:       "5000000uakt",
		EscrowBalance: "4500000.0uakt",
		Transferred:   "500000.000000000000000000uakt",
	}
	depCells := deploymentCells(deployment)
	if got, want := depCells[8], "4.5 AKT"; got != want {
		t.Fatalf("deployment escrow cell = %q, want %q", got, want)
	}
	if got, want := depCells[9], "5 AKT"; got != want {
		t.Fatalf("deployment cost cell = %q, want %q", got, want)
	}

	lease := &store.LeaseRecord{
		ID:    store.LeaseID{Provider: "akash1provider"},
		Price: "2500.000000000000000000uakt",
	}
	if got, want := leaseCells(lease)[5], "2.5 mAKT"; got != want {
		t.Fatalf("lease price cell = %q, want %q", got, want)
	}

	detail := NewDeploymentDetailView(nil, keys.DefaultKeyMap(), deployment)
	detail.leases = []*store.LeaseRecord{lease}
	detail.SetSize(120, 40)
	detail.tab = 1
	leaseTab := ansi.Strip(detail.View().Content)
	if !strings.Contains(leaseTab, "2.5 mAKT") || strings.Contains(leaseTab, lease.Price) {
		t.Fatalf("lease detail does not use canonical price formatting:\n%s", leaseTab)
	}
	detail.tab = 2
	escrowTab := ansi.Strip(detail.View().Content)
	for _, want := range []string{"5 AKT", "4.5 AKT", "500 mAKT", "90.0%"} {
		if !strings.Contains(escrowTab, want) {
			t.Fatalf("escrow detail missing %q:\n%s", want, escrowTab)
		}
	}

	dashboard := NewDashboard(nil, DashboardContext{}, keys.DefaultKeyMap())
	dashboard.deployments = []*store.DeploymentRecord{deployment}
	active := ansi.Strip(dashboard.activeContent(50))
	if !strings.Contains(active, "5 AKT") || strings.Contains(active, deployment.Deposit) {
		t.Fatalf("dashboard active deployment uses raw coin string:\n%s", active)
	}
}

func TestEscrowPercentParsesCoinAmountsAndRejectsMismatchedDenoms(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name             string
		deposit, balance string
		want             float64
	}{
		{name: "dec coins", deposit: "5000000uakt", balance: "4500000.0uakt", want: 0.9},
		{name: "legacy numeric", deposit: "100", balance: "25", want: 0.25},
		{name: "upper clamp", deposit: "100uakt", balance: "101uakt", want: 1},
		{name: "lower clamp", deposit: "100", balance: "-1", want: 0},
		{name: "mismatched denoms", deposit: "100uakt", balance: "50uatom", want: -1},
		{name: "invalid deposit", deposit: "not-a-coin", balance: "50uakt", want: -1},
		{name: "zero deposit", deposit: "0uakt", balance: "0uakt", want: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := escrowPercent(tc.deposit, tc.balance); got != tc.want {
				t.Fatalf("escrowPercent(%q, %q) = %v, want %v", tc.deposit, tc.balance, got, tc.want)
			}
		})
	}
}
