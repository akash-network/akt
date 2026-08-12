package e2e

// Offline e2e for the MCP server: drives the real stdio transport with
// JSON-RPC and asserts over what a client would actually receive. The unit
// tests in internal/mcp pin the annotation policy; these prove the policy is
// wired to the tools that ship.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/actionlog"
)

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *mcpRPCError    `json:"error"`
}

type mcpConversation struct {
	responses []mcpRPCResponse
	stderr    string
	exitCode  int
}

func runMCPConversation(t *testing.T, home string, messages []string, extra ...string) mcpConversation {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := append([]string{"--home", home, "mcp"}, extra...)
	cmd := exec.CommandContext(ctx, aktBinary(t), args...)
	if len(messages) > 0 {
		cmd.Stdin = strings.NewReader(strings.Join(messages, "\n") + "\n")
	} else {
		cmd.Stdin = strings.NewReader("")
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("akt mcp timed out; stderr:\n%s", stderr.String())
	}

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run akt mcp: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}

	var responses []mcpRPCResponse
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var response mcpRPCResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("MCP stdout contained non-JSON protocol data: %v\nline: %s\nstderr:\n%s", err, line, stderr.String())
		}
		responses = append(responses, response)
	}

	return mcpConversation{responses: responses, stderr: stderr.String(), exitCode: exitCode}
}

func (conversation mcpConversation) response(t *testing.T, id int) mcpRPCResponse {
	t.Helper()
	for _, response := range conversation.responses {
		var got int
		if json.Unmarshal(response.ID, &got) == nil && got == id {
			return response
		}
	}
	t.Fatalf("no MCP response for id %d; responses: %#v\nstderr:\n%s", id, conversation.responses, conversation.stderr)
	return mcpRPCResponse{}
}

func initializeMessage(t *testing.T, id int) string {
	t.Helper()
	request, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "e2e", "version": "1"},
		},
	})
	if err != nil {
		t.Fatalf("marshal initialize request: %v", err)
	}
	return string(request)
}

func toolCallMessage(t *testing.T, id int, tool string, arguments map[string]any) string {
	t.Helper()
	request, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": arguments},
	})
	if err != nil {
		t.Fatalf("marshal %s call: %v", tool, err)
	}
	return string(request)
}

