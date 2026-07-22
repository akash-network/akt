package e2e

// Gated localnet e2e tests: these run against a real single-validator Akash
// node and are skipped unless one of the following is set:
//
//   AKT_E2E_RPC=http://host:26657   use an existing node (optionally set
//                                   AKT_E2E_CHAIN_ID, and AKT_E2E_MNEMONIC to
//                                   a funded account's mnemonic to enable the
//                                   bank/tx subtests)
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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	defaultNodeImage     = "ghcr.io/akash-network/node:2.1.0"
	defaultLocalChainID  = "localakash"
	validatorGenesisUakt = "100000000000000uakt" // 100M AKT
)

// localnet describes the chain endpoint the gated tests talk to.
type localnet struct {
	RPC      string // http://127.0.0.1:<port>
	ChainID  string
	Mnemonic string // funded account mnemonic ("" if unknown — bank/tx subtests skip)
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
$AKASH genesis add-account "$ADDR" ` + validatorGenesisUakt + ` --keyring-backend test
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

	if rpc := os.Getenv("AKT_E2E_RPC"); rpc != "" {
		chainID := os.Getenv("AKT_E2E_CHAIN_ID")
		if chainID == "" {
			chainID = defaultLocalChainID
		}
		return &localnet{
			RPC:      rpc,
			ChainID:  chainID,
			Mnemonic: os.Getenv("AKT_E2E_MNEMONIC"),
		}
	}

	if os.Getenv("AKT_E2E_LOCALNET") != "1" {
		t.Skip("no localnet: set AKT_E2E_RPC or AKT_E2E_LOCALNET=1")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("AKT_E2E_LOCALNET=1 but docker is not installed")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("AKT_E2E_LOCALNET=1 but the docker daemon is not reachable")
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

	name := fmt.Sprintf("akt-e2e-localnet-%d", os.Getpid())

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

	return &localnet{RPC: rpc, ChainID: defaultLocalChainID, Mnemonic: key.Mnemonic}
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

// nonZeroDigitRe detects a non-zero balance in pretty output.
var nonZeroDigitRe = regexp.MustCompile(`[1-9]`)

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
		stdout := stripANSI(mustRunAkt(t, home, "query", "bank", "balances", addr))
		if !nonZeroDigitRe.MatchString(stdout) {
			t.Fatalf("expected genesis account %s to have a non-zero balance, got:\n%s", addr, stdout)
		}
	})

	t.Run("TxBankSendRecordsActionLog", func(t *testing.T) {
		if net.Mnemonic == "" {
			t.Skip("no funded mnemonic (set AKT_E2E_MNEMONIC when using AKT_E2E_RPC)")
		}

		mustRunAkt(t, home, "context", "keys", "add", "recipient", "--no-backup")
		recipient := strings.TrimSpace(stripANSI(mustRunAkt(t, home, "context", "keys", "show", "recipient", "-a")))

		// Default gas (auto): exercises gas simulation end to end.
		stdout, stderr, exitCode := runAkt(t, home,
			"tx", "bank", "send", "validator", recipient, "1000000uakt",
			"--fees", "5000uakt", "--yes")
		if exitCode != 0 {
			t.Fatalf("tx bank send failed (exit %d)\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
		}

		// Ground truth: poll the recipient balance until the transfer lands.
		deadline := time.Now().Add(45 * time.Second)
		funded := false
		var lastBalance string
		for time.Now().Before(deadline) {
			lastBalance = stripANSI(mustRunAkt(t, home, "query", "bank", "balances", recipient))
			if nonZeroDigitRe.MatchString(lastBalance) {
				funded = true
				break
			}
			time.Sleep(2 * time.Second)
		}
		if !funded {
			t.Fatalf("recipient %s never received funds; last balance output:\n%s\nsend output:\n%s%s",
				recipient, lastBalance, stdout, stderr)
		}

		// AKT-211 acceptance: the broadcast is recorded in the context
		// action log as a tx entry.
		logOut := stripANSI(mustRunAkt(t, home, "context", "log", "--type", "tx"))
		if !strings.Contains(logOut, "bank.MsgSend") {
			t.Fatalf("expected action log to contain a bank.MsgSend tx entry, got:\n%s", logOut)
		}
		if !strings.Contains(logOut, "success") {
			t.Fatalf("expected a successful tx entry in the action log, got:\n%s", logOut)
		}
	})
}
