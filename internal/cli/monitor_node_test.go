package cli

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestMonitorNodeAliasEndpointSelection(t *testing.T) {
	// Scheme-less endpoints stop at validation before opening the TUI. The
	// error identifies the endpoint actually selected by the public command.
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "node flag",
			args: []string{"--node", "node.example.test"},
			want: "node.example.test",
		},
		{
			name: "node equals form",
			args: []string{"--node=node.example.test"},
			want: "node.example.test",
		},
		{
			name: "rpc overrides node",
			args: []string{"--node", "node.example.test", "--rpc", "rpc.example.test"},
			want: "rpc.example.test",
		},
		{
			name: "rpc wins regardless of flag order",
			args: []string{"--rpc", "rpc.example.test", "--node", "node.example.test"},
			want: "rpc.example.test",
		},
		{
			name: "positional overrides both flags",
			args: []string{"position.example.test", "--node", "node.example.test", "--rpc", "rpc.example.test"},
			want: "position.example.test",
		},
		{
			name: "ignored malformed node does not affect rpc",
			args: []string{"--node", "http://[invalid", "--rpc", "rpc.example.test"},
			want: "rpc.example.test",
		},
	}
	for _, dashboard := range []string{"", "network", "provider", "oracle", "bme"} {
		for _, tc := range cases {
			t.Run(dashboard+"/"+tc.name, func(t *testing.T) {
				home := t.TempDir()
				cmd := NewRootCmd(BuildInfo{Version: "test"})
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				args := append([]string(nil), tc.args...)
				if dashboard != "" {
					args = append([]string{dashboard}, args...)
				}
				cmd.SetArgs(append([]string{"--home", home, "monitor"}, args...))
				err := Execute(cmd)
				want := fmt.Sprintf("RPC endpoint %q must include a scheme and hostname", tc.want)
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("monitor %v: error = %v, want %q", args, err, want)
				}
			})
		}
	}
}
