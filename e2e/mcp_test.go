package e2e

// Offline e2e for the MCP server: drives the real stdio transport with
// JSON-RPC and asserts over what a client would actually receive. The unit
// tests in internal/mcp pin the annotation policy; these prove the policy is
// wired to the tools that ship.

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// mcpToolsList runs `akt mcp` with the given extra args, performs the
// initialize/tools list handshake over stdio, and returns the tool array.
func mcpToolsList(t *testing.T, home string, extra ...string) []map[string]any {
	t.Helper()

	args := append([]string{"--home", home, "mcp"}, extra...)
	cmd := exec.Command(aktBinary(t), args...)

	cmd.Stdin = strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n") + "\n")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start akt mcp: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("akt mcp did not exit; stderr:\n%s", stderr.String())
	}

	// The server answers one JSON-RPC object per line; the tools list is the
	// reply to id 2.
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		var resp struct {
			ID     int `json:"id"`
			Result struct {
				Tools []map[string]any `json:"tools"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if resp.ID == 2 {
			return resp.Result.Tools
		}
	}

	t.Fatalf("no tools/list response\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())

	return nil
}

func annotation(tool map[string]any, key string) (bool, bool) {
	ann, ok := tool["annotations"].(map[string]any)
	if !ok {
		return false, false
	}

	v, ok := ann[key].(bool)

	return v, ok
}

// TestMCPQueryToolsAreAnnotatedReadOnly is the regression this guards: every
// tool once shipped with no annotations at all, so MCP's defaults applied and
// a client saw 19 read-only queries advertised as destructive.
func TestMCPQueryToolsAreAnnotatedReadOnly(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "network", "create", "net", "--chain-id", "akashnet-2", "--rpc", unreachableNode)
	mustRunAkt(t, home, "context", "create", "ctx", "--network", "net", "--set-current")

	tools := mcpToolsList(t, home)
	if len(tools) == 0 {
		t.Fatal("expected the read-only tool set to be registered")
	}

	for _, tool := range tools {
		name, _ := tool["name"].(string)

		ro, ok := annotation(tool, "readOnlyHint")
		if !ok {
			t.Errorf("%s: readOnlyHint absent; MCP would default it to false", name)
			continue
		}
		if !ro {
			t.Errorf("%s: registered without --enable-writes, so it must be readOnlyHint=true", name)
		}

		if d, ok := annotation(tool, "destructiveHint"); ok && d {
			t.Errorf("%s: a query must not be destructiveHint=true", name)
		}
	}
}

// TestMCPWriteToolsRequireOptIn covers both halves of the --enable-writes
// contract: the mutating tools are absent by default, and when enabled they
// are the only ones that are not read-only.
//
// It also guards a bug where --enable-writes could not start at all: the
// command installs no tx flags, so the sign mode was empty and building the
// write client failed with `invalid sign mode ""`.
func TestMCPWriteToolsRequireOptIn(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "network", "create", "net", "--chain-id", "akashnet-2", "--rpc", unreachableNode)
	mustRunAkt(t, home, "context", "create", "ctx", "--network", "net", "--set-current")

	readOnly := mcpToolsList(t, home)
	for _, tool := range readOnly {
		if ro, ok := annotation(tool, "readOnlyHint"); ok && !ro {
			name, _ := tool["name"].(string)
			t.Errorf("%s: a mutating tool must not be registered without --enable-writes", name)
		}
	}

	withWrites := mcpToolsList(t, home, "--enable-writes")
	if len(withWrites) <= len(readOnly) {
		t.Fatalf("--enable-writes registered no additional tools (%d vs %d)", len(withWrites), len(readOnly))
	}

	writes := 0
	for _, tool := range withWrites {
		ro, ok := annotation(tool, "readOnlyHint")
		if !ok {
			continue
		}
		if !ro {
			writes++
			name, _ := tool["name"].(string)
			if d, ok := annotation(tool, "destructiveHint"); !ok || !d {
				t.Errorf("%s: a mutating tool must be destructiveHint=true", name)
			}
		}
	}

	if writes == 0 {
		t.Error("--enable-writes registered no tools marked as mutating")
	}
}
