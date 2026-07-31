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
	defaultNodeImage    = "ghcr.io/akash-network/node:2.1.1"
	defaultLocalChainID = "localakash"
	// Both denominations are funded. A deployment's deposit must match the
	// denom its SDL prices in, and the scaffolds `akt sdl init` emits price
	// in uact — an account holding only uakt cannot deploy them, which is
	// what the deployment lifecycle subtest exercises.
	validatorGenesisFunds = "100000000000000uakt,100000000000000uact" // 100M each
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

// TestLocalnetQueries exercises every module's read path against a live
// chain. The offline suite proves these commands parse their arguments;
// only a real node proves the query round-trips and renders — a query that
// builds a malformed request or mishandles an empty result set passes
// offline and fails here.
func TestLocalnetQueries(t *testing.T) {
	net := localnetRPC(t)
	home := setupLocalnetHome(t, net)

	// Queries needing no account. Chain-wide reads and every module's
	// params, which is the one read path every module is guaranteed to
	// have on a chain with no activity on it yet.
	// Every entry must be a runnable query, not a command group. A group
	// prints its help and exits 0 without reaching the chain, so it passes
	// this matrix no matter what the node does. `query params` is the
	// x/params group — its only subcommand, subspace, needs arguments — and
	// it is deliberately absent for that reason. The check for a new entry:
	// with the context pointed at a dead endpoint it must exit non-zero.
	queries := [][]string{
		{"query", "block"},
		{"query", "auth", "params"},
		{"query", "bank", "total"},
		{"query", "staking", "validators"},
		{"query", "staking", "params"},
		{"query", "staking", "pool"},
		{"query", "distribution", "params"},
		{"query", "slashing", "params"},
		{"query", "mint", "params"},
		{"query", "gov", "params"},
		// Akash modules — the reason this CLI exists, and previously
		// untested against a chain at any point in the suite.
		{"query", "deployment", "params"},
		{"query", "market", "params"},
		{"query", "provider", "list"},
		{"query", "audit", "list"},
		{"query", "bme", "params"},
		{"query", "oracle", "params"},
	}

	if net.Mnemonic != "" {
		addr := validatorAddr(t, home)
		// Account-scoped reads. These take the address positionally: the
		// `list` subcommand form does not exist, so a wrong invocation
		// here fails with a filter parse error rather than an empty table.
		queries = append(queries,
			[]string{"query", "bank", "balances", addr},
			[]string{"query", "auth", "account", addr},
			[]string{"query", "cert", "list", addr},
			[]string{"query", "deployment", addr},
			[]string{"query", "market", "order", addr},
			[]string{"query", "market", "bid", addr},
			[]string{"query", "market", "lease", addr},
		)
	}

	for _, q := range queries {
		t.Run(strings.Join(q[1:], " "), func(t *testing.T) {
			stdout, stderr, exit := runAkt(t, home, q...)
			if exit != 0 {
				t.Fatalf("akt %s exited %d\nstdout: %s\nstderr: %s",
					strings.Join(q, " "), exit, stdout, stderr)
			}
			if strings.TrimSpace(stripANSI(stdout)) == "" {
				t.Fatalf("akt %s produced no output", strings.Join(q, " "))
			}
		})
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
	if net.Mnemonic == "" {
		t.Skip("no funded mnemonic (set AKT_E2E_MNEMONIC when using AKT_E2E_RPC)")
	}

	home := setupLocalnetHome(t, net)
	addr := validatorAddr(t, home)

	// A client certificate is a hard prerequisite: without one on disk,
	// `tx deployment create` fails before it ever builds the message.
	t.Run("CertGenerateAndPublish", func(t *testing.T) {
		mustRunAkt(t, home, "tx", "cert", "generate", "client", "--from", "validator", "--fees", "5000uakt", "--yes")
		mustRunAkt(t, home, "tx", "cert", "publish", "client", "--from", "validator", "--fees", "5000uakt", "--yes")

		if !pollUntil(45*time.Second, func() bool {
			return strings.Contains(stripANSI(mustRunAkt(t, home, "query", "cert", "list", addr)), "valid")
		}) {
			t.Fatal("published certificate never became valid on chain")
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

	var dseq string

	t.Run("DeploymentCreate", func(t *testing.T) {
		stdout, stderr, exit := runAkt(t, home, "tx", "deployment", "create", sdl,
			"--from", "validator", "--deposit", deposit, "--fees", "5000uakt", "--yes")
		if exit != 0 {
			t.Fatalf("tx deployment create failed (exit %d)\nstdout: %s\nstderr: %s", exit, stdout, stderr)
		}

		if !pollUntil(45*time.Second, func() bool {
			out := stripANSI(mustRunAkt(t, home, "query", "deployment", addr))
			return strings.Contains(out, "active")
		}) {
			t.Fatal("deployment never became active")
		}

		dseq = deploymentDSeq(t, home, addr)
		if dseq == "" {
			t.Fatal("could not determine dseq of the created deployment")
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
			return strings.Contains(stripANSI(mustRunAkt(t, home, "query", "market", "order", addr)), "open")
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
		// Escrow takes a scope-prefixed xid, not a bare address.
		mustRunAkt(t, home, "query", "escrow", "accounts", "open", "deployment/"+addr+"/"+dseq)
	})

	t.Run("DeploymentClosePositionalState", func(t *testing.T) {
		if dseq == "" {
			t.Skip("no deployment was created")
		}
		mustRunAkt(t, home, "tx", "deployment", "close", dseq,
			"--from", "validator", "--fees", "5000uakt", "--yes")

		// The positional state filter is a headline change of this CLI and
		// had no live coverage: `query deployment <owner> closed` must
		// filter server-side, not just parse.
		if !pollUntil(45*time.Second, func() bool {
			return strings.Contains(stripANSI(mustRunAkt(t, home, "query", "deployment", addr, "closed")), "closed")
		}) {
			t.Fatal("deployment never reached the closed state")
		}
	})

	t.Run("ActionLogRecordsEveryMutation", func(t *testing.T) {
		logOut := stripANSI(mustRunAkt(t, home, "context", "log", "--type", "tx"))
		for _, want := range []string{"cert", "deployment"} {
			if !strings.Contains(logOut, want) {
				t.Errorf("action log missing a %q tx entry:\n%s", want, logOut)
			}
		}
	})

	// Not covered here, and not coverable on a bare localnet: bid, lease,
	// manifest send, and every `akt provider` gateway command. All of them
	// need a provider bidding on the order, which a single-validator node
	// has none of. They are exercised against the live Console rail in
	// TestConsoleLive and by hand against mainnet.
}

// deploymentDSeq returns the dseq of the owner's most recent deployment by
// parsing JSON output, so the test never scrapes a rendered table.
func deploymentDSeq(t *testing.T, home, addr string) string {
	t.Helper()

	out := mustRunAkt(t, home, "query", "deployment", addr, "--output", "json")

	var parsed struct {
		Deployments []struct {
			Deployment struct {
				ID struct {
					DSeq string `json:"dseq"`
				} `json:"id"`
			} `json:"deployment"`
		} `json:"deployments"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("could not parse deployment query JSON: %v\n%s", err, out)
	}
	if len(parsed.Deployments) == 0 {
		return ""
	}

	return parsed.Deployments[len(parsed.Deployments)-1].Deployment.ID.DSeq
}
