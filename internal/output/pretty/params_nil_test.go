package pretty

import (
	"strings"
	"testing"

	"cosmossdk.io/math"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// nilDec returns a LegacyDec with a nil inner big.Int, exactly as proto
// unmarshalling produces for an omitted field.
func nilDec() math.LegacyDec { return math.LegacyDec{} }

// A node that omits a zero-valued proto field sends nothing on the wire, so
// the field unmarshals with a nil inner big.Int. Rendering must degrade to
// "0" rather than panicking the command.
func TestRenderStakingParamsZeroValueFields(t *testing.T) {
	// Deliberately zero-value: MinCommissionRate carries a nil Dec.
	out := RenderStakingParams(&stakingtypes.Params{BondDenom: "uakt"})

	if !strings.Contains(out, "Min Commission") {
		t.Fatalf("expected the params table to render, got:\n%s", out)
	}
	if !strings.Contains(out, "0%") {
		t.Errorf("nil commission rate should render as 0%%, got:\n%s", out)
	}
}

func TestRenderStakingPoolZeroValueFields(t *testing.T) {
	out := RenderStakingPool(&stakingtypes.Pool{})

	if !strings.Contains(out, "Bonded Tokens") {
		t.Fatalf("expected the pool table to render, got:\n%s", out)
	}
	if !strings.Contains(out, "0 AKT") {
		t.Errorf("nil token amounts should render as 0 AKT, got:\n%s", out)
	}
}

func TestFormatHelpersTolerateNilValues(t *testing.T) {
	// Each of these panicked before the DecOrZero/IntOrZero guards.
	if got := FormatPercentDec(nilDec()); got != "0%" {
		t.Errorf("FormatPercentDec(nil) = %q, want 0%%", got)
	}
	if got := FormatDecAsAKT(nilDec()); !strings.Contains(got, "0") {
		t.Errorf("FormatDecAsAKT(nil) = %q, want a zero amount", got)
	}
}
