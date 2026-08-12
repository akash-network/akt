package e2e

// Gated localnet e2e tests: these run against a real single-validator Akash
// node and are skipped unless one of the following is set:
//
//   AKT_E2E_RPC=http://host:26657   use an existing node read-only. Account
//                                   reads may set AKT_E2E_MNEMONIC. Mutations
//                                   additionally require the explicit chain-ID
//                                   controls documented by localnetRPC.
//   AKT_E2E_LOCALNET=1              bootstrap a throwaway single-validator
//                                   node in docker (image override via
//                                   AKT_E2E_NODE_IMAGE)
//
// The docker bootstrap follows the standard cosmos-sdk single validator
// pattern using the akash binary inside ghcr.io/akash-network/node:
// genesis init -> keys add -> genesis add-account -> genesis gentx ->
// genesis collect -> start. Note the akash CLI names these subcommands
// "add-account" and "collect" (not "add-genesis-account"/"collect-gentxs").

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	defaultNodeImage    = "ghcr.io/akash-network/node@sha256:90a107d333ed47ab382132a9f3cdfc3cf0f1a32cbdfedd2c7eef6bc44f50e993"
	defaultLocalChainID = "localakash"
	externalMutationEnv = "AKT_E2E_EXTERNAL_MUTATIONS"
	mutationChainsEnv   = "AKT_E2E_MUTATION_CHAIN_IDS"
	// Both denominations are funded. A deployment's deposit must match the
	// denom its SDL prices in, and the scaffolds `akt sdl init` emits price
	// in uact — an account holding only uakt cannot deploy them, which is
	// what the deployment lifecycle subtest exercises.
	validatorGenesisFunds = "100000000000000uakt,100000000000000uact" // 100M each
)

// localnet describes the chain endpoint the gated tests talk to.
type localnet struct {
	RPC          string // http://127.0.0.1:<port>
	ChainID      string
	Mnemonic     string // funded account mnemonic ("" if unknown — bank/tx subtests skip)
	Mutations    bool   // true only for a harness-owned chain or an explicitly allowlisted external chain
	HarnessOwned bool   // true only when this test process creates and destroys the complete chain
	Container    string // exact Docker container identity ("" for external RPCs)
}

// bootstrapScript initializes and starts a single-validator akash node.
// It runs inside the node container; the validator key (including its
// mnemonic) is written to /data/validator-key.json for later retrieval via
// docker exec.
const bootstrapScript = `set -e
export HOME=/data
AKASH="akash --home /data"
$AKASH genesis init localnode --chain-id ` + defaultLocalChainID + ` >/dev/null 2>&1
$AKASH keys add validator --keyring-backend test --output json > /data/validator-key.json 2>&1
ADDR=$($AKASH keys show validator -a --keyring-backend test)
$AKASH genesis add-account "$ADDR" ` + validatorGenesisFunds + ` --keyring-backend test
$AKASH genesis gentx validator 1000000000000uakt --chain-id ` + defaultLocalChainID + ` --keyring-backend test --min-self-delegation 1 >/dev/null 2>&1
$AKASH genesis collect >/dev/null 2>&1
echo BOOTSTRAP_DONE
exec $AKASH start --rpc.laddr tcp://0.0.0.0:26657 --grpc.address 0.0.0.0:9090 --minimum-gas-prices 0uakt
`

// localnetRPC resolves the chain endpoint for gated tests: an external node
// via AKT_E2E_RPC, a docker-bootstrapped node via AKT_E2E_LOCALNET=1, or
// skip.
func localnetRPC(t *testing.T) *localnet {
	t.Helper()

	if rpc := strings.TrimSpace(os.Getenv("AKT_E2E_RPC")); rpc != "" {
		configuredChainID := strings.TrimSpace(os.Getenv("AKT_E2E_CHAIN_ID"))
		observedChainID, err := remoteChainID(rpc)
		if err != nil {
			t.Fatalf("inspect AKT_E2E_RPC chain identity: %v", err)
		}
		chainID := configuredChainID
		if chainID == "" {
			chainID = observedChainID
		} else if chainID != observedChainID {
			t.Fatalf("AKT_E2E_CHAIN_ID=%q does not match remote chain %q", chainID, observedChainID)
		}

		mnemonic := strings.TrimSpace(os.Getenv("AKT_E2E_MNEMONIC"))
		mutations, err := externalMutationPolicy(
			configuredChainID,
			observedChainID,
			mnemonic,
			strings.TrimSpace(os.Getenv(externalMutationEnv)),
			os.Getenv(mutationChainsEnv),
		)
		if err != nil {
			t.Fatalf("external localnet mutation policy: %v", err)
		}
		return &localnet{
			RPC:       rpc,
			ChainID:   chainID,
			Mnemonic:  mnemonic,
			Mutations: mutations,
		}
	}

	if os.Getenv("AKT_E2E_LOCALNET") != "1" {
		t.Skip("no localnet: set AKT_E2E_RPC or AKT_E2E_LOCALNET=1")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("AKT_E2E_LOCALNET=1 but docker is not installed: %v", err)
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatalf("AKT_E2E_LOCALNET=1 but the docker daemon is not reachable: %v", err)
	}

	return startDockerLocalnet(t)
}

// startDockerLocalnet boots a single-validator akash node container, waits
// for it to produce blocks, and returns its endpoint plus the funded
// validator mnemonic. The container is removed on test cleanup.
func startDockerLocalnet(t *testing.T) *localnet {
	t.Helper()

	image := os.Getenv("AKT_E2E_NODE_IMAGE")
	if image == "" {
		image = defaultNodeImage
	}

	name := fmt.Sprintf("akt-e2e-localnet-%d-%d", os.Getpid(), time.Now().UnixNano())

	// Pre-pull so the run step (and readiness timeout) isn't dominated by
	// image download time. Failure is non-fatal: the image may already be
	// present locally.
	pull := exec.Command("docker", "pull", image)
	pull.Stdout = io.Discard
	pull.Stderr = io.Discard
	_ = pull.Run()

	scriptPath := filepath.Join(t.TempDir(), "bootstrap.sh")
	if err := os.WriteFile(scriptPath, []byte(bootstrapScript), 0o755); err != nil {
		t.Fatalf("failed to write bootstrap script: %v", err)
	}

	// Bind 26657 to an ephemeral host port to avoid clashing with anything
	// already listening on the default port.
	run := exec.Command("docker", "run", "-d",
		"--name", name,
		"-p", "127.0.0.1::26657",
		"-v", scriptPath+":/bootstrap.sh:ro",
		"--entrypoint", "/bin/sh",
		image, "/bootstrap.sh")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("docker run failed: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})

	// Resolve the ephemeral host port for 26657/tcp.
	portOut, err := exec.Command("docker", "port", name, "26657/tcp").Output()
	if err != nil {
		t.Fatalf("docker port failed: %v", err)
	}
	// Output like "0.0.0.0:55000" or "127.0.0.1:55000" (possibly multiple lines).
	hostPort := ""
	for _, line := range strings.Split(strings.TrimSpace(string(portOut)), "\n") {
		if idx := strings.LastIndex(line, ":"); idx >= 0 {
			hostPort = strings.TrimSpace(line[idx+1:])
			break
		}
	}
	if hostPort == "" {
		t.Fatalf("could not determine mapped RPC port from %q", portOut)
	}

	rpc := "http://127.0.0.1:" + hostPort
	waitForChain(t, name, rpc)

	// Retrieve the validator mnemonic written during bootstrap.
	keyOut, err := exec.Command("docker", "exec", name, "cat", "/data/validator-key.json").Output()
	if err != nil {
		t.Fatalf("failed to read validator key from container: %v", err)
	}
	var key struct {
		Address  string `json:"address"`
		Mnemonic string `json:"mnemonic"`
	}
	if err := json.Unmarshal(keyOut, &key); err != nil {
		t.Fatalf("failed to parse validator key JSON: %v\n%s", err, keyOut)
	}
	if key.Mnemonic == "" {
		t.Fatalf("validator key JSON has no mnemonic:\n%s", keyOut)
	}

	return &localnet{
		RPC:          rpc,
		ChainID:      defaultLocalChainID,
		Mnemonic:     key.Mnemonic,
		Mutations:    true,
		HarnessOwned: true,
		Container:    name,
	}
}

func remoteChainID(rpc string) (string, error) {
	statusURL := strings.TrimRight(rpc, "/") + "/status"
	if strings.HasPrefix(statusURL, "tcp://") {
		statusURL = "http://" + strings.TrimPrefix(statusURL, "tcp://")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(statusURL)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", statusURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s returned HTTP %d", statusURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", statusURL, err)
	}
	if len(body) > 1<<20 {
		return "", fmt.Errorf("GET %s exceeded the 1 MiB status limit", statusURL)
	}
	var status struct {
		Result struct {
			NodeInfo struct {
				Network string `json:"network"`
			} `json:"node_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return "", fmt.Errorf("decode %s: %w", statusURL, err)
	}
	if status.Result.NodeInfo.Network == "" {
		return "", fmt.Errorf("GET %s returned no chain ID", statusURL)
	}
	return status.Result.NodeInfo.Network, nil
}

func externalMutationPolicy(expectedChainID, observedChainID, mnemonic, optIn, allowlist string) (bool, error) {
	if optIn == "" || optIn == "0" {
		return false, nil
	}
	if optIn != "1" {
		return false, fmt.Errorf("%s must be 1 to enable or unset to disable mutations", externalMutationEnv)
	}
	if expectedChainID == "" {
		return false, fmt.Errorf("AKT_E2E_CHAIN_ID is required when %s=1", externalMutationEnv)
	}
	if expectedChainID != observedChainID {
		return false, fmt.Errorf("expected chain %q but remote reports %q", expectedChainID, observedChainID)
	}
	if expectedChainID == "akashnet-2" || expectedChainID == "akashnet-1" {
		return false, fmt.Errorf("refusing mutations on production chain %q", expectedChainID)
	}
	if mnemonic == "" {
		return false, fmt.Errorf("AKT_E2E_MNEMONIC is required when %s=1", externalMutationEnv)
	}

	for _, allowed := range strings.Split(allowlist, ",") {
		if strings.TrimSpace(allowed) == expectedChainID {
			return true, nil
		}
	}
	return false, fmt.Errorf("%s must explicitly contain chain %q", mutationChainsEnv, expectedChainID)
}

func TestRemoteChainID(t *testing.T) {
	server := &http.Server{ReadHeaderTimeout: time.Second}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("status request path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"result":{"node_info":{"network":"sandbox-9"}}}`)
	})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	chainID, err := remoteChainID("tcp://" + listener.Addr().String())
	if err != nil || chainID != "sandbox-9" {
		t.Fatalf("remoteChainID() = %q, %v", chainID, err)
	}
}

func TestExternalMutationPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		expected  string
		observed  string
		mnemonic  string
		optIn     string
		allowlist string
		want      bool
		wantError bool
	}{
		{name: "read only by default", expected: "sandbox", observed: "sandbox", mnemonic: "words", want: false},
		{name: "explicit sandbox", expected: "sandbox", observed: "sandbox", mnemonic: "words", optIn: "1", allowlist: "other, sandbox", want: true},
		{name: "expected ID required", observed: "sandbox", mnemonic: "words", optIn: "1", allowlist: "sandbox", wantError: true},
		{name: "observed mismatch", expected: "sandbox", observed: "other", mnemonic: "words", optIn: "1", allowlist: "sandbox", wantError: true},
		{name: "production prohibited", expected: "akashnet-2", observed: "akashnet-2", mnemonic: "words", optIn: "1", allowlist: "akashnet-2", wantError: true},
		{name: "mnemonic required", expected: "sandbox", observed: "sandbox", optIn: "1", allowlist: "sandbox", wantError: true},
		{name: "wildcard is not an allowlist", expected: "sandbox", observed: "sandbox", mnemonic: "words", optIn: "1", allowlist: "*", wantError: true},
		{name: "invalid opt in", expected: "sandbox", observed: "sandbox", mnemonic: "words", optIn: "yes", allowlist: "sandbox", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := externalMutationPolicy(test.expected, test.observed, test.mnemonic, test.optIn, test.allowlist)
			if got != test.want || (err != nil) != test.wantError {
				t.Fatalf("externalMutationPolicy() = %t, %v; want %t, error=%t", got, err, test.want, test.wantError)
			}
		})
	}
}

func TestEphemeralBankTransferRequiresHarnessOwnership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		net  localnet
		want bool
	}{
		{name: "harness owned", net: localnet{Mutations: true, HarnessOwned: true}, want: true},
		{name: "external mutation opt in", net: localnet{Mutations: true}, want: false},
		{name: "read only external", net: localnet{}, want: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.net.Mutations && test.net.HarnessOwned; got != test.want {
				t.Fatalf("ephemeral bank transfer eligibility = %t, want %t", got, test.want)
			}
		})
	}
}

func TestUniqueNewDeployment(t *testing.T) {
	t.Parallel()
	before := map[string]string{"10": "active", "11": "closed"}

	if dseq, found, err := uniqueNewDeployment(before, before); dseq != "" || found || err != nil {
		t.Fatalf("unchanged set = %q, %t, %v", dseq, found, err)
	}
	if dseq, found, err := uniqueNewDeployment(before, map[string]string{"10": "active", "11": "closed", "12": "active"}); dseq != "12" || !found || err != nil {
		t.Fatalf("unique set difference = %q, %t, %v", dseq, found, err)
	}
	if _, _, err := uniqueNewDeployment(before, map[string]string{"10": "active", "12": "active", "13": "active"}); err == nil {
		t.Fatal("expected an ambiguous set difference to fail")
	}
}

func requireLocalnetMutations(t *testing.T, net *localnet) {
	t.Helper()
	if net.Mutations {
		return
	}
	t.Skipf("external RPC is read-only; set %s=1 and allowlist AKT_E2E_CHAIN_ID in %s to enable transactions", externalMutationEnv, mutationChainsEnv)
}

var heightRe = regexp.MustCompile(`"latest_block_height"\s*:\s*"(\d+)"`)

// waitForChain polls the node's /status endpoint until it reports at least
// one produced block, failing with the container logs on timeout.
func waitForChain(t *testing.T, containerName, rpc string) {
	t.Helper()

	deadline := time.Now().Add(120 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(rpc + "/status")
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == http.StatusOK {
				if m := heightRe.FindSubmatch(body); m != nil && string(m[1]) != "0" {
					return
				}
			}
		}
		time.Sleep(time.Second)
	}

	logs, _ := exec.Command("docker", "logs", containerName).CombinedOutput()
	t.Fatalf("localnet did not produce a block within timeout; container logs:\n%s", logs)
}

// setupLocalnetHome creates an akt home wired to the localnet: a network
// pointing at the node's RPC, an active context, and (when the mnemonic is
// known) the funded validator key imported into the test keyring.
func setupLocalnetHome(t *testing.T, net *localnet) string {
	t.Helper()

	home := t.TempDir()
	initHome(t, home)

	mustRunAkt(t, home, "context", "network", "create", "localnet",
		"--chain-id", net.ChainID, "--rpc", net.RPC)
	mustRunAkt(t, home, "context", "create", "localnet", "--network", "localnet", "--set-current")

	if net.Mnemonic != "" {
		src := filepath.Join(t.TempDir(), "mnemonic.txt")
		if err := os.WriteFile(src, []byte(net.Mnemonic+"\n"), 0o600); err != nil {
			t.Fatalf("failed to write mnemonic file: %v", err)
		}
		mustRunAkt(t, home, "context", "keys", "add", "validator", "--source", src)
	}

	return home
}

