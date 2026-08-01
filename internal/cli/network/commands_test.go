package network

import (
	"bytes"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

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
