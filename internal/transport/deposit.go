package transport

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// depositAuto is the deposit value that defers to the rail's default: the
// chain rail resolves it to the chain-minimum deployment deposit; the
// console rail rejects it (an explicit USD amount is required).
const depositAuto = "auto"

// MinConsoleDepositUSD is the minimum deployment deposit the Console API
// accepts, in USD (SPEC §7.4: console-api deposits are USD, not uakt).
// Exported so every surface enforcing the console minimum (CLI commands,
// workflow adapters) shares one value instead of hard-coding 0.5.
const MinConsoleDepositUSD = 0.5

// Deposit is a deployment deposit parsed from the unified --deposit syntax
// accepted on every rail (see ParseDeposit).
type Deposit struct {
	// Raw is the original input string, preserved for pass-through and
	// error messages.
	Raw string

	// Auto reports the rail-default form: empty or "auto".
	Auto bool

	// IsUSD reports whether the deposit is a USD amount (explicit "5usd"/
	// "$5" forms, and bare numbers, which are USD on the console rail).
	IsUSD bool

	// USD is the deposit amount in USD when IsUSD is true.
	USD float64

	// Coin is the chain coin string (e.g. "5000000uakt") when the deposit
	// is a coin amount.
	Coin string

	// Bare reports that the input was a bare number with no unit. Bare
	// amounts are USD on the console rail; the chain rail rejects them
	// (coins require a denomination), exactly as it always has.
	Bare bool
}

// ParseDeposit parses the unified deposit syntax shared by every transport
// (SPEC §7.4). Exactly one syntax is accepted on all rails; each rail then
// interprets it via Deposit.RailValue:
//
//	""/"auto"               -> rail default (chain: chain-minimum deposit;
//	                           console: error, explicit USD required)
//	"5usd", "$5", "5.50usd" -> USD amount (console rail only)
//	"5000000uakt", "5akt"   -> coin amount (chain rail only)
//	"5", "5.50"             -> bare number: USD on the console rail;
//	                           rejected on the chain rail (coins need a
//	                           denomination — the historical behavior)
//
// The "usd" unit is case-insensitive and always wins over coin parsing, so a
// value ending in "usd" is never treated as a chain denomination.
func ParseDeposit(s string) (Deposit, error) {
	t := strings.TrimSpace(s)

	if t == "" || t == depositAuto {
		return Deposit{Raw: s, Auto: true}, nil
	}

	// USD: "$5", "$5.50".
	if rest, ok := strings.CutPrefix(t, "$"); ok {
		usd, err := parseUSDAmount(rest)
		if err != nil {
			return Deposit{}, fmt.Errorf("invalid deposit %q: %w", s, err)
		}

		return Deposit{Raw: s, IsUSD: true, USD: usd}, nil
	}

	// USD: "5usd", "5.50USD". Checked before coin parsing so "usd" is never
	// treated as a chain denomination.
	if rest, ok := cutSuffixFold(t, "usd"); ok {
		usd, err := parseUSDAmount(rest)
		if err != nil {
			return Deposit{}, fmt.Errorf("invalid deposit %q: %w", s, err)
		}

		return Deposit{Raw: s, IsUSD: true, USD: usd}, nil
	}

	// Bare number: valid syntax on both rails, interpreted per rail.
	if usd, err := strconv.ParseFloat(t, 64); err == nil {
		if err := validUSDAmount(usd); err != nil {
			return Deposit{}, fmt.Errorf("invalid deposit %q: %w", s, err)
		}

		return Deposit{Raw: s, IsUSD: true, USD: usd, Bare: true}, nil
	}

	// Coin: "5000000uakt", "5akt", "5.5akt", ...
	if _, err := sdk.ParseDecCoin(t); err == nil {
		return Deposit{Raw: s, Coin: t}, nil
	}

	return Deposit{}, fmt.Errorf(
		"invalid deposit %q: use a USD amount (e.g. 5usd or $5) or a coin amount (e.g. 5000000uakt)", s)
}

// RailValue returns the rail-native deposit string for the given transport
// kind, or a clear cross-rail error when the deposit form is not valid on
// that rail.
//
// Chain rail: ""/"auto" pass through (the adapter resolves the chain-minimum
// deposit), coin strings pass through to coin parsing, and USD or bare
// amounts are rejected. Console rail: USD and bare amounts become a plain
// USD number (the Console API wire form), ""/"auto" pass through (the
// adapter reports that an explicit USD deposit is required), and coin
// amounts are rejected.
func (d Deposit) RailValue(kind Kind) (string, error) {
	switch kind {
	case KindChain:
		switch {
		case d.Auto:
			return d.Raw, nil
		case d.Coin != "":
			return d.Coin, nil
		case d.Bare:
			return "", fmt.Errorf(
				"deposit %q: a bare amount is a USD deposit, and USD deposits require a console-api context; specify a coin amount like 5000000uakt", d.Raw)
		default:
			return "", fmt.Errorf(
				"deposit %q: USD deposits require a console-api context; specify a coin amount like 5000000uakt", d.Raw)
		}

	case KindConsole:
		switch {
		case d.Auto:
			return d.Raw, nil
		case d.IsUSD:
			return strconv.FormatFloat(d.USD, 'f', -1, 64), nil
		default:
			return "", fmt.Errorf(
				"deposit %q: console deposits are in USD; use e.g. 5usd", d.Raw)
		}

	default:
		return "", fmt.Errorf("unknown transport kind %q", kind)
	}
}

// parseUSDAmount parses the numeric part of a USD deposit form.
func parseUSDAmount(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("missing USD amount")
	}

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid USD amount %q", s)
	}

	if err := validUSDAmount(v); err != nil {
		return 0, err
	}

	return v, nil
}

// validUSDAmount rejects negative and non-finite USD amounts. Zero is left
// to the rail (the console adapter enforces its own USD minimum).
func validUSDAmount(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("USD amount must be a finite number")
	}

	if v < 0 {
		return fmt.Errorf("USD amount must not be negative")
	}

	return nil
}

// cutSuffixFold is strings.CutSuffix with ASCII case-insensitive matching.
func cutSuffixFold(s, suffix string) (string, bool) {
	if len(s) < len(suffix) || !strings.EqualFold(s[len(s)-len(suffix):], suffix) {
		return s, false
	}

	return s[:len(s)-len(suffix)], true
}
