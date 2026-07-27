package e2e

// Offline e2e tests: everything in this file runs against the prebuilt akt
// binary without any chain connectivity. Tests that need a running node live
// in localnet_test.go and are gated behind AKT_E2E_RPC / AKT_E2E_LOCALNET.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Deterministic BIP39 mnemonic used for recovery tests. The derived address
// is stable for coin-type 118, account 0, index 0.
const (
	testMnemonic     = "crouch ignore mail actor month page year cushion execute home pluck among bone helmet witness company regret image atom produce marriage surround top exhibit"
	testMnemonicAddr = "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk"
)

// unreachableNode is an endpoint that fails immediately with connection
// refused, so query/tx commands fail fast at the connection stage without
// ever touching a real network.
const unreachableNode = "tcp://127.0.0.1:1"

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes ANSI color/style escape sequences so tests can assert on
// plain text regardless of terminal styling.
func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// runAktStdin runs akt with the given home and arguments, feeding input on
// stdin (for passphrase prompts that read a line from stdin).
func runAktStdin(t *testing.T, home, stdin string, args ...string) (string, string, int) {
	t.Helper()
	bin := aktBinary(t)
	fullArgs := append([]string{"--home", home}, args...)
	cmd := exec.Command(bin, fullArgs...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run akt: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

// mustRunAkt runs akt and fails the test on non-zero exit.
func mustRunAkt(t *testing.T, home string, args ...string) string {
	t.Helper()
	stdout, stderr, exitCode := runAkt(t, home, args...)
	if exitCode != 0 {
		t.Fatalf("akt %s failed (exit %d)\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), exitCode, stdout, stderr)
	}
	return stdout
}

// --- context keys lifecycle (test keyring backend) ---

func TestKeysLifecycle(t *testing.T) {
	home := t.TempDir()
	initHome(t, home) // config keyring "default" uses backend: test — no OS prompts

	// add: generates a new key and prints the mnemonic backup.
	stdout := mustRunAkt(t, home, "context", "keys", "add", "alice")
	if !strings.Contains(stdout, "name: alice") {
		t.Fatalf("expected add output to contain 'name: alice', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "address: akash1") {
		t.Fatalf("expected add output to contain an akash1 address, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "mnemonic") {
		t.Fatalf("expected add output to mention the mnemonic backup, got:\n%s", stdout)
	}

	// add --no-backup: mnemonic must NOT be printed.
	stdout = mustRunAkt(t, home, "context", "keys", "add", "bob", "--no-backup")
	if strings.Contains(stdout, "mnemonic") {
		t.Fatalf("expected --no-backup output to omit the mnemonic, got:\n%s", stdout)
	}

	// list: both keys present.
	stdout = stripANSI(mustRunAkt(t, home, "context", "keys", "list"))
	for _, name := range []string{"alice", "bob"} {
		if !strings.Contains(stdout, name) {
			t.Fatalf("expected keys list to contain %q, got:\n%s", name, stdout)
		}
	}

	// show -a: prints only the bech32 address.
	stdout = mustRunAkt(t, home, "context", "keys", "show", "alice", "-a")
	addr := strings.TrimSpace(stripANSI(stdout))
	if !strings.HasPrefix(addr, "akash1") {
		t.Fatalf("expected show -a to print a bare akash1 address, got: %q", addr)
	}

	// show (full): includes name, type, address, pubkey.
	stdout = stripANSI(mustRunAkt(t, home, "context", "keys", "show", "alice"))
	for _, field := range []string{"Name:", "Type:", "Address:", "PubKey:"} {
		if !strings.Contains(stdout, field) {
			t.Fatalf("expected keys show to contain %q, got:\n%s", field, stdout)
		}
	}

	// show by address: lookup works with the bech32 address too.
	stdout = stripANSI(mustRunAkt(t, home, "context", "keys", "show", addr))
	if !strings.Contains(stdout, "alice") {
		t.Fatalf("expected keys show <address> to resolve to alice, got:\n%s", stdout)
	}

	// rename.
	mustRunAkt(t, home, "context", "keys", "rename", "alice", "alice-main")
	stdout = stripANSI(mustRunAkt(t, home, "context", "keys", "list"))
	if !strings.Contains(stdout, "alice-main") {
		t.Fatalf("expected keys list to contain 'alice-main' after rename, got:\n%s", stdout)
	}

	// export: passphrase is read from stdin (non-TTY friendly).
	stdout, stderr, exitCode := runAktStdin(t, home, "passphrase123\n", "context", "keys", "export", "alice-main")
	if exitCode != 0 {
		t.Fatalf("keys export failed (exit %d)\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	begin := strings.Index(stdout, "-----BEGIN TENDERMINT PRIVATE KEY-----")
	if begin < 0 {
		t.Fatalf("expected export output to contain armored key, got:\n%s", stdout)
	}
	armor := stdout[begin:]

	// import: round-trip the armored key under a new name.
	armorFile := filepath.Join(t.TempDir(), "alice.armor")
	if err := os.WriteFile(armorFile, []byte(armor), 0o600); err != nil {
		t.Fatalf("failed to write armor file: %v", err)
	}
	stdout, stderr, exitCode = runAktStdin(t, home, "passphrase123\n", "context", "keys", "import", "alice-restored", armorFile)
	if exitCode != 0 {
		t.Fatalf("keys import failed (exit %d)\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}

	// The restored key must have the same address as the original.
	restored := strings.TrimSpace(stripANSI(mustRunAkt(t, home, "context", "keys", "show", "alice-restored", "-a")))
	if restored != addr {
		t.Fatalf("expected restored key address %q to equal original %q", restored, addr)
	}

	// delete with --yes (non-interactive).
	mustRunAkt(t, home, "context", "keys", "delete", "bob", "--yes")
	stdout = stripANSI(mustRunAkt(t, home, "context", "keys", "list"))
	if strings.Contains(stdout, "bob") {
		t.Fatalf("expected keys list to NOT contain 'bob' after delete, got:\n%s", stdout)
	}
}

func TestKeysAddRecoverFromSource(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)

	// Recover from a known mnemonic via --source (non-interactive --recover).
	src := filepath.Join(t.TempDir(), "mnemonic.txt")
	if err := os.WriteFile(src, []byte(testMnemonic+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write mnemonic file: %v", err)
	}

	stdout := mustRunAkt(t, home, "context", "keys", "add", "carol", "--source", src)
	if !strings.Contains(stdout, testMnemonicAddr) {
		t.Fatalf("expected deterministic address %s for recovered key, got:\n%s", testMnemonicAddr, stdout)
	}
	// Recovery via --source must not echo the mnemonic back.
	if strings.Contains(stdout, "crouch ignore mail") {
		t.Fatalf("expected recovery output to omit the mnemonic, got:\n%s", stdout)
	}
}

func TestKeysMnemonic(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)

	stdout := mustRunAkt(t, home, "context", "keys", "mnemonic")
	words := strings.Fields(strings.TrimSpace(stripANSI(stdout)))
	if len(words) != 24 {
		t.Fatalf("expected a 24-word mnemonic, got %d words:\n%s", len(words), stdout)
	}
}

func TestKeysParse(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)

	// bech32 -> hex (+ re-encodings under common prefixes).
	stdout := stripANSI(mustRunAkt(t, home, "context", "keys", "parse", testMnemonicAddr))
	if !strings.Contains(stdout, "Format:  bech32") {
		t.Fatalf("expected parse output to identify bech32 format, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "HRP:     akash") {
		t.Fatalf("expected parse output to contain HRP akash, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "cosmos1") {
		t.Fatalf("expected parse output to re-encode under cosmos prefix, got:\n%s", stdout)
	}

	// Garbage input fails.
	_, stderr, exitCode := runAkt(t, home, "context", "keys", "parse", "zzz-not-an-address")
	if exitCode == 0 {
		t.Fatal("expected non-zero exit for unparseable input")
	}
	if !strings.Contains(stderr, "cannot parse") {
		t.Fatalf("expected parse error message, got:\n%s", stderr)
	}
}

// --- context edit / log / rename / network edit + delete ---

func TestContextEditDefaultAccount(t *testing.T) {
	home := setupContextHome(t)

	mustRunAkt(t, home, "context", "edit", "prod", "--default-account", "carol")

	stdout := stripANSI(mustRunAkt(t, home, "context", "show"))
	if !strings.Contains(stdout, "Default Account") || !strings.Contains(stdout, "carol") {
		t.Fatalf("expected context show to display default account 'carol', got:\n%s", stdout)
	}
}

func TestContextLogRecordsContextActions(t *testing.T) {
	home := setupContextHome(t) // create (+ switch via --set-current)

	// Another recorded action so ordering is observable.
	mustRunAkt(t, home, "context", "edit", "prod", "--default-account", "dave")

	stdout := stripANSI(mustRunAkt(t, home, "context", "log"))

	if !strings.Contains(stdout, "context") {
		t.Fatalf("expected action log to contain 'context' type rows, got:\n%s", stdout)
	}
	for _, action := range []string{"create", "switch", "edit"} {
		if !strings.Contains(stdout, action) {
			t.Fatalf("expected action log to contain %q entry, got:\n%s", action, stdout)
		}
	}

	// Newest-first ordering: the edit (last action) must appear before the
	// create/switch entries that happened earlier.
	editIdx := strings.Index(stdout, "edit")
	createIdx := strings.Index(stdout, "create")
	switchIdx := strings.Index(stdout, "switch")
	if editIdx > createIdx || editIdx > switchIdx {
		t.Fatalf("expected newest-first ordering (edit before switch/create), got:\n%s", stdout)
	}
}

func TestContextRenameRecordsAction(t *testing.T) {
	home := setupContextHome(t)

	mustRunAkt(t, home, "context", "rename", "prod", "production")

	// The context dir (including the action log) moves with the rename and
	// current-context follows it.
	stdout := stripANSI(mustRunAkt(t, home, "context", "log"))
	if !strings.Contains(stdout, "rename") {
		t.Fatalf("expected action log to contain the rename entry, got:\n%s", stdout)
	}

	stdout = stripANSI(mustRunAkt(t, home, "context", "show"))
	if !strings.Contains(stdout, "production") {
		t.Fatalf("expected context show to display renamed context, got:\n%s", stdout)
	}
}

func TestContextNetworkEditAndDelete(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)

	// Custom network (no template).
	mustRunAkt(t, home, "context", "network", "create", "local",
		"--chain-id", "localnet-1", "--rpc", "http://127.0.0.1:26657")

	// edit: change gas prices and chain-id.
	mustRunAkt(t, home, "context", "network", "edit", "local",
		"--gas-prices", "0.05uakt", "--chain-id", "localnet-2")

	stdout := stripANSI(mustRunAkt(t, home, "context", "network", "show", "local"))
	if !strings.Contains(stdout, "0.05uakt") {
		t.Fatalf("expected network show to reflect edited gas prices, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "localnet-2") {
		t.Fatalf("expected network show to reflect edited chain-id, got:\n%s", stdout)
	}

	// delete fails while a context references the network.
	mustRunAkt(t, home, "context", "create", "dev", "--network", "local")
	_, stderr, exitCode := runAkt(t, home, "context", "network", "delete", "local")
	if exitCode == 0 {
		t.Fatalf("expected network delete to fail while referenced by a context, stderr:\n%s", stderr)
	}

	// After removing the referencing context, delete succeeds.
	mustRunAkt(t, home, "context", "delete", "dev", "--yes")
	mustRunAkt(t, home, "context", "network", "delete", "local")

	stdout = stripANSI(mustRunAkt(t, home, "context", "network", "list"))
	if strings.Contains(stdout, "local") {
		t.Fatalf("expected network list to NOT contain 'local' after delete, got:\n%s", stdout)
	}
}

// --- console-api auth method (SPEC §7.1) ---

func TestContextConsoleAPIAuth(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)

	const apiKey = "test-key-secret-123"

	mustRunAkt(t, home, "context", "network", "create", "mainnet", "--template", "mainnet")
	mustRunAkt(t, home, "context", "create", "api",
		"--network", "mainnet",
		"--auth-method", "console-api",
		"--console-api-key", apiKey,
		"--set-current")

	stdout := stripANSI(mustRunAkt(t, home, "context", "show"))

	if !strings.Contains(stdout, "console-api") {
		t.Fatalf("expected context show to display auth method console-api, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "API Key") || !strings.Contains(stdout, "configured") {
		t.Fatalf("expected context show to report the API key as configured, got:\n%s", stdout)
	}
	// The credential itself must never be printed (SPEC §7.1).
	if strings.Contains(stdout, apiKey) {
		t.Fatalf("context show leaked the console API key:\n%s", stdout)
	}

	// Credential file exists at contexts/<name>/console-api-key with 0600 perms.
	credPath := filepath.Join(home, "contexts", "api", "console-api-key")
	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("expected credential file at %s: %v", credPath, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected credential file perms 0600, got %o", perm)
	}
	data, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("failed to read credential file: %v", err)
	}
	if strings.TrimSpace(string(data)) != apiKey {
		t.Fatalf("credential file content mismatch: got %q", string(data))
	}

	// The key must not land in config.yaml.
	cfg, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("failed to read config.yaml: %v", err)
	}
	if strings.Contains(string(cfg), apiKey) {
		t.Fatal("console API key leaked into config.yaml")
	}
}

// --- positional args smoke tests (AKT-650) ---
//
// Query/tx commands establish the RPC connection in PersistentPreRunE, which
// runs AFTER cobra argument validation but BEFORE the in-RunE filter parsing
// (DepFiltersFromArg). Offline we can therefore prove two things:
//   1. cobra-level Args accept the positional form (a rejection would surface
//      as an args error before any connection attempt), and
//   2. the failure is at the connection stage, NOT the old
//      "not a valid address or dseq number" arg-parse rejection.
// Full end-to-end verification that the state keyword filters results
// requires a live node — covered by the gated localnet suite and unit tests
// in internal/cli/chain/flags.

func TestQueryDeploymentPositionalArgsOffline(t *testing.T) {
	home := setupContextHome(t)

	cases := []struct {
		name string
		args []string
	}{
		{"state keyword", []string{"query", "deployment", "active", "--node", unreachableNode}},
		{"owner address", []string{"query", "deployment", testMnemonicAddr, "--node", unreachableNode}},
		{"owner slash dseq", []string{"query", "deployment", testMnemonicAddr + "/12345", "--node", unreachableNode}},
		// Second positional: a state keyword combined with the identity
		// filter (2026-07 positional-only trial; --state is disabled).
		{"identity plus state", []string{"query", "deployment", testMnemonicAddr + "/12345", "active", "--node", unreachableNode}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runAkt(t, home, tc.args...)
			combined := stdout + stderr

			if exitCode == 0 {
				t.Fatalf("expected non-zero exit against unreachable node, got 0:\n%s", combined)
			}
			if strings.Contains(combined, "not a valid address or dseq number") {
				t.Fatalf("positional arg was rejected at parse stage (AKT-650 regression):\n%s", combined)
			}
			if strings.Contains(combined, "unknown command") || strings.Contains(combined, "accepts at most") {
				t.Fatalf("positional arg was rejected by cobra args validation:\n%s", combined)
			}
			// Failure must be at the connection stage.
			if !strings.Contains(combined, "127.0.0.1") {
				t.Fatalf("expected a connection-stage error mentioning the node address, got:\n%s", combined)
			}
		})
	}
}

func TestQueryMarketPositionalStateArgsOffline(t *testing.T) {
	// The market order/bid/lease twins accept the same optional second
	// positional [state]; offline we prove cobra and the parse stage accept
	// the identity+state form and the failure is at the connection stage.
	home := setupContextHome(t)

	cases := []struct {
		name string
		args []string
	}{
		{"order identity plus state", []string{"query", "market", "order", testMnemonicAddr + "/12345/1/1", "open", "--node", unreachableNode}},
		{"bid identity plus state", []string{"query", "market", "bid", testMnemonicAddr + "/12345/1/1", "open", "--node", unreachableNode}},
		{"lease identity plus state", []string{"query", "market", "lease", testMnemonicAddr + "/12345/1/1", "active", "--node", unreachableNode}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runAkt(t, home, tc.args...)
			combined := stdout + stderr

			if exitCode == 0 {
				t.Fatalf("expected non-zero exit against unreachable node, got 0:\n%s", combined)
			}
			if strings.Contains(combined, "not a valid") {
				t.Fatalf("positional arg was rejected at parse stage:\n%s", combined)
			}
			if strings.Contains(combined, "unknown command") || strings.Contains(combined, "accepts at most") {
				t.Fatalf("positional arg was rejected by cobra args validation:\n%s", combined)
			}
			if !strings.Contains(combined, "127.0.0.1") {
				t.Fatalf("expected a connection-stage error mentioning the node address, got:\n%s", combined)
			}
		})
	}
}

func TestTxDeploymentClosePositionalArgsOffline(t *testing.T) {
	// tx deployment close <dseq> requires a node for account/sequence
	// resolution even with --generate-only, so offline we can only assert
	// the positional dseq passes cobra validation and the failure is at the
	// connection stage. The full path is exercised on the localnet.
	home := setupContextHome(t)

	stdout, stderr, exitCode := runAkt(t, home,
		"tx", "deployment", "close", "12345", "--generate-only", "--node", unreachableNode)
	combined := stdout + stderr

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit against unreachable node, got 0:\n%s", combined)
	}
	if strings.Contains(combined, "not a valid") || strings.Contains(combined, "unknown command") {
		t.Fatalf("positional dseq was rejected before connection stage:\n%s", combined)
	}
	if !strings.Contains(combined, "127.0.0.1") {
		t.Fatalf("expected a connection-stage error, got:\n%s", combined)
	}
}

// --- dseq fail-fast guards (2026-07 positional-only trial) ---
//
// With --dseq disabled, the positional dseq is the only source for
// `tx deployment close` and `tx deployment update`. A missing dseq must fail
// during cobra argument validation — before the tx pipeline (connection,
// keyring unlock, broadcast) is entered — with a friendly error pointing at
// the positional form. The --node flag points at an unreachable endpoint so
// any connection attempt would be visible as a 127.0.0.1 error.

func TestTxDeploymentCloseMissingDSeqFailsFast(t *testing.T) {
	home := setupContextHome(t)

	stdout, stderr, exitCode := runAkt(t, home,
		"tx", "deployment", "close", "--node", unreachableNode)
	combined := stdout + stderr

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for missing dseq, got 0:\n%s", combined)
	}
	if !strings.Contains(combined, "dseq is required") {
		t.Fatalf("expected the dseq guard error, got:\n%s", combined)
	}
	if strings.Contains(combined, "127.0.0.1") {
		t.Fatalf("guard must fire before the connection stage, got:\n%s", combined)
	}
}

func TestTxDeploymentUpdateMissingDSeqFailsFast(t *testing.T) {
	home := setupContextHome(t)

	// The SDL file deliberately does not exist: the guard must fire before
	// the file is read and before deployment 0 is ever queried.
	stdout, stderr, exitCode := runAkt(t, home,
		"tx", "deployment", "update", "does-not-exist.yaml", "--node", unreachableNode)
	combined := stdout + stderr

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for missing dseq, got 0:\n%s", combined)
	}
	if !strings.Contains(combined, "dseq is required") {
		t.Fatalf("expected the dseq guard error, got:\n%s", combined)
	}
	if strings.Contains(combined, "127.0.0.1") {
		t.Fatalf("guard must fire before the connection stage, got:\n%s", combined)
	}
	if strings.Contains(combined, "no such file") {
		t.Fatalf("guard must fire before the SDL file is read, got:\n%s", combined)
	}
}

// --- store edge cases ---

func TestStoreImportMissingFile(t *testing.T) {
	home := setupContextHome(t)

	_, stderr, exitCode := runAkt(t, home, "store", "import", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if exitCode == 0 {
		t.Fatal("expected non-zero exit for missing import file")
	}
	if !strings.Contains(stderr, "no such file") {
		t.Fatalf("expected 'no such file' error, got:\n%s", stderr)
	}
}

// --- help backstop: every registered command answers --help with exit 0 ---
//
// This is the coverage backstop for "every command has at least one e2e
// test": each command path below gets a --help invocation as a named subtest,
// so a failure identifies the offending command. The list mirrors the
// registration in internal/cli/root.go (context/tx/query/monitor/mcp/
// provider/store/workflows/version/completion), internal/cli/chain/tx.go and
// internal/cli/chain/query.go.

func TestAllCommandsHelp(t *testing.T) {
	// Hermetic: help must work on a machine with nothing configured, so
	// point the CLI at an empty home. AKT_HOME (not --home) because several
	// SDK group commands disable flag parsing and would ignore the flag.
	t.Setenv("AKT_HOME", t.TempDir())

	commands := [][]string{
		// root-level
		{"version"},
		{"completion"},
		{"completion", "bash"},
		{"completion", "zsh"},
		{"completion", "fish"},
		{"completion", "powershell"},
		{"mcp"},

		// monitor hub + dashboards
		{"monitor"},
		{"monitor", "network"},
		{"monitor", "provider"},
		{"monitor", "oracle"},
		{"monitor", "bme"},

		// context management
		{"context"},
		{"context", "create"},
		{"context", "use"},
		{"context", "list"},
		{"context", "show"},
		{"context", "edit"},
		{"context", "delete"},
		{"context", "rename"},
		{"context", "log"},

		// context network
		{"context", "network"},
		{"context", "network", "create"},
		{"context", "network", "edit"},
		{"context", "network", "delete"},
		{"context", "network", "list"},
		{"context", "network", "show"},

		// context keys
		{"context", "keys"},
		{"context", "keys", "add"},
		{"context", "keys", "delete"},
		{"context", "keys", "list"},
		{"context", "keys", "show"},
		{"context", "keys", "export"},
		{"context", "keys", "import"},
		{"context", "keys", "rename"},
		{"context", "keys", "mnemonic"},
		{"context", "keys", "parse"},

		// provider (off-chain provider interaction)
		{"provider"},
		{"provider", "status"},
		{"provider", "lease-status"},
		{"provider", "lease-logs"},
		{"provider", "lease-events"},
		{"provider", "lease-shell"},
		{"provider", "send-manifest"},
		{"provider", "get-manifest"},
		{"provider", "migrate-hostnames"},
		{"provider", "migrate-endpoints"},

		// store
		{"store"},
		{"store", "status"},
		{"store", "export"},
		{"store", "import"},

		// built-in workflows
		{"deploy"},
		{"update"},
		{"close"},

		// tx subgroups (internal/cli/chain/tx.go)
		{"tx"},
		{"tx", "audit"},
		{"tx", "authz"},
		{"tx", "bank"},
		{"tx", "bank", "send"},
		{"tx", "bme"},
		{"tx", "broadcast"},
		{"tx", "cert"},
		{"tx", "crisis"},
		{"tx", "decode"},
		{"tx", "deployment"},
		{"tx", "deployment", "close"},
		{"tx", "distribution"},
		{"tx", "encode"},
		{"tx", "escrow"},
		{"tx", "evidence"},
		{"tx", "feegrant"},
		{"tx", "gov"},
		{"tx", "ibc"},
		{"tx", "ibc-transfer"},
		{"tx", "market"},
		{"tx", "multi-sign"},
		{"tx", "oracle"},
		{"tx", "provider"},
		{"tx", "sign"},
		{"tx", "sign-batch"},
		{"tx", "slashing"},
		{"tx", "staking"},
		{"tx", "upgrade"},
		{"tx", "validate-signatures"},
		{"tx", "vesting"},
		{"tx", "wasm"},

		// query subgroups (internal/cli/chain/query.go)
		{"query"},
		{"query", "audit"},
		{"query", "auth"},
		{"query", "authz"},
		{"query", "bank"},
		{"query", "bank", "balances"},
		{"query", "block"},
		{"query", "block-results"},
		{"query", "blocks"},
		{"query", "bme"},
		{"query", "cert"},
		{"query", "deployment"},
		{"query", "deployment", "group"},
		{"query", "distribution"},
		{"query", "escrow"},
		{"query", "evidence"},
		{"query", "feegrant"},
		{"query", "gov"},
		{"query", "ibc"},
		{"query", "ibc-transfer"},
		{"query", "market"},
		{"query", "mint"},
		{"query", "module-name-to-address"},
		{"query", "oracle"},
		{"query", "params"},
		{"query", "provider"},
		{"query", "slashing"},
		{"query", "staking"},
		{"query", "tx"},
		{"query", "txs"},
		{"query", "wasm"},
	}

	for _, path := range commands {
		name := strings.Join(path, " ")
		t.Run(name, func(t *testing.T) {
			args := append(append([]string{}, path...), "--help")
			stdout, stderr, exitCode := runAktNoHome(t, args...)
			if exitCode != 0 {
				t.Fatalf("akt %s --help exited %d\nstdout: %s\nstderr: %s", name, exitCode, stdout, stderr)
			}
			if len(strings.TrimSpace(stdout)) == 0 {
				t.Fatalf("akt %s --help produced no output", name)
			}
		})
	}
}
