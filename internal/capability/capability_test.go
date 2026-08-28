package capability_test

import (
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/capability"
	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestResolve(t *testing.T) {
	cases := []struct {
		name string
		rc   *aktctx.Context
		want capability.Set
	}{
		{"nil context", nil, capability.Set{}},
		{
			"keyring with rpc",
			&aktctx.Context{
				Network: aktctx.Network{Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc"}}},
				Keyring: aktctx.Keyring{Name: "default"},
			},
			capability.Set{ChainQuery: true, ChainTx: true, Provider: true},
		},
		{
			"console key only",
			&aktctx.Context{AuthMethod: aktctx.AuthMethodConsoleAPI, ConsoleAPIKey: "sk-x"},
			capability.Set{Console: true},
		},
		{
			"console-preferred context with rpc",
			&aktctx.Context{
				Network:       aktctx.Network{Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc"}}},
				Keyring:       aktctx.Keyring{Name: "default"},
				AuthMethod:    aktctx.AuthMethodConsoleAPI,
				ConsoleAPIKey: "sk-x",
			},
			capability.Set{ChainQuery: true, ChainTx: true, Provider: true, Console: true},
		},
		{
			"rpc without keyring reference",
			&aktctx.Context{
				Network: aktctx.Network{Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc"}}},
			},
			capability.Set{ChainQuery: true, Provider: true},
		},
		{
			"keyring auth with console credential",
			&aktctx.Context{
				Network:       aktctx.Network{Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc"}}},
				Keyring:       aktctx.Keyring{Name: "default"},
				AuthMethod:    aktctx.AuthMethodKeyring,
				ConsoleAPIKey: "sk-x",
			},
			capability.Set{ChainQuery: true, ChainTx: true, Provider: true, Console: true},
		},
	}

	for _, c := range cases {
		if got := capability.Resolve(c.rc); got != c.want {
			t.Errorf("%s: Resolve = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestSatisfies(t *testing.T) {
	consoleOnly := capability.Set{Console: true}

	cases := []struct {
		requirement string
		want        bool
	}{
		{"", true},
		{"console", true},
		{"chain-query", false},
		{"chain-tx|console", true},
		{"chain-tx | console", true},
		{"chain-tx", false},
		{"definitely-not-a-capability", true}, // unknown fails open
	}

	for _, c := range cases {
		if got := consoleOnly.Satisfies(c.requirement); got != c.want {
			t.Errorf("Satisfies(%q) = %v, want %v", c.requirement, got, c.want)
		}
	}
}

func TestExplainNamesRemedies(t *testing.T) {
	s := capability.Set{}

	msg := s.Explain("chain-query")
	if !strings.Contains(msg, "RPC endpoint") || !strings.Contains(msg, "akt context network edit") {
		t.Errorf("chain-query explanation missing remedy: %q", msg)
	}

	msg = s.Explain("chain-tx|console")
	if !strings.Contains(msg, "keyring") || !strings.Contains(msg, "akt console login") || !strings.Contains(msg, ", or ") {
		t.Errorf("alternative explanation wrong: %q", msg)
	}
}

func TestParseMode(t *testing.T) {
	cases := map[string]capability.Mode{
		"dim":     capability.ModeDim,
		"HIDE":    capability.ModeHide,
		"off":     capability.ModeOff,
		"":        capability.ModeDim,
		"garbage": capability.ModeDim,
	}

	for in, want := range cases {
		if got := capability.ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %s, want %s", in, got, want)
		}
	}
}