func TestLocalnet(t *testing.T) {
	net := localnetRPC(t)
	home := setupLocalnetHome(t, net)

	t.Run("QueryBlock", func(t *testing.T) {
		// No --node: the block passthrough commands must resolve the RPC
		// endpoint from the active context like module queries do.
		stdout := mustRunAkt(t, home, "query", "block")
		if !strings.Contains(stdout, net.ChainID) {
			t.Fatalf("expected query block output to contain chain-id %q, got:\n%s", net.ChainID, stdout)
		}
	})

	t.Run("QueryStakingValidators", func(t *testing.T) {
		stdout := stripANSI(mustRunAkt(t, home, "query", "staking", "validators"))
		if !strings.Contains(strings.ToLower(stdout), "bonded") {
			t.Fatalf("expected a bonded validator in output, got:\n%s", stdout)
		}
	})

	t.Run("BankBalances", func(t *testing.T) {
		if net.Mnemonic == "" {
			t.Skip("no funded mnemonic (set AKT_E2E_MNEMONIC when using AKT_E2E_RPC)")
		}

		addr := strings.TrimSpace(stripANSI(mustRunAkt(t, home, "context", "keys", "show", "validator", "-a")))
		balancesOut := mustRunAkt(t, home, "query", "bank", "balances", addr, "--output", "json")
		balances, err := decodeBankBalances([]byte(balancesOut))
		if err != nil {
			t.Fatalf("decode bank balances for %s: %v", addr, err)
		}
		positive := false
		for _, amount := range balances {
			positive = positive || amount.Sign() > 0
		}
		if !positive {
			t.Fatalf("funded account %s returned no positive balance", addr)
		}
		if !net.HarnessOwned {
			return
		}

		stdout := stripANSI(mustRunAkt(t, home, "query", "bank", "balances", addr))
		if !strings.Contains(stdout, "ACT") || !strings.Contains(stdout, "AKT") {
			t.Fatalf("genesis account %s did not render both funded denominations:\n%s", addr, stdout)
		}
		aktUAKT := balances["uakt"]
		aktUACT := balances["uact"]
		if aktUAKT == nil || aktUACT == nil {
			t.Fatalf("Docker genesis account %s omitted uakt or uact: %v", addr, balances)
		}
		observer, err := nativeNodeObserverForLocalnet(net)
		if err != nil {
			t.Fatalf("construct native node observer: %v", err)
		}
		nativeUAKT, err := observer.bankBalance(t.Context(), addr, "uakt")
		if err != nil {
			t.Fatalf("read native uakt balance: %v", err)
		}
		nativeUACT, err := observer.bankBalance(t.Context(), addr, "uact")
		if err != nil {
			t.Fatalf("read native uact balance: %v", err)
		}
		if aktUAKT.Cmp(nativeUAKT) != 0 || aktUACT.Cmp(nativeUACT) != 0 {
			t.Fatalf("akt balances uakt=%s uact=%s disagree with native uakt=%s uact=%s", aktUAKT, aktUACT, nativeUAKT, nativeUACT)
		}
	})

	t.Run("TxBankSendRecordsActionLog", func(t *testing.T) {
		requireLocalnetMutations(t, net)
		if !net.HarnessOwned {
			t.Skip("bank transfer creates an ephemeral recipient and is restricted to the harness-owned throwaway chain")
		}

		mustRunAkt(t, home, "context", "keys", "add", "recipient", "--no-backup")
		recipient := strings.TrimSpace(stripANSI(mustRunAkt(t, home, "context", "keys", "show", "recipient", "-a")))
		beforeBalance := bankBalanceAmount(t, home, recipient, "uakt")
		observer, err := nativeNodeObserverForLocalnet(net)
		if err != nil {
			t.Fatalf("construct native node observer: %v", err)
		}
		nativeBeforeBalance, err := observer.bankBalance(t.Context(), recipient, "uakt")
		if err != nil {
			t.Fatalf("read native recipient balance before send: %v", err)
		}
		if nativeBeforeBalance.Cmp(beforeBalance) != 0 {
			t.Fatalf("akt and native pre-send balances disagree: akt=%s native=%s", beforeBalance, nativeBeforeBalance)
		}

		// Default gas (auto): exercises gas simulation end to end.
		stdout, stderr, exitCode := runAkt(t, home,
			"tx", "bank", "send", "validator", recipient, "1000000uakt",
			"--fees", "5000uakt", "--yes")
		if exitCode != 0 {
			t.Fatalf("tx bank send failed (exit %d)\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
		}
		txHash := localnetTxHashFromPretty(t, stdout)

		// A sync broadcast proves CheckTx acceptance only. Independently query
		// the node until DeliverTx exists, then require exact hash, height, and
		// success before treating any balance observation as authoritative.
		var committed localnetCommittedTx
		var lastTxDiagnostic string
		if !pollUntil(45*time.Second, func() bool {
			queryOut, queryErr, queryExit := runAkt(t, home, "query", "tx", txHash, "--output", "json")
			lastTxDiagnostic = fmt.Sprintf("exit=%d stdout=%dB stderr=%dB", queryExit, len(queryOut), len(queryErr))
			if queryExit != 0 {
				return false
			}
			var err error
			committed, err = decodeLocalnetCommittedTx([]byte(queryOut))
			if err != nil {
				t.Fatalf("decode committed bank transaction: %v", err)
			}
			lastTxDiagnostic = queryOut
			return true
		}) {
			t.Fatalf("bank transaction %s was not queryable after CheckTx acceptance (%s)", txHash, lastTxDiagnostic)
		}
		if !strings.EqualFold(committed.Hash, txHash) || committed.Height == 0 {
			t.Fatalf("committed transaction identity = hash %q height %d, want %s at a positive height", committed.Hash, committed.Height, txHash)
		}
		if committed.Code != 0 {
			t.Fatalf("bank transaction %s failed in DeliverTx with code %d: %s", txHash, committed.Code, committed.RawLog)
		}
		nativeCommitted, err := observer.waitForTransaction(t.Context(), txHash)
		if err != nil {
			t.Fatalf("query native bank transaction receipt: %v", err)
		}
		if nativeCommitted.Hash != txHash || nativeCommitted.Height != committed.Height || nativeCommitted.Code != committed.Code || nativeCommitted.RawLog != committed.RawLog {
			t.Fatalf("native receipt %+v disagrees with akt receipt %+v", nativeCommitted, committed)
		}

		// Ground truth: poll the recipient balance until the transfer lands.
		deadline := time.Now().Add(15 * time.Second)
		funded := false
		lastBalance := new(big.Int).Set(beforeBalance)
		lastNativeBalance := new(big.Int).Set(nativeBeforeBalance)
		wantDelta := big.NewInt(1_000_000)
		for time.Now().Before(deadline) {
			lastBalance = bankBalanceAmount(t, home, recipient, "uakt")
			lastNativeBalance, err = observer.bankBalance(t.Context(), recipient, "uakt")
			if err != nil {
				t.Fatalf("read native recipient balance after send: %v", err)
			}
			aktDelta := new(big.Int).Sub(lastBalance, beforeBalance)
			nativeDelta := new(big.Int).Sub(lastNativeBalance, nativeBeforeBalance)
			if aktDelta.Cmp(wantDelta) == 0 && nativeDelta.Cmp(wantDelta) == 0 && lastBalance.Cmp(lastNativeBalance) == 0 {
				funded = true
				break
			}
			time.Sleep(2 * time.Second)
		}
		if !funded {
			t.Fatalf("recipient %s uakt balances moved akt %s->%s and native %s->%s, want exact matching %s deltas; send output:\n%s%s\ncommitted transaction:\n%s",
				recipient, beforeBalance, lastBalance, nativeBeforeBalance, lastNativeBalance, wantDelta, stdout, stderr, lastTxDiagnostic)
		}

		// AKT-211 acceptance: the synchronous broadcast is recorded in the
		// context action log. CheckTx acceptance initially records it as pending;
		// reading the log reconciles the hash after the balance poll above proves
		// the transaction was committed (SPEC §10.11.4).
		var entries []localnetTxAction
		if !pollUntil(20*time.Second, func() bool {
			entries = localnetTxActions(t, home)
			return len(entries) == 1 && entries[0].Status == "success"
		}) {
			t.Fatalf("bank action log did not converge to one successful entry: %+v", entries)
		}
		entry := entries[0]
		if entry.Type != "tx" || entry.Action != "bank.MsgSend" || entry.Account != "validator" ||
			entry.Height != nativeCommitted.Height || entry.GasUsed <= 0 || entry.DSeq != 0 ||
			entry.Code != nativeCommitted.Code || entry.Error != "" || entry.Timestamp.IsZero() {
			t.Fatalf("bank action log entry has incomplete semantics: %+v", entry)
		}
		decodedHash, err := hex.DecodeString(entry.TxHash)
		if err != nil || len(decodedHash) != 32 || !strings.EqualFold(entry.TxHash, txHash) {
			t.Fatalf("bank action log hash %q does not bind to native transaction %s", entry.TxHash, txHash)
		}
	})
}

type localnetCommittedTx struct {
	Hash   string
	Height int64
	Code   uint32
	RawLog string
}

func decodeLocalnetCommittedTx(data []byte) (localnetCommittedTx, error) {
	var response struct {
		Hash   string          `json:"txhash"`
		Height json.RawMessage `json:"height"`
		Code   json.RawMessage `json:"code"`
		RawLog string          `json:"raw_log"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return localnetCommittedTx{}, err
	}
	if response.Hash == "" || len(response.Height) == 0 || len(response.Code) == 0 {
		return localnetCommittedTx{}, fmt.Errorf("response omitted txhash, height, or code")
	}

	height, err := parseJSONInteger(response.Height, 64)
	if err != nil || height == 0 {
		return localnetCommittedTx{}, fmt.Errorf("invalid transaction height %s", response.Height)
	}
	code, err := parseJSONInteger(response.Code, 32)
	if err != nil {
		return localnetCommittedTx{}, fmt.Errorf("invalid transaction code %s", response.Code)
	}

	return localnetCommittedTx{
		Hash:   response.Hash,
		Height: int64(height),
		Code:   uint32(code),
		RawLog: response.RawLog,
	}, nil
}

func parseJSONInteger(raw json.RawMessage, bitSize int) (uint64, error) {
	value := strings.TrimSpace(string(raw))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return 0, err
		}
		value = decoded
	}
	return strconv.ParseUint(value, 10, bitSize)
}

func TestDecodeLocalnetCommittedTx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		want    localnetCommittedTx
		wantErr bool
	}{
		{
			name: "Cosmos string height",
			body: `{"txhash":"ABC123","height":"17","code":0,"raw_log":""}`,
			want: localnetCommittedTx{Hash: "ABC123", Height: 17},
		},
		{
			name: "numeric height and quoted code",
			body: `{"txhash":"DEF456","height":18,"code":"7","raw_log":"deliver failed"}`,
			want: localnetCommittedTx{Hash: "DEF456", Height: 18, Code: 7, RawLog: "deliver failed"},
		},
		{name: "malformed JSON", body: `{`, wantErr: true},
		{name: "missing hash", body: `{"height":"1","code":0}`, wantErr: true},
		{name: "zero height", body: `{"txhash":"ABC","height":"0","code":0}`, wantErr: true},
		{name: "negative height", body: `{"txhash":"ABC","height":"-1","code":0}`, wantErr: true},
		{name: "fractional height", body: `{"txhash":"ABC","height":1.5,"code":0}`, wantErr: true},
		{name: "overflow code", body: `{"txhash":"ABC","height":"1","code":"4294967296"}`, wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := decodeLocalnetCommittedTx([]byte(test.body))
			if (err != nil) != test.wantErr {
				t.Fatalf("decodeLocalnetCommittedTx() error = %v, want error=%t", err, test.wantErr)
			}
			if !test.wantErr && got != test.want {
				t.Fatalf("decodeLocalnetCommittedTx() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func bankBalanceAmount(t *testing.T, home, account, denom string) *big.Int {
	t.Helper()
	out := mustRunAkt(t, home, "query", "bank", "balances", account, "--output", "json")
	balances, err := decodeBankBalances([]byte(out))
	if err != nil {
		t.Fatalf("decode bank balance JSON for %s: %v\n%s", account, err, out)
	}
	if amount := balances[denom]; amount != nil {
		return new(big.Int).Set(amount)
	}
	return new(big.Int)
}

func decodeBankBalances(data []byte) (map[string]*big.Int, error) {
	var response struct {
		Balances []struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode bank balance response: %w", err)
	}
	result := make(map[string]*big.Int, len(response.Balances))
	for _, coin := range response.Balances {
		if coin.Denom == "" || strings.TrimSpace(coin.Denom) != coin.Denom {
			return nil, fmt.Errorf("bank balance returned invalid denomination %q", coin.Denom)
		}
		if _, found := result[coin.Denom]; found {
			return nil, fmt.Errorf("bank balance returned duplicate denomination %s", coin.Denom)
		}
		amount := new(big.Int)
		if _, ok := amount.SetString(coin.Amount, 10); !ok || amount.Sign() < 0 || amount.String() != coin.Amount {
			return nil, fmt.Errorf("bank balance returned invalid %s amount %q", coin.Denom, coin.Amount)
		}
		result[coin.Denom] = amount
	}
	return result, nil
}

func TestDecodeBankBalances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantDenom string
		want      string
		wantError bool
	}{
		{name: "external single denom", body: `{"balances":[{"denom":"ibc/ABC","amount":"7"}]}`, wantDenom: "ibc/ABC", want: "7"},
		{name: "empty balance list", body: `{"balances":[]}`},
		{name: "duplicate denom", body: `{"balances":[{"denom":"uakt","amount":"1"},{"denom":"uakt","amount":"2"}]}`, wantError: true},
		{name: "empty denom", body: `{"balances":[{"denom":"","amount":"1"}]}`, wantError: true},
		{name: "negative amount", body: `{"balances":[{"denom":"uakt","amount":"-1"}]}`, wantError: true},
		{name: "noncanonical amount", body: `{"balances":[{"denom":"uakt","amount":"01"}]}`, wantError: true},
		{name: "malformed JSON", body: `{`, wantError: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := decodeBankBalances([]byte(test.body))
			if (err != nil) != test.wantError {
				t.Fatalf("decodeBankBalances() error = %v, want error=%t", err, test.wantError)
			}
			if test.wantDenom != "" && (got[test.wantDenom] == nil || got[test.wantDenom].String() != test.want) {
				t.Fatalf("decodeBankBalances()[%q] = %v, want %s", test.wantDenom, got[test.wantDenom], test.want)
			}
		})
	}
}

// pollUntil retries fn until it returns true or the deadline passes. Chain
// state is only visible a block or two after a tx is accepted, so assertions
// that read back what a tx wrote must poll rather than read once.
func pollUntil(timeout time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

type queryCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func TestValidateDeadEndpointQueryAttempt(t *testing.T) {
	t.Parallel()

	help := queryCommandResult{stdout: "generated help document", exitCode: 0}
	tests := []struct {
		name      string
		attempt   queryCommandResult
		wantError bool
	}{
		{
			name:    "transport failure",
			attempt: queryCommandResult{stderr: "opaque transport diagnostic", exitCode: 1},
		},
		{
			name:      "help-only success",
			attempt:   queryCommandResult{stdout: "generated help document", exitCode: 0},
			wantError: true,
		},
		{
			name:      "help-only nonzero exit",
			attempt:   queryCommandResult{stdout: "\n generated help document \n", exitCode: 1},
			wantError: true,
		},
		{
			name:      "silent failure",
			attempt:   queryCommandResult{exitCode: 1},
			wantError: true,
		},
		{
			name:      "invalid help probe",
			attempt:   queryCommandResult{stderr: "opaque transport diagnostic", exitCode: 1},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			helpResult := help
			if test.name == "invalid help probe" {
				helpResult.exitCode = 1
			}
			err := validateDeadEndpointQueryAttempt(test.attempt, helpResult)
			if (err != nil) != test.wantError {
				t.Fatalf("validateDeadEndpointQueryAttempt() error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}

// validateDeadEndpointQueryAttempt checks that an invocation reached query
// execution rather than returning a command group's help document.
func validateDeadEndpointQueryAttempt(attempt, help queryCommandResult) error {
	helpOutput := normalizeQueryCommandOutput(help)
	if help.exitCode != 0 {
		return fmt.Errorf("help probe exited %d", help.exitCode)
	}
	if helpOutput == "" {
		return fmt.Errorf("help probe produced no output")
	}

	if attempt.exitCode == 0 {
		return fmt.Errorf("dead-endpoint query exited successfully")
	}

	attemptOutput := normalizeQueryCommandOutput(attempt)
	if attemptOutput == "" {
		return fmt.Errorf("dead-endpoint query failed without a diagnostic")
	}
	if attemptOutput == helpOutput {
		return fmt.Errorf("dead-endpoint query returned only its help document")
	}

	return nil
}

func normalizeQueryCommandOutput(result queryCommandResult) string {
	return strings.TrimSpace(stripANSI(result.stdout + "\n" + result.stderr))
}

type localnetQueryCase struct {
	args   []string
	assert func(*testing.T, map[string]any)
}

func decodeLocalnetQueryJSON(t *testing.T, args []string, stdout string) map[string]any {
	t.Helper()

	var document map[string]any
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("akt %s did not return one JSON object: %v", strings.Join(args, " "), err)
	}
	if len(document) == 0 {
		t.Fatalf("akt %s returned an empty JSON object", strings.Join(args, " "))
	}

	return document
}

func assertQueryArray(field string, requireValue bool) func(*testing.T, map[string]any) {
	return func(t *testing.T, document map[string]any) {
		t.Helper()

		value, exists := document[field]
		if !exists {
			t.Fatalf("query response has no %q field", field)
		}
		items, ok := value.([]any)
		if !ok {
			t.Fatalf("query response field %q is not an array", field)
		}
		if requireValue && len(items) == 0 {
			t.Fatalf("query response field %q is empty", field)
		}
	}
}

func assertQueryObject(field string, requireValue bool) func(*testing.T, map[string]any) {
	return func(t *testing.T, document map[string]any) {
		t.Helper()

		value, exists := document[field]
		if !exists {
			t.Fatalf("query response has no %q field", field)
		}
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("query response field %q is not an object", field)
		}
		if requireValue && !queryJSONHasScalar(object) {
			t.Fatalf("query response field %q contains no state value", field)
		}
	}
}

func assertQueryRootKeys(keys ...string) func(*testing.T, map[string]any) {
	return func(t *testing.T, document map[string]any) {
		t.Helper()
		for _, key := range keys {
			value, exists := document[key]
			if !exists || value == nil {
				t.Fatalf("query response has no populated root field %q", key)
			}
		}
	}
}

func queryJSONHasScalar(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for _, child := range value {
			if queryJSONHasScalar(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if queryJSONHasScalar(child) {
				return true
			}
		}
	case nil:
		return false
	default:
		return true
	}

	return false
}

func queryJSONContainsString(value any, key, expected string) bool {
	switch value := value.(type) {
	case map[string]any:
		if actual, ok := value[key].(string); ok && actual == expected {
			return true
		}
		for _, child := range value {
			if queryJSONContainsString(child, key, expected) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if queryJSONContainsString(child, key, expected) {
				return true
			}
		}
	}

	return false
}

func queryJSONContainsPositiveInteger(value any, key string) bool {
	switch value := value.(type) {
	case map[string]any:
		if positiveJSONInteger(value[key]) {
			return true
		}
		for _, child := range value {
			if queryJSONContainsPositiveInteger(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if queryJSONContainsPositiveInteger(child, key) {
				return true
			}
		}
	}

	return false
}

func positiveJSONInteger(value any) bool {
	switch value := value.(type) {
	case float64:
		return value > 0 && value == float64(uint64(value))
	case string:
		seenNonZero := false
		for _, digit := range value {
			if digit < '0' || digit > '9' {
				return false
			}
			seenNonZero = seenNonZero || digit != '0'
		}
		return seenNonZero
	default:
		return false
	}
}

func queryJSONHasPositiveCoin(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		denom, hasDenom := value["denom"].(string)
		if hasDenom && denom != "" && positiveJSONInteger(value["amount"]) {
			return true
		}
		for _, child := range value {
			if queryJSONHasPositiveCoin(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if queryJSONHasPositiveCoin(child) {
				return true
			}
		}
	}

	return false
}

// TestLocalnetQueries exercises every module's read path against a live
// chain. The offline suite proves these commands parse their arguments;
// only a real node proves the query round-trips and renders — a query that
// builds a malformed request or mishandles an empty result set passes
// offline and fails here.
func TestLocalnetQueries(t *testing.T) {
	net := localnetRPC(t)
	home := setupLocalnetHome(t, net)
	actionCountBefore := localnetActionLogCount(t, home)
	var failedRPCRequests atomic.Int64
	failedRPC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		failedRPCRequests.Add(1)
		http.Error(w, "intentional localnet transport probe failure", http.StatusServiceUnavailable)
	}))
	t.Cleanup(failedRPC.Close)
	deadHome := setupLocalnetHome(t, &localnet{
		RPC:     failedRPC.URL,
		ChainID: net.ChainID,
	})

	// Queries needing no account. Chain-wide reads and every module's
	// params, which is the one read path every module is guaranteed to
	// have on a chain with no activity on it yet.
	// Every entry must be a runnable query, not a command group. A group
	// prints its help and exits 0 without reaching the chain, so it passes
	// this matrix no matter what the node does. `query params` is the
	// x/params group — its only subcommand, subspace, needs arguments — and
	// it is deliberately absent for that reason. The check for a new entry:
	// with the context pointed at a dead endpoint it must exit non-zero.
	queries := []localnetQueryCase{
		{
			args: []string{"query", "block"},
			assert: func(t *testing.T, document map[string]any) {
				t.Helper()
				if !queryJSONContainsString(document, "chain_id", net.ChainID) {
					t.Fatalf("block response does not identify chain %q", net.ChainID)
				}
			},
		},
		{args: []string{"query", "auth", "params"}, assert: assertQueryRootKeys("max_memo_characters", "tx_sig_limit")},
		{
			args: []string{"query", "bank", "total"},
			assert: func(t *testing.T, document map[string]any) {
				t.Helper()
				assertQueryArray("supply", true)(t, document)
				if !queryJSONHasPositiveCoin(document["supply"]) {
					t.Fatal("bank supply contains no positive coin")
				}
			},
		},
		{
			args: []string{"query", "staking", "validators"},
			assert: func(t *testing.T, document map[string]any) {
				t.Helper()
				assertQueryArray("validators", true)(t, document)
				if !queryJSONContainsString(document["validators"], "status", "BOND_STATUS_BONDED") {
					t.Fatal("staking response contains no bonded validator")
				}
			},
		},
		{args: []string{"query", "staking", "params"}, assert: assertQueryRootKeys("unbonding_time", "bond_denom", "max_validators")},
		{
			args: []string{"query", "staking", "pool"},
			assert: func(t *testing.T, document map[string]any) {
				t.Helper()
				if !queryJSONContainsPositiveInteger(document, "bonded_tokens") {
					t.Fatal("staking pool reports no bonded tokens")
				}
			},
		},
		{args: []string{"query", "distribution", "params"}, assert: assertQueryRootKeys("community_tax", "withdraw_addr_enabled")},
		{args: []string{"query", "slashing", "params"}, assert: assertQueryRootKeys("signed_blocks_window", "downtime_jail_duration")},
		{args: []string{"query", "mint", "params"}, assert: assertQueryRootKeys("mint_denom", "blocks_per_year")},
		{args: []string{"query", "gov", "params"}, assert: assertQueryObject("params", true)},
		{
			args: []string{"query", "upgrade", "module_versions"},
			assert: func(t *testing.T, document map[string]any) {
				t.Helper()
				assertQueryArray("module_versions", true)(t, document)
				if !queryJSONContainsPositiveInteger(document["module_versions"], "version") {
					t.Fatal("upgrade response contains no positive module version")
				}
			},
		},
		// Akash modules — the reason this CLI exists, and previously
		// untested against a chain at any point in the suite.
		{args: []string{"query", "deployment", "params"}, assert: assertQueryObject("params", true)},
		{args: []string{"query", "market", "params"}, assert: assertQueryObject("params", true)},
		{args: []string{"query", "provider", "list"}, assert: assertQueryArray("providers", false)},
		{args: []string{"query", "audit", "list"}, assert: assertQueryArray("providers", false)},
		{args: []string{"query", "bme", "params"}, assert: assertQueryObject("params", true)},
		{args: []string{"query", "oracle", "params"}, assert: assertQueryObject("params", true)},
	}

	if net.Mnemonic != "" {
		addr := validatorAddr(t, home)
		// Account-scoped reads. These take the address positionally: the
		// `list` subcommand form does not exist, so a wrong invocation
		// here fails with a filter parse error rather than an empty table.
		queries = append(queries,
			localnetQueryCase{
				args: []string{"query", "bank", "balances", addr},
				assert: func(t *testing.T, document map[string]any) {
					t.Helper()
					assertQueryArray("balances", true)(t, document)
					if !queryJSONHasPositiveCoin(document["balances"]) {
						t.Fatal("funded account contains no positive coin")
					}
				},
			},
			localnetQueryCase{
				args: []string{"query", "auth", "account", addr},
				assert: func(t *testing.T, document map[string]any) {
					t.Helper()
					if _, exists := document["account"]; !exists {
						t.Fatal("auth response has no account field")
					}
					if !queryJSONContainsString(document["account"], "address", addr) {
						t.Fatal("auth response does not contain the requested account address")
					}
				},
			},
			localnetQueryCase{args: []string{"query", "cert", "list", addr}, assert: assertQueryArray("certificates", false)},
			localnetQueryCase{args: []string{"query", "deployment", addr}, assert: assertQueryArray("deployments", false)},
			localnetQueryCase{args: []string{"query", "market", "order", addr}, assert: assertQueryArray("orders", false)},
			localnetQueryCase{args: []string{"query", "market", "bid", addr}, assert: assertQueryArray("bids", false)},
			localnetQueryCase{args: []string{"query", "market", "lease", addr}, assert: assertQueryArray("leases", false)},
		)
	}

	for _, query := range queries {
		t.Run(strings.Join(query.args[1:], " "), func(t *testing.T) {
			args := append(append([]string{}, query.args...), "--output", "json")
			stdout, stderr, exit := runAkt(t, home, args...)
			if exit != 0 {
				t.Fatalf("akt %s exited %d (stdout %d bytes, stderr %d bytes)",
					strings.Join(args, " "), exit, len(stdout), len(stderr))
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("akt %s succeeded with unexpected stderr (%d bytes)", strings.Join(args, " "), len(stderr))
			}

			document := decodeLocalnetQueryJSON(t, args, stdout)
			query.assert(t, document)

			requestsBefore := failedRPCRequests.Load()
			deadStdout, deadStderr, deadExit := runAkt(t, deadHome, args...)
			requestsAfterAttempt := failedRPCRequests.Load()
			helpArgs := append(append([]string{}, args...), "--help")
			helpStdout, helpStderr, helpExit := runAkt(t, deadHome, helpArgs...)
			requestsAfterHelp := failedRPCRequests.Load()
			if requestsAfterAttempt <= requestsBefore {
				t.Fatalf("akt %s failed before making a transport request", strings.Join(args, " "))
			}
			if requestsAfterHelp != requestsAfterAttempt {
				t.Fatalf("akt %s --help made %d transport requests", strings.Join(args, " "), requestsAfterHelp-requestsAfterAttempt)
			}
			if err := validateDeadEndpointQueryAttempt(
				queryCommandResult{stdout: deadStdout, stderr: deadStderr, exitCode: deadExit},
				queryCommandResult{stdout: helpStdout, stderr: helpStderr, exitCode: helpExit},
			); err != nil {
				t.Fatalf("akt %s did not prove a leaf transport attempt: %v", strings.Join(args, " "), err)
			}
		})
	}

	if actionCountAfter := localnetActionLogCount(t, home); actionCountAfter != actionCountBefore {
		t.Fatalf("read-only query matrix changed the action log from %d to %d entries", actionCountBefore, actionCountAfter)
	}
}

// validatorAddr returns the funded validator's bech32 address.
func validatorAddr(t *testing.T, home string) string {
	t.Helper()
	return strings.TrimSpace(stripANSI(mustRunAkt(t, home, "context", "keys", "show", "validator", "-a")))
}

// sdlPricingDenomRe extracts the denom an SDL prices in. A deployment's
// deposit must use the same denom, so the lifecycle test reads it from the
// generated SDL rather than hardcoding one and drifting from the scaffold.
var sdlPricingDenomRe = regexp.MustCompile(`(?m)^\s*denom:\s*(\S+)`)

// TestLocalnetDeploymentLifecycle covers the path this CLI exists to serve,
// end to end on a real chain: publish a client certificate, create a
// deployment from a scaffold, observe the order the market opens for it,
// then close it and see the state change. Everything up to bid/lease is
// reachable without a provider; see the note at the close of this test for
// what is not.
func TestLocalnetDeploymentLifecycle(t *testing.T) {
	net := localnetRPC(t)
	requireLocalnetMutations(t, net)
	if !net.HarnessOwned {
		t.Skip("deployment lifecycle creates a persistent client certificate and is restricted to the harness-owned throwaway chain")
	}

	home := setupLocalnetHome(t, net)
	addr := validatorAddr(t, home)
	observer, err := nativeNodeObserverForLocalnet(net)
	if err != nil {
		t.Fatalf("construct native node observer: %v", err)
	}
	beforeCertificates := certificateSerialStates(t, home, addr)
	nativeBeforeCertificates, err := observer.certificateStates(t.Context(), addr)
	if err != nil {
		t.Fatalf("read native certificate baseline: %v", err)
	}
	if err := compareNativeCertificateStates(addr, beforeCertificates, nativeBeforeCertificates); err != nil {
		t.Fatalf("akt and native certificate baselines disagree: %v", err)
	}
	txHashes := make(map[string]string, 3)

	// A client certificate is a hard prerequisite: without one on disk,
	// `tx deployment create` fails before it ever builds the message.
	t.Run("CertGenerateAndPublish", func(t *testing.T) {
		mustRunAkt(t, home, "tx", "cert", "generate", "client", "--from", "validator", "--fees", "5000uakt", "--yes")
		stdout := mustRunAkt(t, home, "tx", "cert", "publish", "client", "--from", "validator", "--fees", "5000uakt", "--yes")
		publishHash := localnetTxHashFromPretty(t, stdout)
		publishReceipt, err := observer.waitForTransaction(t.Context(), publishHash)
		if err != nil {
			t.Fatalf("query native certificate transaction receipt: %v", err)
		}
		if publishReceipt.Hash != publishHash || publishReceipt.Height == 0 || publishReceipt.Code != 0 {
			t.Fatalf("native certificate receipt = %+v, want exact successful %s", publishReceipt, publishHash)
		}
		txHashes["cert.MsgCreateCertificate"] = publishHash

		var serial string
		if !pollUntil(45*time.Second, func() bool {
			after := certificateSerialStates(t, home, addr)
			nativeAfter, nativeErr := observer.certificateStates(t.Context(), addr)
			if nativeErr != nil || compareNativeCertificateStates(addr, after, nativeAfter) != nil {
				return false
			}
			var discoveryErr error
			serial, _, discoveryErr = uniqueNewResource(beforeCertificates, after)
			id := nativeNodeCertificateID{Owner: addr, Serial: serial}
			_, existedNatively := nativeBeforeCertificates[id]
			return discoveryErr == nil && serial != "" && !existedNatively && after[serial] == "valid" && nativeAfter[id] == "valid"
		}) {
			t.Fatal("akt and native observers did not agree on one newly valid certificate")
		}
	})

	// Dogfood the scaffold: the SDL under test is the one `akt sdl init`
	// hands a new user, so a scaffold that does not deploy fails here.
	sdl := filepath.Join(t.TempDir(), "deploy.yaml")
	sdlBody := mustRunAkt(t, home, "sdl", "init", "web")
	if err := os.WriteFile(sdl, []byte(sdlBody), 0o600); err != nil {
		t.Fatalf("failed to write generated SDL: %v", err)
	}

	m := sdlPricingDenomRe.FindStringSubmatch(sdlBody)
	if m == nil {
		t.Fatalf("no pricing denom in generated SDL:\n%s", sdlBody)
	}
	deposit := "5000000" + m[1]

	beforeDeployments := deploymentDSeqs(t, home, addr)
	nativeBeforeDeployments, err := observer.deploymentStates(t.Context(), addr)
	if err != nil {
		t.Fatalf("read native deployment baseline: %v", err)
	}
	if err := compareNativeDeploymentStates(addr, beforeDeployments, nativeBeforeDeployments); err != nil {
		t.Fatalf("akt and native deployment baselines disagree: %v", err)
	}
	var dseq string

	t.Run("DeploymentCreate", func(t *testing.T) {
		stdout, stderr, exit := runAkt(t, home, "tx", "deployment", "create", sdl,
			"--from", "validator", "--deposit", deposit, "--fees", "5000uakt", "--yes")
		if exit != 0 {
			t.Fatalf("tx deployment create failed (exit %d)\nstdout: %s\nstderr: %s", exit, stdout, stderr)
		}
		createHash := localnetTxHashFromPretty(t, stdout)
		createReceipt, err := observer.waitForTransaction(t.Context(), createHash)
		if err != nil {
			t.Fatalf("query native deployment-create receipt: %v", err)
		}
		if createReceipt.Hash != createHash || createReceipt.Height == 0 || createReceipt.Code != 0 {
			t.Fatalf("native deployment-create receipt = %+v, want exact successful %s", createReceipt, createHash)
		}
		txHashes["deployment.MsgCreateDeployment"] = createHash

		var discoveryErr error
		if !pollUntil(45*time.Second, func() bool {
			var found bool
			dseq, found, discoveryErr = uniqueNewDeployment(beforeDeployments, deploymentDSeqs(t, home, addr))
			return found || discoveryErr != nil
		}) {
			t.Fatal("created deployment identity never appeared")
		}
		if discoveryErr != nil {
			t.Fatalf("refusing to select a deployment after create: %v", discoveryErr)
		}
		if !pollUntil(45*time.Second, func() bool {
			aktAfter := deploymentDSeqs(t, home, addr)
			nativeAfter, nativeErr := observer.deploymentStates(t.Context(), addr)
			if nativeErr != nil || compareNativeDeploymentStates(addr, aktAfter, nativeAfter) != nil {
				return false
			}
			id := nativeNodeDeploymentID{Owner: addr, DSeq: dseq}
			_, existedNatively := nativeBeforeDeployments[id]
			return !existedNatively && aktAfter[dseq] == "active" && nativeAfter[id] == "active"
		}) {
			t.Fatalf("akt and native observers did not agree that deployment %s became active", dseq)
		}
	})

	t.Run("MarketOpensAnOrder", func(t *testing.T) {
		if dseq == "" {
			t.Skip("no deployment was created")
		}
		// Creating a deployment must open an order for its group. Without
		// a provider nothing will bid, but the order proves the market
		// module saw the deployment.
		if !pollUntil(45*time.Second, func() bool {
			out := stripANSI(mustRunAkt(t, home, "query", "market", "order", addr+"/"+dseq+"/1/1"))
			if !strings.Contains(out, dseq) || !strings.Contains(out, "open") {
				return false
			}
			id := nativeNodeOrderID{Owner: addr, DSeq: dseq, GSeq: 1, OSeq: 1}
			order, err := observer.order(t.Context(), id)
			return err == nil && order.ID == id && order.State == "open"
		}) {
			t.Fatal("market never opened an order for the deployment")
		}
	})

	t.Run("QueryGroupAndEscrow", func(t *testing.T) {
		if dseq == "" {
			t.Skip("no deployment was created")
		}
		group := stripANSI(mustRunAkt(t, home, "query", "deployment", "group", addr+"/"+dseq+"/1"))
		if !strings.Contains(group, dseq) {
			t.Fatalf("group query did not report dseq %s:\n%s", dseq, group)
		}
		groupID := nativeNodeGroupID{Owner: addr, DSeq: dseq, GSeq: 1}
		nativeGroup, err := observer.group(t.Context(), groupID)
		if err != nil || nativeGroup.ID != groupID || nativeGroup.State != "open" {
			t.Fatalf("native group query = %+v, %v; want exact open group %+v", nativeGroup, err, groupID)
		}
		// Escrow takes a scope-prefixed xid, not a bare address.
		mustRunAkt(t, home, "query", "escrow", "accounts", "open", "deployment/"+addr+"/"+dseq)
		escrow, err := observer.escrowAccount(t.Context(), addr, dseq)
		wantEscrowID := nativeNodeEscrowID{Scope: "deployment", XID: addr + "/" + dseq}
		if err != nil || escrow.ID != wantEscrowID || escrow.Owner != addr || escrow.State != "open" {
			t.Fatalf("native escrow query = %+v, %v; want exact open deployment escrow %+v", escrow, err, wantEscrowID)
		}
		if escrow.balance(m[1]).Cmp(big.NewRat(5_000_000, 1)) != 0 {
			t.Fatalf("native escrow %s balance = %s, want exact deposit 5000000", m[1], escrow.balance(m[1]))
		}
	})

	t.Run("DeploymentClosePositionalState", func(t *testing.T) {
		if dseq == "" {
			t.Skip("no deployment was created")
		}
		stdout := mustRunAkt(t, home, "tx", "deployment", "close", dseq,
			"--from", "validator", "--fees", "5000uakt", "--yes")
		closeHash := localnetTxHashFromPretty(t, stdout)
		closeReceipt, err := observer.waitForTransaction(t.Context(), closeHash)
		if err != nil {
			t.Fatalf("query native deployment-close receipt: %v", err)
		}
		if closeReceipt.Hash != closeHash || closeReceipt.Height == 0 || closeReceipt.Code != 0 {
			t.Fatalf("native deployment-close receipt = %+v, want exact successful %s", closeReceipt, closeHash)
		}
		txHashes["deployment.MsgCloseDeployment"] = closeHash

		// The positional state filter is a headline change of this CLI and
		// had no live coverage: `query deployment <owner> closed` must
		// filter server-side, not just parse.
		if !pollUntil(45*time.Second, func() bool {
			aktAfter := deploymentDSeqs(t, home, addr)
			nativeAfter, nativeErr := observer.deploymentStates(t.Context(), addr)
			return nativeErr == nil && compareNativeDeploymentStates(addr, aktAfter, nativeAfter) == nil &&
				aktAfter[dseq] == "closed" && nativeAfter[nativeNodeDeploymentID{Owner: addr, DSeq: dseq}] == "closed"
		}) {
			t.Fatal("deployment never reached the closed state")
		}
		closed := deploymentDSeqsByState(t, home, addr, "closed")
		if closed[dseq] != "closed" {
			t.Fatalf("positional closed filter omitted deployment %s: %v", dseq, closed)
		}
		if _, exists := deploymentDSeqsByState(t, home, addr, "active")[dseq]; exists {
			t.Fatalf("positional active filter still returned closed deployment %s", dseq)
		}
	})

	t.Run("ActionLogRecordsEveryMutation", func(t *testing.T) {
		var entries []localnetTxAction
		if !pollUntil(20*time.Second, func() bool {
			entries = localnetTxActions(t, home)
			if len(entries) != 3 {
				return false
			}
			for _, entry := range entries {
				if entry.Status != "success" {
					return false
				}
			}
			return true
		}) {
			t.Fatalf("transaction action log did not converge to three terminal entries: %+v", entries)
		}

		parsedDSeq, err := strconv.ParseUint(dseq, 10, 64)
		if err != nil {
			t.Fatalf("parse created dseq %q: %v", dseq, err)
		}
		type expectedAction struct {
			dseq uint64
			hash string
		}
		expected := map[string]expectedAction{
			"cert.MsgCreateCertificate":      {hash: txHashes["cert.MsgCreateCertificate"]},
			"deployment.MsgCreateDeployment": {dseq: parsedDSeq, hash: txHashes["deployment.MsgCreateDeployment"]},
			"deployment.MsgCloseDeployment":  {dseq: parsedDSeq, hash: txHashes["deployment.MsgCloseDeployment"]},
		}
		seen := make(map[string]struct{}, len(entries))
		for _, entry := range entries {
			want, ok := expected[entry.Action]
			if !ok {
				t.Errorf("unexpected transaction action %q", entry.Action)
				continue
			}
			if _, duplicate := seen[entry.Action]; duplicate {
				t.Errorf("duplicate transaction action %q", entry.Action)
			}
			seen[entry.Action] = struct{}{}
			receipt, err := observer.queryTransaction(t.Context(), want.hash)
			if err != nil {
				t.Errorf("query native transaction %s for action %q: %v", want.hash, entry.Action, err)
				continue
			}
			if entry.Type != "tx" || entry.Account != "validator" || entry.DSeq != want.dseq ||
				entry.Height != receipt.Height || entry.GasUsed <= 0 || entry.Status != "success" ||
				entry.Error != "" || entry.Code != receipt.Code || entry.Timestamp.IsZero() {
				t.Errorf("transaction action %q has incomplete semantics: %+v", entry.Action, entry)
			}
			decodedHash, err := hex.DecodeString(entry.TxHash)
			if err != nil || len(decodedHash) != 32 || !strings.EqualFold(entry.TxHash, want.hash) {
				t.Errorf("transaction action %q hash %q does not bind to native transaction %s", entry.Action, entry.TxHash, want.hash)
			}
		}
		if len(seen) != len(expected) {
			t.Errorf("transaction action set = %v, want %v", seen, expected)
		}
	})

	// Not covered here, and not coverable on a bare localnet: provider-side
	// bidding, lease selection, manifest delivery, workloads, or direct
	// `akt provider` gateway commands. They need a real provider and Kubernetes
	// environment. The live Console lane proves only its managed rail and is not
	// a substitute; SPEC §12.11 tracks the missing provider/Kubernetes lane.
}

// deploymentDSeqs returns every exact deployment identity owned by addr. The
// lifecycle snapshots this set before create so it never relies on list order
// or risks closing another actor's deployment.
func deploymentDSeqs(t *testing.T, home, addr string) map[string]string {
	t.Helper()
	return deploymentDSeqsByState(t, home, addr, "")
}

func deploymentDSeqsByState(t *testing.T, home, addr, state string) map[string]string {
	t.Helper()

	deployments := make(map[string]string)
	pageKey := ""
	seenPageKeys := make(map[string]struct{})
	for {
		args := []string{"query", "deployment", addr}
		if state != "" {
			args = append(args, state)
		}
		args = append(args, "--output", "json", "--limit", "100")
		if pageKey != "" {
			args = append(args, "--page-key", pageKey)
		}
		out := mustRunAkt(t, home, args...)

		var parsed struct {
			Deployments []struct {
				Deployment struct {
					State string `json:"state"`
					ID    struct {
						DSeq string `json:"dseq"`
					} `json:"id"`
				} `json:"deployment"`
			} `json:"deployments"`
			Pagination *struct {
				NextKey string `json:"next_key"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("could not parse deployment query JSON: %v\n%s", err, out)
		}
		if parsed.Pagination == nil {
			t.Fatal("deployment query omitted pagination metadata")
		}
		for _, item := range parsed.Deployments {
			dseq := item.Deployment.ID.DSeq
			if dseq == "" || item.Deployment.State == "" {
				t.Fatal("deployment query returned an entry without dseq or state")
			}
			if state != "" && item.Deployment.State != state {
				t.Fatalf("deployment %s returned state %q through the %q positional filter", dseq, item.Deployment.State, state)
			}
			if _, duplicate := deployments[dseq]; duplicate {
				t.Fatalf("deployment pagination returned duplicate dseq %s", dseq)
			}
			deployments[dseq] = item.Deployment.State
		}
		pageKey = parsed.Pagination.NextKey
		if pageKey == "" {
			return deployments
		}
		if _, repeated := seenPageKeys[pageKey]; repeated {
			t.Fatalf("deployment pagination repeated page key %q", pageKey)
		}
		seenPageKeys[pageKey] = struct{}{}
	}
}

type localnetTxAction struct {
	Timestamp time.Time `json:"ts"`
	Type      string    `json:"type"`
	Action    string    `json:"action"`
	TxHash    string    `json:"tx_hash"`
	Height    int64     `json:"height"`
	GasUsed   int64     `json:"gas_used"`
	Code      uint32    `json:"code"`
	DSeq      uint64    `json:"dseq"`
	Account   string    `json:"account"`
	Error     string    `json:"error"`
	Status    string    `json:"status"`
}

func localnetTxActions(t *testing.T, home string) []localnetTxAction {
	t.Helper()
	// Reading through the public command first performs pending-transaction
	// reconciliation. The assertion itself then reads the append-only JSONL
	// storage independently, so the production reader cannot validate its own
	// writer and collapse logic.
	out := stripANSI(mustRunAkt(t, home, "context", "log", "--type", "tx", "--output", "json"))
	var commandEntries []localnetTxAction
	if err := json.Unmarshal([]byte(out), &commandEntries); err != nil {
		t.Fatalf("decode transaction action log JSON: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(filepath.Join(home, "contexts", "localnet", "actions.log"))
	if err != nil {
		t.Fatalf("read raw transaction action log: %v", err)
	}
	rawEntries, err := decodeLocalnetRawTxActions(raw)
	if err != nil {
		t.Fatalf("decode raw transaction action log: %v", err)
	}
	if !reflect.DeepEqual(commandEntries, rawEntries) {
		t.Fatalf("context log output disagrees with independently decoded JSONL:\ncommand: %+v\nraw: %+v", commandEntries, rawEntries)
	}
	return rawEntries
}

func localnetActionLogCount(t *testing.T, home string) int {
	t.Helper()
	// Flush/reconcile through the CLI, then count physical JSONL records without
	// relying on the command's decoder.
	mustRunAkt(t, home, "context", "log", "--output", "json")
	raw, err := os.ReadFile(filepath.Join(home, "contexts", "localnet", "actions.log"))
	if err != nil {
		t.Fatalf("read complete raw action log: %v", err)
	}
	count, err := countLocalnetRawActionEntries(raw)
	if err != nil {
		t.Fatalf("decode complete raw action log: %v", err)
	}
	return count
}

func decodeLocalnetRawTxActions(data []byte) ([]localnetTxAction, error) {
	entries := make([]localnetTxAction, 0)
	indices := make(map[string]int)
	for lineNumber, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry localnetTxAction
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if entry.Type != "tx" {
			continue
		}
		if entry.TxHash != "" {
			if index, exists := indices[entry.TxHash]; exists {
				entry.Timestamp = entries[index].Timestamp
				entries[index] = entry
				continue
			}
			indices[entry.TxHash] = len(entries)
		}
		entries = append(entries, entry)
	}
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	return entries, nil
}

func countLocalnetRawActionEntries(data []byte) (int, error) {
	count := 0
	for lineNumber, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry json.RawMessage
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return 0, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if len(entry) == 0 || entry[0] != '{' {
			return 0, fmt.Errorf("line %d is not a JSON object", lineNumber+1)
		}
		count++
	}
	return count, nil
}

func certificateSerialStates(t *testing.T, home, addr string) map[string]string {
	t.Helper()

	certificates := make(map[string]string)
	pageKey := ""
	seenPageKeys := make(map[string]struct{})
	for {
		args := []string{"query", "cert", "list", addr, "--output", "json", "--limit", "100"}
		if pageKey != "" {
			args = append(args, "--page-key", pageKey)
		}
		out := mustRunAkt(t, home, args...)
		var parsed struct {
			Certificates []struct {
				Certificate struct {
					State string `json:"state"`
				} `json:"certificate"`
				Serial string `json:"serial"`
			} `json:"certificates"`
			Pagination *struct {
				NextKey string `json:"next_key"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("could not parse certificate query JSON: %v\n%s", err, out)
		}
		if parsed.Pagination == nil {
			t.Fatal("certificate query omitted pagination metadata")
		}
		for _, item := range parsed.Certificates {
			if item.Serial == "" || item.Certificate.State == "" {
				t.Fatal("certificate query returned an entry without serial or state")
			}
			if _, duplicate := certificates[item.Serial]; duplicate {
				t.Fatalf("certificate pagination returned duplicate serial %s", item.Serial)
			}
			certificates[item.Serial] = item.Certificate.State
		}
		pageKey = parsed.Pagination.NextKey
		if pageKey == "" {
			return certificates
		}
		if _, repeated := seenPageKeys[pageKey]; repeated {
			t.Fatalf("certificate pagination repeated page key %q", pageKey)
		}
		seenPageKeys[pageKey] = struct{}{}
	}
}

func localnetTxHashFromPretty(t *testing.T, output string) string {
	t.Helper()
	match := regexp.MustCompile(`(?m)^\s*Hash:\s*([0-9A-Fa-f]{64})\s*$`).FindStringSubmatch(stripANSI(output))
	if match == nil {
		t.Fatalf("transaction output did not report a hash:\n%s", output)
	}
	return strings.ToUpper(match[1])
}

func compareNativeCertificateStates(owner string, akt map[string]string, native map[nativeNodeCertificateID]string) error {
	if len(akt) != len(native) {
		return fmt.Errorf("akt returned %d certificates and native akash returned %d", len(akt), len(native))
	}
	for serial, state := range akt {
		if native[nativeNodeCertificateID{Owner: owner, Serial: serial}] != state {
			return fmt.Errorf("certificate %s state differs", serial)
		}
	}
	return nil
}

func compareNativeDeploymentStates(owner string, akt map[string]string, native map[nativeNodeDeploymentID]string) error {
	if len(akt) != len(native) {
		return fmt.Errorf("akt returned %d deployments and native akash returned %d", len(akt), len(native))
	}
	for dseq, state := range akt {
		if native[nativeNodeDeploymentID{Owner: owner, DSeq: dseq}] != state {
			return fmt.Errorf("deployment %s state differs", dseq)
		}
	}
	return nil
}

func uniqueNewResource(before, after map[string]string) (string, bool, error) {
	created := ""
	for id := range after {
		if _, existed := before[id]; existed {
			continue
		}
		if created != "" {
			return "", false, fmt.Errorf("multiple new resources appeared (%s and %s)", created, id)
		}
		created = id
	}
	return created, created != "", nil
}

func uniqueNewDeployment(before, after map[string]string) (string, bool, error) {
	created := ""
	for dseq := range after {
		if _, existed := before[dseq]; existed {
			continue
		}
		if created != "" {
			return "", false, fmt.Errorf("multiple new deployments appeared (%s and %s)", created, dseq)
		}
		created = dseq
	}
	return created, created != "", nil
}

func deploymentState(t *testing.T, home, addr, dseq string) string {
	t.Helper()
	return deploymentDSeqs(t, home, addr)[dseq]
}
