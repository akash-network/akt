package pretty

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
)

func TestRenderBMEVaultStateTokenLines(t *testing.T) {
	response := &bmetypes.QueryVaultStateResponse{VaultState: bmetypes.State{
		Balances:      sdk.Coins{sdk.NewInt64Coin("uact", 195427161531), sdk.NewInt64Coin("uakt", 572320058657)},
		TotalBurned:   sdk.Coins{sdk.NewInt64Coin("uact", 1062528246771), sdk.NewInt64Coin("uakt", 0)},
		TotalMinted:   sdk.Coins{sdk.NewInt64Coin("uact", 1226632271979), sdk.NewInt64Coin("uakt", 0)},
		RemintCredits: sdk.Coins{sdk.NewInt64Coin("uakt", 272320058657)},
	}}
	got := ansi.Strip(RenderBMEVaultState(response))
	rows := []struct{ label, amount, denom string }{
		{"Balances", "195427.161531", "ACT,"},
		{"", "572320.058657", "AKT"},
		{"Total Burned", "1062528.246771", "ACT,"},
		{"", "0", "AKT"},
		{"Total Minted", "1226632.271979", "ACT,"},
		{"", "0", "AKT"},
		{"Remint Credits", "272320.058657", "AKT"},
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("got %d lines, want one header and %d token lines:\n%s", len(lines), len(rows), got)
	}
	amountEnd := -1
	for i, row := range rows {
		line := lines[i+1]
		if !strings.HasSuffix(line, row.amount+" "+row.denom) {
			t.Errorf("line %d lost a token, precision, or separator: %q", i, line)
		}
		if row.label == "" {
			if strings.Contains(line, ":") || !strings.HasPrefix(line, strings.Repeat(" ", KVKeyWidth+3)) {
				t.Errorf("continuation line %d is not indented under the value column: %q", i, line)
			}
		} else if !strings.HasPrefix(line, "  "+row.label+":") {
			t.Errorf("line %d lost its label: %q", i, line)
		}
		end := strings.LastIndex(line, " "+row.denom)
		if amountEnd == -1 {
			amountEnd = end
		} else if end != amountEnd {
			t.Errorf("line %d amount ends at column %d, want %d", i, end, amountEnd)
		}
	}
	if compact := FormatCoins(response.VaultState.Balances); compact != "195427.161531 ACT, 572320.058657 AKT" {
		t.Fatalf("compact coin formatting changed: %q", compact)
	}
}
