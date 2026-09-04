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
// console rail sends no deposit at all.
const depositAuto = "auto"

// FundingDocsURL explains how Console funds deployments from account credits.
// The console rail's deposit rejection is the one place a user is told that
// something they typed no longer exists, so it points here.
const FundingDocsURL = "https://akash.network/docs/getting-started/how-funding-works/"

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
//	"5usd", "$5", "5.50usd" -> USD amount (console rail only; plain
//	                              decimal notation with at most two
//	                              fractional digits)
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
		return Deposit{Raw: t, Auto: true}, nil
	}

	// USD: "$5", "$5.50".
	if rest, ok := strings.CutPrefix(t, "$"); ok {
		usd, err := parseUSDAmount(rest)
		if err != nil {
			return Deposit{}, fmt.Errorf("invalid deposit %q: %w", s, err)
		}

		return Deposit{Raw: t, IsUSD: true, USD: usd}, nil
	}

	// USD: "5usd", "5.50USD". Checked before coin parsing so "usd" is never
	// treated as a chain denomination.
	if rest, ok := cutSuffixFold(t, "usd"); ok {
		usd, err := parseUSDAmount(rest)
		if err != nil {
			return Deposit{}, fmt.Errorf("invalid deposit %q: %w", s, err)
		}

		return Deposit{Raw: t, IsUSD: true, USD: usd}, nil
	}

	// Bare number: valid syntax on both rails, interpreted per rail. Values
	// that look numeric stay on the USD parsing path so syntax such as an
	// exponent or a Go numeric separator cannot fall through to coin parsing.
	if looksLikeBareUSD(t) {
		usd, err := parseUSDAmount(t)
		if err != nil {
			return Deposit{}, fmt.Errorf("invalid deposit %q: %w", s, err)
		}

		return Deposit{Raw: t, IsUSD: true, USD: usd, Bare: true}, nil
	}

	// Coin: "5000000uakt", "5akt", "5.5akt", ...
	if _, err := sdk.ParseDecCoin(t); err == nil {
		return Deposit{Raw: t, Coin: t}, nil
	}

	return Deposit{}, fmt.Errorf(
		"invalid deposit %q: use auto, a USD amount (e.g. 5usd or $5), or an explicit chain coin amount", s)
}

// RailValue returns the rail-native deposit string for the given transport
// kind, or a clear cross-rail error when the deposit form is not valid on
// that rail.
//
// Chain rail: ""/"auto" pass through (the adapter resolves the chain-minimum
// deposit), coin strings pass through to coin parsing, and USD or bare
// amounts are rejected. Console rail: only ""/"auto" is accepted, and it
// resolves to no deposit at all, because the Console funds every deployment
// from the account's credits. An explicit amount is rejected here rather than sent,
// because POST /v1/deployments discards it: a user who typed a number would
// otherwise believe it bounded their spend.
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
				"deposit %q: a bare amount has no denomination; use auto (recommended), or specify an explicit coin amount in the network's deployment deposit denomination", d.Raw)
		default:
			return "", fmt.Errorf(
				"deposit %q: chain deposits are coins, not USD; use auto (recommended), or specify an explicit coin amount in the network's deployment deposit denomination", d.Raw)
		}

	case KindConsole:
		if d.Auto {
			return "", nil
		}

		return "", fmt.Errorf(
			"deposit %q: console deployments are funded automatically from your account credits, so they take no deposit; drop the argument. See %s", d.Raw, FundingDocsURL)

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
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("USD amount must not be negative")
	}

	integer, fraction, hasPoint := strings.Cut(s, ".")
	if integer == "" || !decimalDigits(integer) || (hasPoint && (fraction == "" || !decimalDigits(fraction))) {
		return 0, fmt.Errorf("invalid USD amount %q: use plain decimal notation", s)
	}
	if hasPoint && len(fraction) > 2 {
		return 0, fmt.Errorf("USD amount must have at most two fractional digits")
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

func decimalDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return s != ""
}

func looksLikeBareUSD(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r == '.', r == '+', r == '-', r == 'e', r == 'E', r == '_':
		default:
			return false
		}
	}

	return s != ""
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
