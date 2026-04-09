package pretty

import (
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func TestSection(t *testing.T) {
	golden.RequireEqual(t, Section("Test Section"))
}

func TestBold(t *testing.T) {
	golden.RequireEqual(t, Bold("bold text"))
}

func TestDim(t *testing.T) {
	golden.RequireEqual(t, Dim("dim text"))
}

func TestKV(t *testing.T) {
	tests := map[string]struct {
		key   string
		value string
	}{
		"Simple": {"Status", "active"},
		"Styled": {"State", ColorState("active")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			KV(&buf, tc.key, tc.value)
			golden.RequireEqual(t, buf.String())
		})
	}
}

func TestFormatCoin(t *testing.T) {
	tests := map[string]struct {
		coin sdk.Coin
	}{
		"Micro":    {sdk.NewInt64Coin("uakt", 500)},
		"Milli":    {sdk.NewInt64Coin("uakt", 3000)},
		"Base":     {sdk.NewInt64Coin("uakt", 5300000)},
		"NonMicro": {sdk.NewInt64Coin("atom", 100)},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatCoin(tc.coin))
		})
	}
}

func TestFormatCoins(t *testing.T) {
	tests := map[string]struct {
		coins sdk.Coins
	}{
		"Single":   {sdk.NewCoins(sdk.NewInt64Coin("uakt", 5000000))},
		"Multiple": {sdk.NewCoins(sdk.NewInt64Coin("uakt", 5000000), sdk.NewInt64Coin("uusdc", 10000000))},
		"Empty":    {nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatCoins(tc.coins))
		})
	}
}

func TestColorState(t *testing.T) {
	tests := map[string]struct {
		state string
	}{
		"Active":  {"active"},
		"Closed":  {"closed"},
		"Paused":  {"paused"},
		"Unknown": {"custom"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, ColorState(tc.state))
		})
	}
}

func TestFormatHeight(t *testing.T) {
	tests := map[string]struct {
		height int64
	}{
		"Zero":  {0},
		"Small": {999},
		"Large": {18234567},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatHeight(tc.height))
		})
	}
}

func TestFormatDurationPretty(t *testing.T) {
	tests := map[string]struct {
		d time.Duration
	}{
		"Seconds":   {30 * time.Second},
		"Minutes":   {5*time.Minute + 30*time.Second},
		"Hours":     {2*time.Hour + 15*time.Minute},
		"Days":      {3 * 24 * time.Hour},
		"DaysHours": {7*24*time.Hour + 12*time.Hour + 30*time.Minute},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatDuration(tc.d))
		})
	}
}

func TestFormatPercentPretty(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"Zero":     {"0"},
		"OneThird": {"0.334000000000000000"},
		"Full":     {"1.000000000000000000"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatPercent(tc.input))
		})
	}
}

func TestFormatBool(t *testing.T) {
	tests := map[string]struct {
		b bool
	}{
		"True":  {true},
		"False": {false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatBool(tc.b))
		})
	}
}

func TestFormatCPU(t *testing.T) {
	tests := map[string]struct {
		val math.Int
	}{
		"Whole":      {math.NewInt(4000)},
		"Fractional": {math.NewInt(2500)},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatCPU(tc.val))
		})
	}
}

func TestWriteTable(t *testing.T) {
	var buf strings.Builder
	WriteTable(&buf, []string{"NAME", "VALUE"}, [][]string{
		{"foo", "bar"},
		{"hello", "world"},
	})
	golden.RequireEqual(t, buf.String())
}
