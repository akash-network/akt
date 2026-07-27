package capability_test

import (
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/capability"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// TestHasFailsOpenOnUnknownCapability covers the default arm of Set.Has
// directly. The gating layer reads capability names from cobra annotations, so
// a typo'd annotation must never hide a working command — failing open is the
// documented safety choice, and this is the branch that implements it.
func TestHasFailsOpenOnUnknownCapability(t *testing.T) {
	empty := capability.Set{}

	if !empty.Has(capability.Capability("chian-query")) {
		t.Error("an unknown capability must fail open (never gate)")
	}
	if !empty.Has("") {
		t.Error("an empty capability must fail open")
	}

	// Every known capability still gates on the empty set.
	for _, c := range []capability.Capability{
		capability.ChainQuery, capability.ChainTx, capability.Console, capability.Provider,
	} {
		if empty.Has(c) {
			t.Errorf("%s must not be satisfied by the empty set", c)
		}
	}
}

// TestExplainFallsBackForUnknownRequirements covers Explain's "no known
// capability" branch. An annotation nobody has a remedy for must still produce
// a usable message rather than an empty string appended to an error.
func TestExplainFallsBackForUnknownRequirements(t *testing.T) {
	s := capability.Set{}

	msg := s.Explain("teleportation")
	if msg == "" {
		t.Fatal("Explain must never return an empty message")
	}
	if !strings.Contains(msg, "teleportation") {
		t.Errorf("fallback message should quote the requirement, got %q", msg)
	}

	// A mixed expression keeps only the remedies it knows about.
	msg = s.Explain("teleportation|console")
	if strings.Contains(msg, "teleportation") {
		t.Errorf("unknown alternatives must be dropped when a known one exists, got %q", msg)
	}
	if !strings.Contains(msg, "akt console login") {
		t.Errorf("the known alternative's remedy is missing from %q", msg)
	}
	// Exactly one capability survived, so exactly one "<name> (" clause is
	// listed — the remedy text itself contains commas, so count clauses
	// rather than searching for a separator.
	if got := strings.Count(msg, " ("); got != 2 {
		t.Errorf("expected a single capability clause, got %q", msg)
	}
}

// TestResolveIgnoresNetworkWithoutRPC pins the exact signal each capability
// keys off. A context that names a network with no RPC endpoints cannot query
// or transact, and a chain-less console context must not be offered chain
// commands just because it has a network entry.
func TestResolveIgnoresNetworkWithoutRPC(t *testing.T) {
	cases := []struct {
		name string
		rc   *aktctx.Context
		want capability.Set
	}{
		{
			"named network, no endpoints",
			&aktctx.Context{Network: aktctx.Network{Name: "mainnet"}},
			capability.Set{},
		},
		{
			"empty rpc list",
			&aktctx.Context{Network: aktctx.Network{Endpoints: aktctx.Endpoints{RPC: []string{}}}},
			capability.Set{},
		},
		{
			"rest/grpc only",
			&aktctx.Context{Network: aktctx.Network{Endpoints: aktctx.Endpoints{
				API:  []string{"https://api"},
				GRPC: []string{"grpc:9090"},
			}}},
			capability.Set{},
		},
		{
			"console key present but empty",
			&aktctx.Context{ConsoleAPIKey: ""},
			capability.Set{},
		},
	}

	for _, c := range cases {
		if got := capability.Resolve(c.rc); got != c.want {
			t.Errorf("%s: Resolve = %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestSatisfiesWithAllAlternativesUnsatisfied pins the multi-alternative
// rejection that gates workflow commands ("chain-tx|console"): a context with
// neither rail must be told it has neither.
func TestSatisfiesWithAllAlternativesUnsatisfied(t *testing.T) {
	empty := capability.Set{}

	if empty.Satisfies("chain-tx|console") {
		t.Error("a context with neither rail must not satisfy chain-tx|console")
	}

	msg := empty.Explain("chain-tx|console")
	if !strings.Contains(msg, ", or ") {
		t.Errorf("both remedies should be offered, got %q", msg)
	}
	if !strings.Contains(msg, "akt context network edit") || !strings.Contains(msg, "akt console login") {
		t.Errorf("explanation missing a remedy: %q", msg)
	}
}

// TestModeStringsAreStable pins the configured gating-mode values. They come
// from user config (defaults.command-gating), so renaming one silently
// downgrades every existing config to the default mode.
func TestModeStringsAreStable(t *testing.T) {
	cases := map[capability.Mode]string{
		capability.ModeDim:  "dim",
		capability.ModeHide: "hide",
		capability.ModeOff:  "off",
	}

	for mode, want := range cases {
		if string(mode) != want {
			t.Errorf("mode %v = %q, want %q", mode, string(mode), want)
		}
		if got := capability.ParseMode("  " + strings.ToUpper(want) + "  "); got != mode {
			t.Errorf("ParseMode with padding/case = %v, want %v", got, mode)
		}
	}
}

// TestCapabilityNamesAreStable pins the annotation vocabulary. The values are
// written into cobra annotations across the command tree; renaming one without
// updating every call site silently un-gates those commands (Has fails open).
func TestCapabilityNamesAreStable(t *testing.T) {
	cases := map[capability.Capability]string{
		capability.ChainQuery: "chain-query",
		capability.ChainTx:    "chain-tx",
		capability.Console:    "console",
		capability.Provider:   "provider",
	}

	for c, want := range cases {
		if string(c) != want {
			t.Errorf("capability %v = %q, want %q", c, string(c), want)
		}
	}

	if capability.AnnotationKey != "akt.requires" {
		t.Errorf("AnnotationKey = %q, want akt.requires", capability.AnnotationKey)
	}
}
