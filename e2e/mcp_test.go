package e2e

// Offline e2e for the MCP server: drives the real stdio transport with
// JSON-RPC and asserts over what a client would actually receive. The unit
// tests in internal/mcp pin the annotation policy; these prove the policy is
// wired to the tools that ship.

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"sort"
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

func TestMCPToolInventory(t *testing.T) {
	// Tool registration only needs the presence of a key; tools/list does not
	// call the Console API. This keeps the full two-rail inventory hermetic.
	t.Setenv("AKT_CONSOLE_API_KEY", "test-key")

	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "network", "create", "net", "--chain-id", "akashnet-2", "--rpc", unreachableNode)
	mustRunAkt(t, home, "context", "create", "ctx", "--network", "net", "--set-current")

	readNames := []string{
		"akash_account_balance",
		"akash_block_height",
		"akash_get_bid",
		"akash_get_deployment",
		"akash_get_group",
		"akash_get_lease",
		"akash_get_order",
		"akash_get_provider",
		"akash_lease_status",
		"akash_list_audited_providers",
		"akash_list_bids",
		"akash_list_certificates",
		"akash_list_deployments",
		"akash_list_leases",
		"akash_list_orders",
		"akash_list_providers",
		"akash_node_status",
		"akash_provider_status",
		"akash_service_status",
		"console_get_deployment",
		"console_get_provider",
		"console_gpu_prices",
		"console_list_bids",
		"console_list_deployments",
		"console_list_providers",
		"console_usage_history",
		"console_wallet_balance",
	}
	writes := []string{
		"akash_close_deployment",
		"akash_close_lease",
		"akash_create_lease",
		"akash_submit_manifest",
		"console_close_deployment",
		"console_deposit",
	}

	assertToolNames(t, mcpToolsList(t, home), readNames)
	assertToolNames(t, mcpToolsList(t, home, "--enable-writes"), append(readNames, writes...))
}

func assertToolNames(t *testing.T, tools []map[string]any, want []string) {
	t.Helper()

	want = append([]string(nil), want...)
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		got = append(got, name)
	}
	sort.Strings(got)
	sort.Strings(want)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("tool inventory mismatch\ngot (%d):\n%s\nwant (%d):\n%s",
			len(got), strings.Join(got, "\n"), len(want), strings.Join(want, "\n"))
	}
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

// mcpCallTool performs the initialize handshake and one tools/call, returning
// the call's result plus whatever the server wrote to stderr. Both matter: a
// handler that dereferences a missing dependency takes the process down, and
// that only shows up on stderr.
func mcpCallTool(t *testing.T, home, tool string) (result map[string]any, stderr string) {
	t.Helper()

	call, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("marshal call: %v", err)
	}

	cmd := exec.Command(aktBinary(t), "--home", home, "mcp")
	cmd.Stdin = strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}}`,
		string(call),
	}, "\n") + "\n")

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb

	if err := cmd.Start(); err != nil {
		t.Fatalf("start akt mcp: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("akt mcp did not exit; stderr:\n%s", errb.String())
	}

	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var resp struct {
			ID     int            `json:"id"`
			Result map[string]any `json:"result"`
		}
		if json.Unmarshal([]byte(line), &resp) == nil && resp.ID == 2 {
			return resp.Result, errb.String()
		}
	}

	return nil, errb.String()
}

// TestMCPChainToolReachesTheNode calls a chain-backed tool, which is the only
// way to catch a server that lists its tools correctly and then cannot run
// them.
//
// The command builds its client context from the cobra command, which carries
// a node URI but no RPC client — the tx and query trees each construct one in
// their own PersistentPreRunE, and mcp has neither. Every chain tool therefore
// failed: the query tools reported being offline, and the node tools
// dereferenced the missing client and killed the process. Listing the tools
// and calling a Console tool both pass in that state, which is how it shipped.
//
// The node here is unreachable on purpose, so the call must fail — but it has
// to fail as a transport error, having reached the point of dialling.
func TestMCPChainToolReachesTheNode(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "network", "create", "net", "--chain-id", "akashnet-2", "--rpc", unreachableNode)
	mustRunAkt(t, home, "context", "create", "ctx", "--network", "net", "--set-current")

	// A node tool: these are the ones that panicked rather than erroring.
	result, stderr := mcpCallTool(t, home, "akash_block_height")

	if strings.Contains(stderr, "panic:") {
		t.Fatalf("calling a chain tool panicked:\n%s", stderr)
	}

	if result == nil {
		t.Fatalf("no response to tools/call; the server likely died\nstderr:\n%s", stderr)
	}

	text := ""
	if content, ok := result["content"].([]any); ok && len(content) > 0 {
		if first, ok := content[0].(map[string]any); ok {
			text, _ = first["text"].(string)
		}
	}

	// "offline mode" is what the client reports when it was handed no RPC
	// client at all -- the bug -- as distinct from failing to reach the node.
	if strings.Contains(text, "no RPC client") || strings.Contains(text, "offline mode") {
		t.Fatalf("the chain tool was given no RPC client: %s", text)
	}

	if !strings.Contains(text, "connection refused") && !strings.Contains(text, "dial") {
		t.Fatalf("expected a transport error from the unreachable node, got: %s", text)
	}
}
