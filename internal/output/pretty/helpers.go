package pretty

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"cosmossdk.io/math"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// Styles used throughout pretty output. All colors are sourced from the
// shared theme package so CLI pretty output and TUI views are consistent.
// Each style below is byte-identical to the deprecated theme alias it
// replaced; where the theme has no canonical style with the same rendering
// (bold-only, underlined section, Slate500 key), the style is built here from
// canonical theme colors rather than from a deprecated alias.
var (
	StyleBold    = lipgloss.NewStyle().Bold(true)
	StyleDim     = theme.Secondary
	StyleSection = lipgloss.NewStyle().Bold(true).Underline(true)
	StyleKey     = lipgloss.NewStyle().Foreground(theme.Slate500)
	StyleGreen   = theme.StateGreen
	StyleYellow  = theme.StateYellow
	StyleRed     = lipgloss.NewStyle().Foreground(theme.AccentRed)
	StyleGray    = theme.Secondary
	StyleCyan    = lipgloss.NewStyle().Foreground(theme.BlueColor)
	StyleMagenta = lipgloss.NewStyle().Foreground(theme.PurpleColor)

	StyleHeader = lipgloss.NewStyle().Bold(true).Foreground(theme.Slate500)
)

// FormatCoin formats a Cosmos SDK Coin for human display.
//
// Any denom with a "u" prefix (standard Cosmos micro-denomination convention)
// is converted to the most readable unit:
//   - >= 1 base unit (1_000_000 micro): base denom  (e.g., "5.3 AKT")
//   - >= 0.001 base (1_000 micro):      milli denom (e.g., "3 mAKT")
//   - < 0.001 base:                     micro denom (e.g., "500 uAKT")
//
// Trailing zeros are always stripped. Denoms without a "u" prefix are shown as-is.
func FormatCoin(coin sdk.Coin) string {
	if isMicroDenom(coin.Denom) {
		return formatMicroDenom(coin.Amount, coin.Denom)
	}
	return fmt.Sprintf("%s %s", coin.Amount.String(), coin.Denom)
}

// isMicroDenom returns true when the denom follows the Cosmos micro-denomination
// convention: a "u" prefix followed by at least one more character (e.g., "uakt",
// "uatom", "uosmo"). IBC denoms ("ibc/...") and other formats are excluded.
func isMicroDenom(denom string) bool {
	return len(denom) >= 2 && denom[0] == 'u' && denom[1] != '/' && denom[1] >= 'a' && denom[1] <= 'z'
}

// formatMicroDenom converts a micro-denomination amount into the best
// human-readable unit. The "u" prefix is stripped to derive the base symbol:
//
//	uakt  → AKT  / mAKT  / uAKT
//	uatom → ATOM / mATOM / uATOM
//	uosmo → OSMO / mOSMO / uOSMO
func formatMicroDenom(amount math.Int, denom string) string {
	base := strings.ToUpper(denom[1:]) // "uakt" → "AKT"

	amount = IntOrZero(amount)
	uamt := amount.Int64()

	switch {
	case uamt == 0:
		return fmt.Sprintf("0 %s", base)

	case abs64(uamt) >= 1_000_000:
		d := math.LegacyNewDecFromInt(amount).Quo(math.LegacyNewDec(1_000_000))
		return fmt.Sprintf("%s %s", TrimDecTrailingZeros(d.String()), base)

	case abs64(uamt) >= 1_000:
		d := math.LegacyNewDecFromInt(amount).Quo(math.LegacyNewDec(1_000))
		return fmt.Sprintf("%s m%s", TrimDecTrailingZeros(d.String()), base)

	default:
		return fmt.Sprintf("%d u%s", uamt, base)
	}
}

