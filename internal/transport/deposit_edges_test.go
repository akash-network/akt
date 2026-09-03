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
	params := map[string]string{"deposit": "auto", "dseq": "12345"}

	out, err := translateDepositParam(KindConsole, params)
	if err != nil {
		t.Fatalf("translateDepositParam: %v", err)
	}

	if params["deposit"] != "auto" {
		t.Errorf("input map was mutated: deposit = %q", params["deposit"])
	}
	if out["deposit"] != "" {
		t.Errorf("translated deposit = %q, want empty: the console rail sends no deposit", out["deposit"])
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

// TestConsoleRailRejectsEveryExplicitDeposit pins the console rail's whole
// contract: credits fund the deployment, so only the rail default resolves and
// every explicit form is refused locally rather than sent to an API that
// discards it. The rejection must name the docs so a user knows where to look.
func TestConsoleRailRejectsEveryExplicitDeposit(t *testing.T) {
	for _, in := range []string{"5usd", "$5", "5", "0.5usd", "5000000uakt", "5akt"} {
		dep, err := ParseDeposit(in)
		if err != nil {
			t.Fatalf("ParseDeposit(%q): %v", in, err)
		}

		got, err := dep.RailValue(KindConsole)
		if err == nil {
			t.Errorf("RailValue(console) for %q: expected a rejection, got %q", in, got)
			continue
		}
		if !strings.Contains(err.Error(), FundingDocsURL) {
			t.Errorf("RailValue(console) for %q: error %q does not point at %s", in, err, FundingDocsURL)
		}
	}
}
