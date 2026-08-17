package pretty

import (
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/ansi"
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

// plainLines strips ANSI styling and splits rendered output into lines,
// dropping the trailing empty line left by the final newline.
func plainLines(s string) []string {
	lines := strings.Split(strings.TrimSuffix(ansi.Strip(s), "\n"), "\n")
	return lines
}

func TestWriteTableOrEmpty(t *testing.T) {
	tests := map[string]struct {
		rows [][]string
		want []string
	}{
		"Empty": {
			rows: nil,
			want: []string{"(no deployments)"},
		},
		"WithRows": {
			rows: [][]string{{"12345", "active"}},
			want: []string{"ID     STATE ", "12345  active"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			WriteTableOrEmpty(&buf, []string{"ID", "STATE"}, tc.rows, "(no deployments)")

			got := plainLines(buf.String())
			if len(got) != len(tc.want) {
				t.Fatalf("got %d lines %q, want %d %q", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// A search that matched nothing must say so. Printing the header alone reads
// as a rendering failure rather than as an empty result (SPEC §10.3).
func TestWriteTableNoRowsNeverPrintsBareHeader(t *testing.T) {
	cols := []ColDef{{Header: "ID"}, {Header: "PRICE", Align: AlignRight}}

	tests := map[string]func(w *strings.Builder){
		"WriteTableCols":        func(w *strings.Builder) { WriteTableCols(w, cols, nil) },
		"WriteTableOrEmpty":     func(w *strings.Builder) { WriteTableOrEmpty(w, []string{"ID", "PRICE"}, nil, "(no bids)") },
		"WriteTableColsOrEmpty": func(w *strings.Builder) { WriteTableColsOrEmpty(w, cols, nil, "(no bids)") },
	}

	for name, write := range tests {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			write(&buf)

			got := ansi.Strip(buf.String())
			if strings.Contains(got, "ID") || strings.Contains(got, "PRICE") {
				t.Errorf("empty table printed its header: %q", got)
			}
			if !strings.HasPrefix(got, "(no ") {
				t.Errorf("empty table = %q, want a (no ...) message", got)
			}
		})
	}
}

// A header must sit over its own column, not float in the middle of it.
func TestWriteTableColsHeaderAlignmentMatchesColumn(t *testing.T) {
	cols := []ColDef{
		{Header: "ID"},
		{Header: "PRICE", Align: AlignRight},
	}
	rows := [][]string{
		{"akash1deployment/1/1/1", "12.5 uakt"},
		{"akash1deployment/2/1/1", "1000000.25 uakt"},
	}

	var buf strings.Builder
	WriteTableCols(&buf, cols, rows)
	lines := plainLines(buf.String())

	if len(lines) != 3 {
		t.Fatalf("got %d lines %q, want 3", len(lines), lines)
	}

	// Left-aligned column: header starts where its cells start.
	for i, line := range lines {
		if !strings.HasPrefix(line, []string{"ID", rows[0][0], rows[1][0]}[i]) {
			t.Errorf("line %d = %q, does not start its left-aligned column", i, line)
		}
	}

	// Right-aligned column: header ends where its cells end.
	for i, line := range lines {
		if end := len(strings.TrimRight(line, " ")); end != len(lines[0]) {
			t.Errorf("line %d = %q ends at %d, want the right-aligned column edge %d",
				i, line, end, len(lines[0]))
		}
	}

	if !strings.HasSuffix(lines[0], "PRICE") {
		t.Errorf("header = %q, want PRICE flush with the right-aligned column", lines[0])
	}
}

// padRight never truncates, so a key wider than its column pushes its own
// value out of line unless the column is widened for it.
func TestSubKVWidthKeepsWideKeysInColumn(t *testing.T) {
	const key = "Chain transactions" // 19 columns with the colon

	var overflow strings.Builder
	SubKV(&overflow, key, "available")
	if got, want := valueColumn(t, overflow.String()), subKVValueColumn(SubKVKeyWidth); got == want {
		t.Fatalf("key %q unexpectedly fits the default column (value at %d)", key, got)
	}

	var widened strings.Builder
	SubKVWidth(&widened, len(key)+1, key, "available")
	if got, want := valueColumn(t, widened.String()), subKVValueColumn(len(key)+1); got != want {
		t.Errorf("value column = %d, want %d", got, want)
	}
}

// KV and SubKV share a value column at any width, as long as SubKV's column is
// SubKVIndentDelta narrower (SPEC §10.12).
func TestKVAndSubKVShareValueColumn(t *testing.T) {
	for _, width := range []int{KVKeyWidth, 23, 30} {
		var kv, sub strings.Builder
		KVWidth(&kv, width, "Key", "value")
		SubKVWidth(&sub, width-SubKVIndentDelta, "Key", "value")

		if got, want := valueColumn(t, sub.String()), valueColumn(t, kv.String()); got != want {
			t.Errorf("width %d: SubKV value column = %d, KV value column = %d", width, got, want)
		}
	}
}

func subKVValueColumn(width int) int {
	const subKVIndent = 6
	return subKVIndent + width + 1
}

// valueColumn returns the 0-based column where the value of a single
// "key: value" line begins.
func valueColumn(t *testing.T, line string) int {
	t.Helper()

	col, ok := valueColumnOf(line)
	if !ok {
		t.Fatalf("line %q is not a key-value line", strings.TrimSuffix(ansi.Strip(line), "\n"))
	}

	return col
}

// valueColumnOf reports the 0-based column where a "key: value" line's value
// begins, and whether the line has one (a section title or a key-only header
// does not).
func valueColumnOf(line string) (int, bool) {
	plain := strings.TrimSuffix(ansi.Strip(line), "\n")

	colon := strings.Index(plain, ":")
	if colon < 0 {
		return 0, false
	}

	for i := colon + 1; i < len(plain); i++ {
		if plain[i] != ' ' {
			return i, true
		}
	}

	return 0, false
}