// DecOrZero returns d, or zero when d carries no value. proto3 omits
// zero-valued fields on the wire, so a field a node never set unmarshals
// into a LegacyDec whose inner big.Int is nil — semantically zero, but any
// arithmetic on it panics. Every formatter takes its Dec through here so a
// sparse response renders as "0" instead of crashing the command.
func DecOrZero(d math.LegacyDec) math.LegacyDec {
	if d.IsNil() {
		return math.LegacyZeroDec()
	}

	return d
}

// IntOrZero is DecOrZero for math.Int, for the same wire reason.
func IntOrZero(i math.Int) math.Int {
	if i.IsNil() {
		return math.ZeroInt()
	}

	return i
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// FormatDecCoin formats a Cosmos SDK DecCoin for human display.
// Micro-denom scaling is applied for u-prefixed denoms (same as FormatCoin).
// Trailing zeros are always stripped.
func FormatDecCoin(coin sdk.DecCoin) string {
	if isMicroDenom(coin.Denom) {
		return formatMicroDenomDec(coin.Amount, coin.Denom)
	}
	return fmt.Sprintf("%s %s", TrimDecTrailingZeros(coin.Amount.String()), coin.Denom)
}

// FormatDecAsAKT formats a math.LegacyDec amount (in uakt) using AKT sub-denominations.
func FormatDecAsAKT(amount math.LegacyDec) string {
	return formatMicroDenomDec(DecOrZero(amount), "uakt")
}

// FormatDecAmount formats a LegacyDec amount with a denom.
// Micro-denom scaling is applied for u-prefixed denoms (same as FormatCoin).
// Trailing zeros are always stripped.
func FormatDecAmount(amount math.LegacyDec, denom string) string {
	if isMicroDenom(denom) {
		return formatMicroDenomDec(amount, denom)
	}
	return fmt.Sprintf("%s %s", TrimDecTrailingZeros(amount.String()), denom)
}

// FormatCoins formats a list of sdk.Coins. Each coin is formatted via FormatCoin.
// Multiple coins are joined with ", ".
func FormatCoins(coins sdk.Coins) string {
	if len(coins) == 0 {
		return "0"
	}
	parts := make([]string, len(coins))
	for i, c := range coins {
		parts[i] = FormatCoin(c)
	}
	return strings.Join(parts, ", ")
}

// FormatDecCoins formats a list of sdk.DecCoins. Each coin is formatted via FormatDecCoin.
// Multiple coins are joined with ", ".
func FormatDecCoins(coins sdk.DecCoins) string {
	if len(coins) == 0 {
		return "0"
	}
	parts := make([]string, len(coins))
	for i, c := range coins {
		parts[i] = FormatDecCoin(c)
	}
	return strings.Join(parts, ", ")
}

// formatMicroDenomDec is the LegacyDec counterpart of formatMicroDenom.
// It scales micro-denominated Dec amounts to the most readable unit.
func formatMicroDenomDec(amount math.LegacyDec, denom string) string {
	base := strings.ToUpper(denom[1:])

	million := math.LegacyNewDec(1_000_000)
	thousand := math.LegacyNewDec(1_000)

	absAmt := amount
	if absAmt.IsNegative() {
		absAmt = absAmt.Neg()
	}

	switch {
	case absAmt.IsZero():
		return fmt.Sprintf("0 %s", base)
	case absAmt.GTE(million):
		d := amount.Quo(million)
		return fmt.Sprintf("%s %s", TrimDecTrailingZeros(d.String()), base)
	case absAmt.GTE(thousand):
		d := amount.Quo(thousand)
		return fmt.Sprintf("%s m%s", TrimDecTrailingZeros(d.String()), base)
	default:
		return fmt.Sprintf("%s u%s", TrimDecTrailingZeros(amount.String()), base)
	}
}

// TrimDecTrailingZeros removes unnecessary trailing zeros from a decimal string.
// "100000.000000000000000000" → "100000", "1.500000" → "1.5", "0.100" → "0.1"
func TrimDecTrailingZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// priceDecimals is the precision every price is reported at. The oracle
// publishes its source prices with 8 decimal places (`0.53598949`), but values
// the chain derives from them — a TWAP, a remint credit price — are LegacyDecs
// carrying all 18 (`0.536004234885265376`). Reporting both as they arrive puts
// two widths on the same quantity in the same view. Rounding derived prices to
// the oracle's own precision loses nothing real and makes them comparable.
const priceDecimals = 8

// FormatPriceDec formats a decimal price at priceDecimals with trailing zeros
// stripped, guarding the nil Dec proto3 leaves for an unset field.
//
// Prices are rounded far finer than ratios: they are quoted in whole units, so
// AKT near `0.003125` must survive intact.
func FormatPriceDec(d math.LegacyDec) string {
	return TrimDecTrailingZeros(roundDec(DecOrZero(d), priceDecimals).String())
}

// roundDec rounds a decimal to places decimal places, half away from zero.
func roundDec(d math.LegacyDec, places int64) math.LegacyDec {
	scale := math.LegacyNewDec(10).Power(uint64(places))

	return math.LegacyNewDecFromInt(d.Mul(scale).RoundInt()).Quo(scale)
}

// FormatHeight formats a block height with comma grouping.
func FormatHeight(height int64) string {
	if height <= 0 {
		return "-"
	}
	return formatWithCommas(height)
}

// formatWithCommas adds comma separators to an integer.
func formatWithCommas(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteByte(',')
		}
	}
	for i := remainder; i < len(s); i += 3 {
		if i > remainder {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}

// ColorState returns a lipgloss-styled state string with appropriate color.
func ColorState(state string) string {
	lower := strings.ToLower(state)
	switch lower {
	case "active", "open", "bonded", "passed", "valid":
		return StyleGreen.Render(state)
	case "paused", "insufficient_funds", "overdrawn", "unbonding",
		"voting_period", "deposit_period":
		return StyleYellow.Render(state)
	case "closed", "lost", "unbonded", "rejected", "failed",
		"jailed", "revoked":
		return StyleRed.Render(state)
	case "invalid", "unspecified":
		return StyleGray.Render(state)
	default:
		return state
	}
}

// Bold renders text in bold.
func Bold(s string) string {
	return StyleBold.Render(s)
}

// Dim renders text in a faint style.
func Dim(s string) string {
	return StyleDim.Render(s)
}

// Section renders a section header.
func Section(title string) string {
	return StyleSection.Render(title)
}

// Key column geometry for KV/SubKV (SPEC §10.12).
//
// SubKV is indented SubKVIndentDelta columns deeper than KV, so its key column
// is that much narrower and both land their values in the same column. Padding
// never truncates, so a key wider than its column pushes its own value out of
// line: a section whose keys do not fit the defaults must widen BOTH columns
// together, via KVWidth and SubKVWidth, keeping the delta.
const (
	// KVKeyWidth is the default KV key column width.
	KVKeyWidth = 20
	// SubKVIndentDelta is how much deeper SubKV indents than KV.
	SubKVIndentDelta = 4
	// SubKVKeyWidth is the SubKV key column width matching KVKeyWidth.
	SubKVKeyWidth = KVKeyWidth - SubKVIndentDelta
)

// KV writes a key-value pair with consistent alignment.
// The key is rendered in the dim/faint style with a trailing colon.
// Padding is ANSI-aware so styled keys align correctly.
func KV(w io.Writer, key, value string) {
	KVWidth(w, KVKeyWidth, key, value)
}

// KVWidth writes a key-value pair like KV but with a custom key column width.
// Use this when the default KV width (KVKeyWidth) is too narrow for the keys in
// a section; pair it with SubKVWidth(width-SubKVIndentDelta) for any SubKV
// entries in the same section.
func KVWidth(w io.Writer, width int, key, value string) {
	styled := StyleKey.Render(key + ":")
	fmt.Fprintf(w, "  %s %s\n", padRight(styled, width), value)
}

// KVHeader writes a key-only line that introduces a group of SubKV entries.
// Rendered at the same indent as KV but with no value.
func KVHeader(w io.Writer, key string) {
	styled := StyleKey.Render(key + ":")
	fmt.Fprintf(w, "  %s\n", styled)
}

// SubKV writes an indented sub-key-value pair for nested sections.
// Uses 6-space indent (vs KV's 2-space) with a narrower key column
// so that values align with parent KV entries at the same column.
func SubKV(w io.Writer, key, value string) {
	SubKVWidth(w, SubKVKeyWidth, key, value)
}

// SubKVWidth writes a sub-key-value pair like SubKV but with a custom key
// column width. It is the SubKV counterpart of KVWidth: a section that widens
// its KV column to width must widen its SubKV column to
// width-SubKVIndentDelta, or the two stop sharing a value column.
func SubKVWidth(w io.Writer, width int, key, value string) {
	styled := StyleKey.Render(key + ":")
	fmt.Fprintf(w, "      %s %s\n", padRight(styled, width), value)
}

// FormatResourceBytes formats a byte count (from ResourceValue) as a
// human-readable string (e.g. "512 Mi", "2 Gi", "1.5 Ti").
// Uses the largest unit where the value is >= 1, with up to one decimal.
func FormatResourceBytes(val math.Int) string {
	const (
		ki float64 = 1024
		mi         = ki * 1024
		gi         = mi * 1024
		ti         = gi * 1024
	)

	v := float64(val.Int64())

	switch {
	case v >= ti:
		if int64(v)%int64(ti) == 0 {
			return fmt.Sprintf("%d Ti", int64(v/ti))
		}
		return fmt.Sprintf("%.1f Ti", v/ti)
	case v >= gi:
		if int64(v)%int64(gi) == 0 {
			return fmt.Sprintf("%d Gi", int64(v/gi))
		}
		return fmt.Sprintf("%.1f Gi", v/gi)
	case v >= mi:
		if int64(v)%int64(mi) == 0 {
			return fmt.Sprintf("%d Mi", int64(v/mi))
		}
		return fmt.Sprintf("%.1f Mi", v/mi)
	case v >= ki:
		if int64(v)%int64(ki) == 0 {
			return fmt.Sprintf("%d Ki", int64(v/ki))
		}
		return fmt.Sprintf("%.1f Ki", v/ki)
	default:
		return fmt.Sprintf("%d bytes", int64(v))
	}
}

// FormatCPU formats CPU millicores as a human-readable string.
// CPU units in Akash are millicores (1000 = 1 CPU).
func FormatCPU(val math.Int) string {
	m := val.Int64()
	if m%1000 == 0 {
		return fmt.Sprintf("%d", m/1000)
	}
	return fmt.Sprintf("%.1f", float64(m)/1000.0)
}

// displayWidth returns the visible display width of a string in terminal
// columns, correctly handling ANSI escape sequences (SGR, CSI, OSC) and
// multi-width Unicode characters (e.g., East Asian fullwidth). Delegates to
// lipgloss.Width which uses charmbracelet/x/ansi and go-runewidth internally.
func displayWidth(s string) int {
	return lipgloss.Width(s)
}

// AlignLeft is the default column alignment (pad right).
const AlignLeft = 0

// AlignRight pads on the left so values are right-aligned.
const AlignRight = 1

// ColDef defines a table column with a header and optional alignment.
type ColDef struct {
	Header string
	Align  int // AlignLeft (default) or AlignRight
}

// padRight pads a string to width based on its display width (ANSI-aware).
func padRight(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}

// padLeft pads a string on the left to width based on its display width (ANSI-aware).
func padLeft(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return strings.Repeat(" ", width-dw) + s
}

// DefaultEmptyMessage is what a table prints when it has no rows and the
// caller did not name what is missing. Callers should name it — see
// WriteTableOrEmpty.
const DefaultEmptyMessage = "(no results)"

// colDefs converts plain string headers into left-aligned column definitions.
func colDefs(headers []string) []ColDef {
	cols := make([]ColDef, len(headers))
	for i, h := range headers {
		cols[i] = ColDef{Header: h}
	}
	return cols
}

// WriteTableOrEmpty writes a table with string headers, or emptyMsg (dimmed) when
// there are no rows. emptyMsg names what was searched for — "(no deployments)",
// "(no bids)" — because a table header with nothing under it reads as a
// rendering failure rather than as an empty result (SPEC §10.3).
func WriteTableOrEmpty(w io.Writer, headers []string, rows [][]string, emptyMsg string) {
	WriteTableColsOrEmpty(w, colDefs(headers), rows, emptyMsg)
}

// WriteTableColsOrEmpty is WriteTableOrEmpty for tables with column
// definitions (supporting alignment).
func WriteTableColsOrEmpty(w io.Writer, cols []ColDef, rows [][]string, emptyMsg string) {
	if len(rows) == 0 {
		fmt.Fprintln(w, Dim(emptyMsg))
		return
	}
	WriteTableCols(w, cols, rows)
}

// WriteTableCols writes a table with column definitions (supporting alignment)
// and rows to w. Column widths are computed based on display width (stripping
// ANSI escape codes) so that styled text does not break alignment. Each header
// is aligned like the column it labels.
//
// With no rows it writes DefaultEmptyMessage rather than a bare header; prefer
// WriteTableColsOrEmpty, which names what is missing.
func WriteTableCols(w io.Writer, cols []ColDef, rows [][]string) {
	const colGap = 2

	if len(rows) == 0 {
		fmt.Fprintln(w, Dim(DefaultEmptyMessage))
		return
	}

	// Style headers.
	styledHeaders := make([]string, len(cols))
	for i, c := range cols {
		styledHeaders[i] = StyleHeader.Render(c.Header)
	}

	// Compute column widths from display widths.
	colWidths := make([]int, len(cols))
	for i := range cols {
		colWidths[i] = displayWidth(styledHeaders[i])
	}

	for _, row := range rows {
		for i := 0; i < len(row) && i < len(colWidths); i++ {
			dw := displayWidth(row[i])
			if dw > colWidths[i] {
				colWidths[i] = dw
			}
		}
	}

	// Print header. A header is padded exactly like the column it labels, so
	// it sits over its own data instead of floating mid-column.
	for i, h := range styledHeaders {
		if i > 0 {
			fmt.Fprint(w, strings.Repeat(" ", colGap))
		}
		if cols[i].Align == AlignRight {
			fmt.Fprint(w, padLeft(h, colWidths[i]))
		} else {
			fmt.Fprint(w, padRight(h, colWidths[i]))
		}
	}
	fmt.Fprintln(w)

	// Print rows.
	for _, row := range rows {
		for i := 0; i < len(row); i++ {
			if i > 0 {
				fmt.Fprint(w, strings.Repeat(" ", colGap))
			}
			if i < len(colWidths) {
				if cols[i].Align == AlignRight {
					fmt.Fprint(w, padLeft(row[i], colWidths[i]))
				} else {
					fmt.Fprint(w, padRight(row[i], colWidths[i]))
				}
			} else {
				fmt.Fprint(w, row[i])
			}
		}
		fmt.Fprintln(w)
	}
}

// Newline writes an empty line to w.
func Newline(w io.Writer) {
	fmt.Fprintln(w)
}

// FormatDuration formats a time.Duration as a human-readable string.
//
//	21 days, 7d 12h 30m, 10h 5m, 5m 30s, 30s
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}

	totalSec := int64(d.Seconds())
	days := totalSec / 86400
	hours := (totalSec % 86400) / 3600
	minutes := (totalSec % 3600) / 60
	seconds := totalSec % 60

	switch {
	case days > 0 && hours == 0 && minutes == 0 && seconds == 0:
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	case hours > 0 && minutes == 0 && seconds == 0:
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0 && seconds == 0:
		return fmt.Sprintf("%dm", minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

// FormatDurationString parses a proto-JSON duration string (e.g. "1814400s")
// and formats it as a human-readable string via FormatDuration.
// Falls back to returning the raw string on parse failure.
func FormatDurationString(s string) string {
	if s == "" || s == "0s" {
		return "0s"
	}

	// Proto-JSON encodes Duration as "{seconds}s" or "{seconds}.{nanos}s".
	if strings.HasSuffix(s, "s") {
		trimmed := strings.TrimSuffix(s, "s")
		seconds, err := strconv.ParseFloat(trimmed, 64)
		if err == nil {
			return FormatDuration(time.Duration(seconds * float64(time.Second)))
		}
	}

	// Try Go's time.ParseDuration as fallback.
	d, err := time.ParseDuration(s)
	if err != nil {
		return s
	}
	return FormatDuration(d)
}

// FormatPercent converts a Cosmos SDK decimal string (e.g. "0.334000000000000000")
// to a human-readable percentage (e.g. "33.4%"). Trailing zeros are stripped.
// Returns the raw string on parse failure.
func FormatPercent(s string) string {
	if s == "" {
		return "0%"
	}

	d, err := math.LegacyNewDecFromStr(s)
	if err != nil {
		return s
	}

	return FormatPercentDec(d)
}

// FormatPercentDec converts a math.LegacyDec to a percentage string.
func FormatPercentDec(d math.LegacyDec) string {
	pct := DecOrZero(d).MulInt64(100)
	return TrimDecTrailingZeros(pct.String()) + "%"
}

// FormatBool formats a boolean as a color-coded "Yes" / "No" string.
func FormatBool(b bool) string {
	if b {
		return StyleGreen.Render("Yes")
	}
	return "No"
}

// ─── Shared format helpers (used by both CLI pretty output and TUI) ──

// FormatNumber formats an int64 with comma-separated thousands.
// Example: 18234567 → "18,234,567"
func FormatNumber(n int64) string {
	return formatWithCommas(n)
}

// FormatPower formats voting power in a compact human-readable way.
// Example: 1500000 → "1.5M", 2500 → "2.5K"
func FormatPower(power int64) string {
	if power >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(power)/1_000_000_000)
	}
	if power >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(power)/1_000_000)
	}
	if power >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(power)/1_000)
	}
	return fmt.Sprintf("%d", power)
}

