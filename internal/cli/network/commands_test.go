package network

import (
	"bytes"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// runList executes `network list` with the given flags and returns stdout.
func runList(t *testing.T, m *aktctx.Manager, args ...string) string {
	t.Helper()
	t.Setenv("NO_COLOR", "")

	cmd := listCmd(func() *aktctx.Manager { return m })
	cmd.Flags().String("output", "pretty", "Output format")

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("network list: %v", err)
	}

	return stdout.String()
}

// List renders through the shared pretty renderer (SPEC §10.8) and prints
// endpoints in full: the hand-built table it used to write truncated RPC URLs
// at 40 characters, which is not a URL anyone can use.
func TestListRendersFullEndpointsOnCommandWriter(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	const rpc = "https://a-very-long-rpc-endpoint.example.com:26657/rpc-path"
	if err := m.CreateNetwork(aktctx.Network{
		Name:      "custom",
		ChainID:   "akashnet-2",
		Endpoints: aktctx.Endpoints{RPC: []string{rpc}},
	}); err != nil {
		t.Fatalf("create network: %v", err)
	}

	out := runList(t, m)
	if !strings.Contains(out, rpc) {
		t.Errorf("network list truncated its RPC endpoint: %q", out)
	}
	if strings.Contains(out, "...") {
		t.Errorf("network list must not elide endpoints: %q", out)
	}
}

// An empty list explains itself on the pretty path but must stay an empty
// array in structured output (SPEC §10.3).
func TestListEmpty(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if out := runList(t, m); !strings.Contains(out, "No networks configured") {
		t.Errorf("pretty output should explain the empty result, got %q", out)
	}

	out := runList(t, m, "--output", "json")
	if strings.Contains(out, "No networks configured") {
		t.Errorf("structured output must not be prose, got %q", out)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("json output = %q, want an empty array", out)
	}
}

func TestShowUsesPlainCommandWriterOutsideTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if m.GetNetwork("mainnet") == nil {
		if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
			t.Fatalf("create mainnet: %v", err)
		}
	}

	cmd := showCmd(func() *aktctx.Manager { return m })
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"mainnet"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("network show: %v", err)
	}
	if !strings.Contains(stdout.String(), "mainnet") {
		t.Fatalf("network show did not use command writer: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("network show emitted ANSI outside a TTY: %q", stdout.String())
	}
}