// mcpToolsList runs `akt mcp` with the given extra args, performs the
// initialize/tools list handshake over stdio, and returns the tool array.
func mcpToolsList(t *testing.T, home string, extra ...string) []map[string]any {
	t.Helper()

	conversation := runMCPConversation(t, home, []string{
		initializeMessage(t, 1),
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, extra...)
	if conversation.exitCode != 0 {
		t.Fatalf("akt mcp exited %d; stderr:\n%s", conversation.exitCode, conversation.stderr)
	}
	response := conversation.response(t, 2)
	if response.Error != nil {
		t.Fatalf("tools/list error = %d %s", response.Error.Code, response.Error.Message)
	}
	var result struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	return result.Tools
}

func TestMCPInitializationAndProtocolErrorsAreStructured(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "network", "create", "net", "--chain-id", "akashnet-2", "--rpc", unreachableNode)
	mustRunAkt(t, home, "context", "create", "ctx", "--network", "net", "--set-current")

	conversation := runMCPConversation(t, home, []string{
		"not-json",
		`{"jsonrpc":"2.0","id":4,"method":"initialize","params":[]}`,
		initializeMessage(t, 1),
		`{"jsonrpc":"2.0","id":2,"method":"does/not/exist"}`,
		toolCallMessage(t, 3, "tool_that_does_not_exist", map[string]any{}),
	})
	if conversation.exitCode != 0 {
		t.Fatalf("akt mcp exited %d; stderr:\n%s", conversation.exitCode, conversation.stderr)
	}

	var parseError *mcpRPCError
	for _, response := range conversation.responses {
		if string(response.ID) == "null" {
			parseError = response.Error
			break
		}
	}
	if parseError == nil || parseError.Code != -32700 || parseError.Message != "Parse error" {
		t.Fatalf("parse error = %#v, want -32700 Parse error", parseError)
	}

	badInitialize := conversation.response(t, 4)
	if badInitialize.Error == nil || badInitialize.Error.Code != -32600 {
		t.Fatalf("malformed initialize error = %#v, want -32600", badInitialize.Error)
	}

	initialize := conversation.response(t, 1)
	if initialize.Error != nil {
		t.Fatalf("initialize error = %#v", initialize.Error)
	}
	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
		Capabilities struct {
			Tools map[string]any `json:"tools"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(initialize.Result, &initResult); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if initResult.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocol version = %q, want negotiated 2024-11-05", initResult.ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "akash-mcp" || initResult.ServerInfo.Version != "0.1.0" {
		t.Errorf("server info = %#v", initResult.ServerInfo)
	}
	if initResult.Capabilities.Tools == nil {
		t.Error("initialize result did not advertise tools capability")
	}

	unknownMethod := conversation.response(t, 2)
	if unknownMethod.Error == nil || unknownMethod.Error.Code != -32601 {
		t.Fatalf("unknown method error = %#v, want -32601", unknownMethod.Error)
	}
	unknownTool := conversation.response(t, 3)
	if unknownTool.Error == nil || unknownTool.Error.Code != -32602 ||
		!strings.Contains(unknownTool.Error.Message, "tool_that_does_not_exist") {
		t.Fatalf("unknown tool error = %#v, want named -32602 error", unknownTool.Error)
	}
}

func TestMCPExitsCleanlyOnEOF(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "create", "api",
		"--auth-method", "console-api",
		"--console-api-key", "test-key",
		"--set-current")

	conversation := runMCPConversation(t, home, nil)
	if conversation.exitCode != 0 {
		t.Fatalf("EOF exit code = %d; stderr:\n%s", conversation.exitCode, conversation.stderr)
	}
	if len(conversation.responses) != 0 {
		t.Fatalf("EOF produced protocol responses: %#v", conversation.responses)
	}
	if !strings.Contains(conversation.stderr, "starting stdio server") {
		t.Fatalf("startup banner missing from stderr: %s", conversation.stderr)
	}
}

func TestMCPStopsCleanlyOnInterrupt(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "create", "api",
		"--auth-method", "console-api",
		"--console-api-key", "test-key",
		"--set-current")

	cmd := exec.Command(aktBinary(t), "--home", home, "mcp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("create MCP stdin: %v", err)
	}
	defer stdin.Close()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("create MCP stderr: %v", err)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Start(); err != nil {
		t.Fatalf("start MCP server: %v", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	banner := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		if scanner.Scan() {
			banner <- scanner.Text()
			return
		}
		banner <- ""
	}()

	select {
	case line := <-banner:
		if !strings.Contains(line, "starting stdio server") {
			_ = cmd.Process.Kill()
			t.Fatalf("unexpected startup stderr: %q", line)
		}
	case err := <-wait:
		t.Fatalf("MCP exited before interrupt: %v", err)
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("MCP did not start within 10 seconds")
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("interrupt MCP server: %v", err)
	}

	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("MCP interrupt exit: %v\nstdout:\n%s", err, stdout.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("MCP did not stop within 10 seconds of interrupt")
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("interrupt wrote non-protocol data to stdout: %s", stdout.String())
	}
}

func TestMCPRequiresAtLeastOneUsableRail(t *testing.T) {
	t.Setenv("AKT_CONSOLE_API_KEY", "")
	home := t.TempDir()
	initHome(t, home)

	conversation := runMCPConversation(t, home, nil)
	if conversation.exitCode == 0 {
		t.Fatal("MCP started without a chain context or Console credential")
	}
	if !strings.Contains(conversation.stderr, "context") && !strings.Contains(conversation.stderr, "no tools available") {
		t.Fatalf("unhelpful capability failure:\n%s", conversation.stderr)
	}
}

func TestMCPStartsConsoleOnlyFromEnvironmentWithoutConfig(t *testing.T) {
	t.Setenv("AKT_CONSOLE_API_KEY", "test-key")
	home := t.TempDir()

	conversation := runMCPConversation(t, home, []string{
		initializeMessage(t, 1),
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	})
	if conversation.exitCode != 0 {
		t.Fatalf("Console-only MCP exited %d; stderr:\n%s", conversation.exitCode, conversation.stderr)
	}
	response := conversation.response(t, 2)
	if response.Error != nil {
		t.Fatalf("tools/list error = %d %s", response.Error.Code, response.Error.Message)
	}
	var result struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	assertToolNames(t, result.Tools, []string{
		"console_get_deployment",
		"console_get_provider",
		"console_gpu_prices",
		"console_list_bids",
		"console_list_deployments",
		"console_list_providers",
		"console_usage_history",
		"console_wallet_balance",
	})
	for _, tool := range result.Tools {
		name, _ := tool["name"].(string)
		if strings.HasPrefix(name, "akash_") {
			t.Fatalf("Console-only MCP registered chain tool %q", name)
		}
	}
	if strings.Contains(conversation.stderr, "first-run wizard") ||
		strings.Contains(conversation.stderr, "no akt configuration found") {
		t.Fatalf("MCP invoked interactive bootstrap:\n%s", conversation.stderr)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("MCP created configuration in an empty home: %v", err)
	}
}

func TestMCPContextlessWritesFailBeforeStartup(t *testing.T) {
	t.Setenv("AKT_CONSOLE_API_KEY", "test-key")
	conversation := runMCPConversation(t, t.TempDir(), nil, "--enable-writes")
	if conversation.exitCode == 0 {
		t.Fatal("contextless MCP writes started without an action-log destination")
	}
	if !strings.Contains(conversation.stderr, "per-context action log") {
		t.Fatalf("contextless write error omitted audit remedy:\n%s", conversation.stderr)
	}
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

	readTools := mcpToolsList(t, home)
	assertToolNames(t, readTools, readNames)
	assertToolMetadata(t, readTools, nil)

	allTools := mcpToolsList(t, home, "--enable-writes")
	assertToolNames(t, allTools, append(readNames, writes...))
	writeSet := make(map[string]bool, len(writes))
	for _, name := range writes {
		writeSet[name] = true
	}
	assertToolMetadata(t, allTools, writeSet)

	// A Console-auth context can still have a chain RPC for public reads, but
	// it intentionally owns no local wallet. --enable-writes must therefore
	// expose Console mutations without advertising unusable chain/provider
	// mutations or dropping the healthy chain read rail.
	consoleHome := t.TempDir()
	initHome(t, consoleHome)
	mustRunAkt(t, consoleHome, "context", "network", "create", "net", "--chain-id", "akashnet-2", "--rpc", unreachableNode)
	mustRunAkt(t, consoleHome, "context", "create", "console",
		"--network", "net",
		"--auth-method", "console-api",
		"--console-api-key", "test-key",
		"--set-current")
	consoleWrites := []string{"console_close_deployment", "console_deposit"}
	consoleAuthTools := mcpToolsList(t, consoleHome, "--enable-writes")
	assertToolNames(t, consoleAuthTools, append(append([]string(nil), readNames...), consoleWrites...))
	consoleWriteSet := make(map[string]bool, len(consoleWrites))
	for _, name := range consoleWrites {
		consoleWriteSet[name] = true
	}
	assertToolMetadata(t, consoleAuthTools, consoleWriteSet)
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

func assertToolMetadata(t *testing.T, tools []map[string]any, writes map[string]bool) {
	t.Helper()

	for _, tool := range tools {
		name, ok := tool["name"].(string)
		if !ok || name == "" {
			t.Errorf("tool has no name: %#v", tool)
			continue
		}
		if description, ok := tool["description"].(string); !ok || strings.TrimSpace(description) == "" {
			t.Errorf("%s: description = %#v", name, tool["description"])
		}

		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			t.Errorf("%s: inputSchema = %#v", name, tool["inputSchema"])
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%s: inputSchema.type = %#v, want object", name, schema["type"])
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s: inputSchema.properties = %#v", name, schema["properties"])
			continue
		}
		if required, ok := schema["required"].([]any); ok {
			for _, raw := range required {
				property, _ := raw.(string)
				if _, exists := properties[property]; property == "" || !exists {
					t.Errorf("%s: required property %q has no schema", name, property)
				}
			}
		}

		isWrite := writes[name]
		expected := map[string]bool{
			"readOnlyHint":    !isWrite,
			"destructiveHint": isWrite,
			"idempotentHint":  !isWrite,
			"openWorldHint":   true,
		}
		for key, want := range expected {
			got, present := annotation(tool, key)
			if !present || got != want {
				t.Errorf("%s: annotations.%s = %t (present=%t), want %t", name, key, got, present, want)
			}
		}
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

func TestMCPWriteCallRequiresOptIn(t *testing.T) {
	t.Setenv("AKT_CONSOLE_API_KEY", "test-key")
	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "network", "create", "net", "--chain-id", "akashnet-2", "--rpc", unreachableNode)
	mustRunAkt(t, home, "context", "create", "ctx", "--network", "net", "--set-current")

	arguments := map[string]any{"dseq": "1", "amount_usd": 1}
	withoutWrites := runMCPConversation(t, home, []string{
		initializeMessage(t, 1),
		toolCallMessage(t, 2, "console_deposit", arguments),
	})
	response := withoutWrites.response(t, 2)
	if response.Error == nil || response.Error.Code != -32602 || !strings.Contains(response.Error.Message, "console_deposit") {
		t.Fatalf("write call without opt-in error = %#v, want named -32602", response.Error)
	}

	withWrites := runMCPConversation(t, home, []string{
		initializeMessage(t, 1),
		toolCallMessage(t, 2, "console_deposit", map[string]any{"dseq": "1", "amount_usd": 0.01}),
	}, "--enable-writes")
	result := decodeToolResult(t, withWrites.response(t, 2))
	if !result.isError || !strings.Contains(result.text, "amount_usd must be greater than or equal to 0.5") {
		t.Fatalf("write call with opt-in result = %#v", result)
	}
}

func TestMCPArgumentBoundariesReturnToolErrorsWithoutCallingBackends(t *testing.T) {
	t.Setenv("AKT_CONSOLE_API_KEY", "")

	var backendMu sync.Mutex
	rpcStartupRequests := 0
	rpcRequests := 0
	consoleRequests := 0
	rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendMu.Lock()
		if r.URL.Path == "/websocket" {
			// The write-capable chain client opens its CometBFT WebSocket while
			// the MCP process starts. Count that separately from application
			// RPCs made by tool handlers.
			rpcStartupRequests++
		} else {
			rpcRequests++
		}
		backendMu.Unlock()
		http.Error(w, "counting chain RPC", http.StatusInternalServerError)
	}))
	defer rpcServer.Close()
	consoleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		backendMu.Lock()
		consoleRequests++
		backendMu.Unlock()
		http.Error(w, "an invalid MCP call reached Console", http.StatusInternalServerError)
	}))
	defer consoleServer.Close()

	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "network", "create", "net", "--chain-id", "akashnet-2", "--rpc", rpcServer.URL)
	mustRunAkt(t, home, "context", "create", "chain", "--network", "net", "--set-current")
	mustRunAkt(t, home, "context", "create", "console",
		"--network", "net",
		"--auth-method", "console-api",
		"--console-api-url", consoleServer.URL,
		"--console-api-key", "test-key")

	type boundaryCase struct {
		tool string
		args map[string]any
		want string
	}
	chainCases := []boundaryCase{
		{tool: "akash_list_deployments", args: map[string]any{"state": "pending"}, want: "state must be one of active, closed"},
		{tool: "akash_list_bids", args: map[string]any{"owner": "akash1owner", "dseq": 1.5}, want: "dseq must be a whole number"},
		{tool: "akash_close_deployment", args: map[string]any{}, want: "missing required parameter: dseq"},
		{tool: "akash_submit_manifest", args: map[string]any{"provider": testMnemonicAddr, "dseq": 1, "manifest_json": "{"}, want: "invalid manifest JSON"},
		{tool: "akash_submit_manifest", args: map[string]any{"provider": testMnemonicAddr, "dseq": 1, "manifest_json": "[]"}, want: "manifest is empty"},
		{tool: "akash_lease_status", args: map[string]any{"provider": testMnemonicAddr, "provider_url": "https://credential-capture.invalid", "dseq": 1, "gseq": 1, "oseq": 1}, want: "unknown parameter: provider_url"},
	}
	consoleCases := []boundaryCase{
		{tool: "console_list_deployments", args: map[string]any{"limit": "fifty"}, want: "limit must be a number"},
		{tool: "console_list_providers", args: map[string]any{"scope": "active"}, want: "scope must be one of all, trial"},
		{tool: "console_get_provider", args: map[string]any{}, want: "missing required parameter: address"},
		{tool: "console_deposit", args: map[string]any{"dseq": "1", "amount_usd": 0.01}, want: "amount_usd must be greater than or equal to 0.5"},
	}

	runCases := func(t *testing.T, contextName string, cases []boundaryCase) {
		t.Helper()
		mustRunAkt(t, home, "context", "use", contextName)

		actionLogPath := filepath.Join(home, "contexts", contextName, "actions.log")
		actionLogBefore, err := os.ReadFile(actionLogPath)
		if err != nil {
			t.Fatalf("read %s action log before invalid MCP calls: %v", contextName, err)
		}

		messages := []string{initializeMessage(t, 1)}
		for i, tc := range cases {
			messages = append(messages, toolCallMessage(t, i+2, tc.tool, tc.args))
		}
		conversation := runMCPConversation(t, home, messages, "--enable-writes")
		if conversation.exitCode != 0 {
			t.Fatalf("akt mcp exited %d; stderr:\n%s", conversation.exitCode, conversation.stderr)
		}
		if strings.Contains(conversation.stderr, "panic:") {
			t.Fatalf("invalid tool call panicked:\n%s", conversation.stderr)
		}

		for i, tc := range cases {
			result := decodeToolResult(t, conversation.response(t, i+2))
			if !result.isError || !strings.Contains(result.text, tc.want) {
				t.Errorf("%s result = %#v, want tool error containing %q", tc.tool, result, tc.want)
			}
		}

		actionLogAfter, err := os.ReadFile(actionLogPath)
		if err != nil {
			t.Fatalf("read %s action log after invalid MCP calls: %v", contextName, err)
		}
		if !bytes.Equal(actionLogAfter, actionLogBefore) {
			t.Fatalf("invalid MCP calls appended the %s action log\nbefore:\n%s\nafter:\n%s",
				contextName, actionLogBefore, actionLogAfter)
		}
	}
	t.Run("chain rail", func(t *testing.T) { runCases(t, "chain", chainCases) })
	t.Run("Console rail", func(t *testing.T) { runCases(t, "console", consoleCases) })

	backendMu.Lock()
	gotRPCStartupRequests := rpcStartupRequests
	gotRPCRequests := rpcRequests
	gotConsoleRequests := consoleRequests
	backendMu.Unlock()
	if gotRPCStartupRequests < 1 {
		t.Fatal("keyring MCP did not reach the counting chain endpoint during client startup")
	}
	if gotRPCRequests != 0 || gotConsoleRequests != 0 {
		t.Fatalf("invalid MCP calls made application backend requests: chain RPC=%d Console=%d",
			gotRPCRequests, gotConsoleRequests)
	}
}

type decodedToolResult struct {
	isError bool
	text    string
}

func decodeToolResult(t *testing.T, response mcpRPCResponse) decodedToolResult {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("tools/call JSON-RPC error = %d %s", response.Error.Code, response.Error.Message)
	}
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	var text strings.Builder
	for _, content := range result.Content {
		if content.Type == "text" {
			text.WriteString(content.Text)
		}
	}
	return decodedToolResult{isError: result.IsError, text: text.String()}
}

// mcpCallTool performs the initialize handshake and one tools/call, returning
// the call's result plus whatever the server wrote to stderr. Both matter: a
// handler that dereferences a missing dependency takes the process down, and
// that only shows up on stderr.
func mcpCallTool(t *testing.T, home, tool string) (result map[string]any, stderr string) {
	t.Helper()
	return mcpCallToolWithArgs(t, home, tool, map[string]any{})
}

func mcpCallToolWithArgs(
	t *testing.T,
	home string,
	tool string,
	arguments map[string]any,
	extra ...string,
) (result map[string]any, stderr string) {
	t.Helper()
	conversation := runMCPConversation(t, home, []string{
		initializeMessage(t, 1),
		toolCallMessage(t, 2, tool, arguments),
	}, extra...)
	if conversation.exitCode != 0 {
		t.Fatalf("akt mcp exited %d; stderr:\n%s", conversation.exitCode, conversation.stderr)
	}
	response := conversation.response(t, 2)
	if response.Error != nil {
		return nil, conversation.stderr
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("decode %s result: %v", tool, err)
	}
	return result, conversation.stderr
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

func TestMCPReadOnlyConsoleCallDoesNotAppendActionLog(t *testing.T) {
	requestSeen := make(chan struct{}, 1)
	consoleAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/balances" {
			t.Errorf("Console request = %s %s, want GET /v1/balances", r.Method, r.URL.Path)
		}
		if key := r.Header.Get("x-api-key"); key != "test-key" {
			t.Errorf("x-api-key = %q, want configured credential", key)
		}
		requestSeen <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"balance":1500000,"deployments":2500000,"total":4000000}}`))
	}))
	defer consoleAPI.Close()

	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "create", "api",
		"--auth-method", "console-api",
		"--console-api-url", consoleAPI.URL,
		"--console-api-key", "test-key",
		"--set-current")

	actionLogPath := filepath.Join(home, "contexts", "api", "actions.log")
	before, err := os.ReadFile(actionLogPath)
	if err != nil {
		t.Fatalf("read action log before MCP query: %v", err)
	}

	conversation := runMCPConversation(t, home, []string{
		initializeMessage(t, 1),
		toolCallMessage(t, 2, "console_wallet_balance", map[string]any{}),
	})
	if conversation.exitCode != 0 {
		t.Fatalf("akt mcp exited %d; stderr:\n%s", conversation.exitCode, conversation.stderr)
	}
	result := decodeToolResult(t, conversation.response(t, 2))
	if result.isError {
		t.Fatalf("wallet balance returned an MCP error: %s", result.text)
	}
	var balance map[string]float64
	if err := json.Unmarshal([]byte(result.text), &balance); err != nil {
		t.Fatalf("decode wallet balance: %v\n%s", err, result.text)
	}
	want := map[string]float64{
		"available_usd":      1.5,
		"in_deployments_usd": 2.5,
		"total_usd":          4,
	}
	for field, expected := range want {
		if balance[field] != expected {
			t.Errorf("%s = %v, want %v", field, balance[field], expected)
		}
	}
	select {
	case <-requestSeen:
	default:
		t.Fatal("Console API did not receive the read-only MCP call")
	}

	after, err := os.ReadFile(actionLogPath)
	if err != nil {
		t.Fatalf("read action log after MCP query: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("read-only MCP call appended the action log\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestMCPConsoleMutationsAppendOneActionLogEntryPerAttempt(t *testing.T) {
	requests := make(chan string, 2)
	var stateMu sync.Mutex
	depositApplied := false
	consoleAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/deployments/") {
			dseq := strings.TrimPrefix(r.URL.Path, "/v1/deployments/")
			stateMu.Lock()
			applied := depositApplied
			stateMu.Unlock()
			amount := "1000000"
			if dseq == "41" && applied {
				amount = "6000000"
			}
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"` + dseq + `"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"` + amount + `"}],"transferred":[]}}}}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/deposit-deployment" {
			t.Errorf("unexpected Console request = %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if key := r.Header.Get("x-api-key"); key != "test-key" {
			t.Errorf("x-api-key = %q, want configured credential", key)
		}

		var envelope struct {
			Data struct {
				DSeq    string  `json:"dseq"`
				Deposit float64 `json:"deposit"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Errorf("decode Console deposit: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		requests <- envelope.Data.DSeq
		if envelope.Data.Deposit != 5 {
			t.Errorf("deposit = %v, want 5 USD", envelope.Data.Deposit)
		}

		if envelope.Data.DSeq == "42" {
			http.Error(w, "deployment is settling", http.StatusConflict)
			return
		}
		stateMu.Lock()
		depositApplied = true
		stateMu.Unlock()
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"41"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"6000000"}],"transferred":[]}}}}`))
	}))
	defer consoleAPI.Close()

	home := t.TempDir()
	initHome(t, home)
	mustRunAkt(t, home, "context", "create", "api",
		"--auth-method", "console-api",
		"--console-api-url", consoleAPI.URL,
		"--console-api-key", "test-key",
		"--set-current")

	conversation := runMCPConversation(t, home, []string{
		initializeMessage(t, 1),
		toolCallMessage(t, 2, "console_deposit", map[string]any{"dseq": "41", "amount_usd": 5}),
		toolCallMessage(t, 3, "console_deposit", map[string]any{"dseq": "42", "amount_usd": 5}),
	}, "--enable-writes")
	if conversation.exitCode != 0 {
		t.Fatalf("akt mcp exited %d; stderr:\n%s", conversation.exitCode, conversation.stderr)
	}
	if result := decodeToolResult(t, conversation.response(t, 2)); result.isError {
		t.Fatalf("successful deposit returned an MCP error: %s", result.text)
	}
	if result := decodeToolResult(t, conversation.response(t, 3)); !result.isError || !strings.Contains(result.text, "unexpected status 409") {
		t.Fatalf("failed deposit result = %#v", result)
	}

	seen := map[string]int{}
	for range 2 {
		select {
		case dseq := <-requests:
			seen[dseq]++
		case <-time.After(5 * time.Second):
			t.Fatal("Console API did not receive both MCP mutations")
		}
	}
	if seen["41"] != 1 || seen["42"] != 1 {
		t.Fatalf("Console mutation requests = %v, want one for each dseq", seen)
	}

	logger, err := actionlog.Open(filepath.Join(home, "contexts", "api", "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	entries, err := logger.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if err != nil {
		t.Fatalf("read Console action log: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("two MCP mutations logged %d Console entries, want exactly 2: %+v", len(entries), entries)
	}
	byDSeq := make(map[uint64]actionlog.Entry, len(entries))
	for _, entry := range entries {
		byDSeq[entry.DSeq] = entry
	}
	if got := byDSeq[41]; got.Action != "deposit" || got.Status != "success" || got.Error != "" {
		t.Errorf("successful MCP deposit entry = %+v", got)
	}
	if got := byDSeq[42]; got.Action != "deposit" || got.Status != "failed" || !strings.Contains(got.Error, "409") {
		t.Errorf("failed MCP deposit entry = %+v", got)
	}
}