// FormatShortDuration formats a duration for compact display (block times).
// Uses ms/s/m format: "350ms", "3.5s", "1m30s"
func FormatShortDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%.0fs", int(d.Minutes()), d.Seconds()-float64(int(d.Minutes()))*60)
}

// FormatBytes formats a byte count as a human-readable string using
// binary units: Ki, Mi, Gi, Ti.
func FormatBytes(bytes uint64) string {
	const (
		ki = 1024
		mi = 1024 * ki
		gi = 1024 * mi
		ti = 1024 * gi
	)
	switch {
	case bytes >= ti:
		return fmt.Sprintf("%.0fTi", float64(bytes)/float64(ti))
	case bytes >= gi:
		return fmt.Sprintf("%.0fGi", float64(bytes)/float64(gi))
	case bytes >= mi:
		return fmt.Sprintf("%dMi", bytes/mi)
	default:
		return fmt.Sprintf("%d", bytes)
	}
}

// FormatMemoryRatio formats a memory available/total ratio as "avail/total"
// using human-readable byte units.
func FormatMemoryRatio(available, total uint64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%s/%s", FormatBytes(available), FormatBytes(total))
}

// FormatResourceRatio formats an available/total resource count as "avail/total".
func FormatResourceRatio(available, total uint64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d", available, total)
}
