package cli

import (
	"strings"
	"testing"
)

func TestMonitorExamplesUseWebsocketEndpoint(t *testing.T) {
	t.Parallel()

	root := NewRootCmd(BuildInfo{Version: "test"})
	for _, path := range [][]string{{"monitor"}, {"monitor", "network"}} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("find %q: %v", strings.Join(path, " "), err)
		}
		if !strings.Contains(cmd.Example, "https://rpc.akt.dev:443/rpc") {
			t.Errorf("%s example does not use the verified WebSocket endpoint:\n%s", strings.Join(path, " "), cmd.Example)
		}
	}
}
