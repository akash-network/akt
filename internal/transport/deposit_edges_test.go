package transport

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestRailValueUnknownKind covers the default arm of the rail switch. A new
// Kind added without a RailValue case must produce an explicit error rather
// than silently returning an empty deposit string, which downstream would read
// as "auto" and could deploy with the wrong funding.
func TestRailValueUnknownKind(t *testing.T) {
	dep, err := ParseDeposit("5usd")
	if err != nil {
		t.Fatalf("ParseDeposit: %v", err)
	}

	value, err := dep.RailValue(Kind("ibc"))
	if err == nil {
		t.Fatal("an unknown transport kind must be rejected")
	}
	if value != "" {
		t.Errorf("value = %q, want empty on error", value)
	}
	if !strings.Contains(err.Error(), "unknown transport kind") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestParseDepositRejectsNonFiniteAmounts covers validUSDAmount's NaN/Inf arm.
// Go's ParseFloat accepts "NaN" and "Inf", and either would serialize into a
// Console deposit request as a value the API cannot interpret.
func TestParseDepositRejectsNonFiniteAmounts(t *testing.T) {
	for _, in := range []string{"NaN", "Inf", "+Inf", "-Inf", "$NaN", "Infusd", "NaNusd"} {
		if dep, err := ParseDeposit(in); err == nil {
			t.Errorf("ParseDeposit(%q) = %+v, want an error", in, dep)
		}
	}

	// Sanity: the guard is what rejects them, not a parse failure upstream.
	if _, err := strconv.ParseFloat("NaN", 64); err != nil {
		t.Fatalf("precondition: ParseFloat should accept NaN, got %v", err)
	}
	if err := validUSDAmount(math.NaN()); err == nil {
		t.Error("validUSDAmount(NaN) must be an error")
	}
	if err := validUSDAmount(math.Inf(1)); err == nil {
		t.Error("validUSDAmount(+Inf) must be an error")
	}
}

// TestCutSuffixFoldBoundaries covers the length guard in the case-insensitive
// suffix cut. Without it a short input like "sd" would slice out of range.
func TestCutSuffixFoldBoundaries(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"5usd", "5", true},
		{"5USD", "5", true},
		{"5UsD", "5", true},
		{"usd", "", true}, // suffix equals the whole string
		{"sd", "sd", false},
		{"", "", false},
		{"5uakt", "5uakt", false},
	}

	for _, c := range cases {
		got, ok := cutSuffixFold(c.in, "usd")
		if got != c.want || ok != c.wantOK {
			t.Errorf("cutSuffixFold(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

// TestTranslateDepositParamDoesNotMutateInput pins that the rail translation
// leaves the caller's params map alone. Workflow steps reuse their params map
// across retries, so an in-place rewrite would double-translate on the second
// attempt.
func TestTranslateDepositParamDoesNotMutateInput(t *testing.T) {
	params := map[string]string{"deposit": "5usd", "dseq": "12345"}

	out, err := translateDepositParam(KindConsole, params)
	if err != nil {
		t.Fatalf("translateDepositParam: %v", err)
	}

	if params["deposit"] != "5usd" {
		t.Errorf("input map was mutated: deposit = %q", params["deposit"])
	}
	if out["deposit"] != "5" {
		t.Errorf("translated deposit = %q, want the console wire form 5", out["deposit"])
	}
	if out["dseq"] != "12345" {
		t.Errorf("unrelated params must be carried over, got %v", out)
	}
}

// TestTranslateDepositParamReusesMapWhenUnchanged covers the fast path: when
// the rail-native value equals the raw input there is nothing to rewrite, and
// the original map is returned as-is.
func TestTranslateDepositParamReusesMapWhenUnchanged(t *testing.T) {
	params := map[string]string{"deposit": "5000000uakt"}

	out, err := translateDepositParam(KindChain, params)
	if err != nil {
		t.Fatalf("translateDepositParam: %v", err)
	}
	if out["deposit"] != "5000000uakt" {
		t.Errorf("chain coin deposits must pass through, got %q", out["deposit"])
	}

	// "auto" also passes through untouched on the chain rail.
	autoParams := map[string]string{"deposit": "auto"}
	out, err = translateDepositParam(KindChain, autoParams)
	if err != nil {
		t.Fatalf("translateDepositParam(auto): %v", err)
	}
	if out["deposit"] != "auto" {
		t.Errorf(`"auto" must pass through on the chain rail, got %q`, out["deposit"])
	}
}

// TestMinConsoleDepositMatchesClientConstant pins the single-source-of-truth
// claim in the doc comment. If the console client's minimum ever drifts from
// the transport constant, the CLI would accept a deposit the API rejects.
func TestMinConsoleDepositMatchesClientConstant(t *testing.T) {
	if MinConsoleDepositUSD != 0.5 {
		t.Errorf("MinConsoleDepositUSD = %v, want 0.5", MinConsoleDepositUSD)
	}

	dep, err := ParseDeposit("0.5usd")
	if err != nil {
		t.Fatalf("ParseDeposit: %v", err)
	}
	if dep.USD < MinConsoleDepositUSD {
		t.Errorf("the documented minimum must itself be acceptable, got %v", dep.USD)
	}
}
