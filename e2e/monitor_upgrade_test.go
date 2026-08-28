package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	monitorrpc "pkg.akt.dev/akt/internal/monitor/rpc"
)

const monitorUpgradeChainID = "akt-monitor-upgrade"

const monitorClusterSetupScript = `set -eu
CHAIN_ID="` + monitorUpgradeChainID + `"
FUNDS="1000000000000uakt,1000000000000uact"
BASE=/cluster/validator0

for INDEX in 0 1 2; do
  HOME_DIR="/cluster/validator${INDEX}"
  akash --home "$HOME_DIR" genesis init "validator-${INDEX}" --chain-id "$CHAIN_ID" >/dev/null 2>&1
  akash --home "$HOME_DIR" keys add validator --keyring-backend test --output json > "$HOME_DIR/validator-key.json" 2>&1
done

for INDEX in 0 1 2; do
  HOME_DIR="/cluster/validator${INDEX}"
  ADDRESS=$(akash --home "$HOME_DIR" keys show validator -a --keyring-backend test)
  akash --home "$BASE" genesis add-account "$ADDRESS" "$FUNDS" --keyring-backend test
done

for INDEX in 1 2; do
  cp "$BASE/config/genesis.json" "/cluster/validator${INDEX}/config/genesis.json"
done

for SPEC in "0 60000000" "1 25000000" "2 15000000"; do
  set -- $SPEC
  INDEX=$1
  STAKE=$2
  HOME_DIR="/cluster/validator${INDEX}"
  akash --home "$HOME_DIR" genesis gentx validator "${STAKE}uakt" --chain-id "$CHAIN_ID" --keyring-backend test --min-self-delegation 1 >/dev/null 2>&1
  if [ "$INDEX" != "0" ]; then
    GENTX=$(find "$HOME_DIR/config/gentx" -name '*.json' -type f | head -n 1)
    cp "$GENTX" "$BASE/config/gentx/validator-${INDEX}.json"
  fi
done

akash --home "$BASE" genesis collect >/dev/null 2>&1
for INDEX in 1 2; do
  cp "$BASE/config/genesis.json" "/cluster/validator${INDEX}/config/genesis.json"
done

for INDEX in 0 1 2; do
  HOME_DIR="/cluster/validator${INDEX}"
  sed -i 's/^timeout_commit = ".*"/timeout_commit = "500ms"/' "$HOME_DIR/config/config.toml"
  akash --home "$HOME_DIR" comet show-node-id > "/cluster/node-id-${INDEX}"
done
`

const monitorClusterStartScript = `set -eu
HOME_DIR="/cluster/validator${INDEX}"
MARKER="$HOME_DIR/halt-complete"

start_node() {
  akash --home "$HOME_DIR" start \
    --rpc.laddr tcp://0.0.0.0:26657 \
    --p2p.laddr tcp://0.0.0.0:26656 \
    --p2p.persistent_peers "$PEERS" \
    --minimum-gas-prices 0uakt "$@"
}

if [ ! -f "$MARKER" ]; then
  set +e
  start_node --halt-height "$HALT_HEIGHT"
  STATUS=$?
  set -e
  touch "$MARKER"
  exit "$STATUS"
fi

exec akash --home "$HOME_DIR" start \
  --rpc.laddr tcp://0.0.0.0:26657 \
  --p2p.laddr tcp://0.0.0.0:26656 \
  --p2p.persistent_peers "$PEERS" \
  --minimum-gas-prices 0uakt
`

type monitorUpgradeCluster struct {
	rpc        string
	containers []string
}

func TestMonitorThreeValidatorUpgradeHaltAndRestart(t *testing.T) {
	if os.Getenv("AKT_E2E_MONITOR_UPGRADE") != "1" {
		t.Skip("set AKT_E2E_MONITOR_UPGRADE=1 to run the three-validator halt/restart scenario")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("Docker is required: %v", err)
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatalf("Docker daemon is not reachable: %v", err)
	}

	const haltHeight = int64(12)
	cluster := startMonitorUpgradeCluster(t, haltHeight)
	client := monitorrpc.NewClient(cluster.rpc, cluster.rpc)

	validators, err := client.GetValidators(t.Context())
	if err != nil {
		t.Fatalf("load monitor validator set: %v", err)
	}
	powers := make([]int, 0, len(validators))
	for _, validator := range validators {
		power, parseErr := strconv.Atoi(validator.VotingPower)
		if parseErr != nil {
			t.Fatalf("parse validator power %q: %v", validator.VotingPower, parseErr)
		}
		powers = append(powers, power)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(powers)))
	if got, want := fmt.Sprint(powers), "[60 25 15]"; got != want {
		t.Fatalf("validator powers = %s, want %s", got, want)
	}

	initial := subscribeMonitorConsensus(t, client, 15*time.Second)
	first := awaitMonitorSnapshot(t, initial, 15*time.Second, func(snapshot monitorrpc.ConsensusSnapshot) bool {
		return snapshot.State != nil && snapshot.State.TotalVotingPower == 100
	})
	if first.State.TotalValidators != 3 {
		t.Fatalf("initial monitor validator count = %d, want 3", first.State.TotalValidators)
	}
	// Keep consuming while the chain reaches its halt so the producer can
	// observe the socket closure instead of filling its snapshot buffer.
	go func() {
		for range initial.Snapshots {
		}
	}()

	completeMonitorUpgradeHalt(t, cluster.containers, haltHeight, 45*time.Second)
	awaitMonitorSubscriptionClose(t, initial, 10*time.Second)

	restartMonitorValidator(t, cluster.containers[0])
	waitForMonitorRPC(t, cluster.rpc, cluster.containers[0], 30*time.Second)
	stalledHeight, err := monitorChainHeight(cluster.rpc)
	if err != nil {
		t.Fatalf("read stalled chain height: %v", err)
	}

	minority := subscribeMonitorConsensus(t, client, 30*time.Second)
	minorityVote := awaitMonitorSnapshot(t, minority, 30*time.Second, func(snapshot monitorrpc.ConsensusSnapshot) bool {
		if snapshot.State == nil || snapshot.State.TotalVotingPower != 100 {
			return false
		}
		return snapshot.State.PrevotePower == 60 || snapshot.State.PrecommitPower == 60
	})
	if minorityVote.State.PrevotePower > 60 || minorityVote.State.PrecommitPower > 60 {
		t.Fatalf("60%% restart reported impossible vote power: prevote=%d precommit=%d",
			minorityVote.State.PrevotePower, minorityVote.State.PrecommitPower)
	}
	assertMonitorHeightStalled(t, cluster.rpc, stalledHeight, 2*time.Second)

	restartMonitorValidator(t, cluster.containers[1])
	quorum := awaitMonitorSnapshot(t, minority, 30*time.Second, func(snapshot monitorrpc.ConsensusSnapshot) bool {
		if snapshot.State == nil || snapshot.State.TotalVotingPower != 100 || snapshot.State.Height <= stalledHeight {
			return false
		}
		return snapshot.State.PrevotePower >= 85 || snapshot.State.PrecommitPower >= 85
	})
	if quorum.State.Height <= stalledHeight {
		t.Fatalf("85%% voting power did not resume blocks: stalled=%d observed=%d", stalledHeight, quorum.State.Height)
	}

	restartMonitorValidator(t, cluster.containers[2])
	all := awaitMonitorSnapshot(t, minority, 30*time.Second, func(snapshot monitorrpc.ConsensusSnapshot) bool {
		if snapshot.State == nil || snapshot.State.TotalVotingPower != 100 || snapshot.State.Height <= quorum.State.Height {
			return false
		}
		return snapshot.State.PrevotePower == 100 || snapshot.State.PrecommitPower == 100
	})
	if all.State.TotalValidators != 3 || all.State.TotalVotingPower != 100 {
		t.Fatalf("full restart changed monitor denominator: validators=%d power=%d",
			all.State.TotalValidators, all.State.TotalVotingPower)
	}
}

func startMonitorUpgradeCluster(t *testing.T, haltHeight int64) monitorUpgradeCluster {
	t.Helper()

	image := strings.TrimSpace(os.Getenv("AKT_E2E_NODE_IMAGE"))
	if image == "" {
		image = defaultNodeImage
	}
	prefix := fmt.Sprintf("akt-monitor-upgrade-%d-%d", os.Getpid(), time.Now().UnixNano())
	network := prefix + "-network"
	root := t.TempDir()
	setupPath := filepath.Join(root, "setup.sh")
	startPath := filepath.Join(root, "start.sh")
	clusterPath := filepath.Join(root, "cluster")
	if err := os.MkdirAll(clusterPath, 0o755); err != nil {
		t.Fatalf("create monitor cluster directory: %v", err)
	}
	if err := os.WriteFile(setupPath, []byte(monitorClusterSetupScript), 0o755); err != nil {
		t.Fatalf("write monitor setup script: %v", err)
	}
	if err := os.WriteFile(startPath, []byte(monitorClusterStartScript), 0o755); err != nil {
		t.Fatalf("write monitor start script: %v", err)
	}

	runDocker(t, "network", "create", network)
	containers := []string{prefix + "-validator0", prefix + "-validator1", prefix + "-validator2"}
	t.Cleanup(func() {
		args := append([]string{"rm", "-f"}, containers...)
		_ = exec.Command("docker", args...).Run()
		_ = exec.Command("docker", "network", "rm", network).Run()
	})

	runDocker(t, "run", "--rm",
		"-v", setupPath+":/setup.sh:ro",
		"-v", clusterPath+":/cluster",
		"--entrypoint", "/bin/sh",
		image, "/setup.sh",
	)

	nodeIDs := make([]string, 3)
	for index := range nodeIDs {
		data, err := os.ReadFile(filepath.Join(clusterPath, fmt.Sprintf("node-id-%d", index)))
		if err != nil {
			t.Fatalf("read validator %d node ID: %v", index, err)
		}
		nodeIDs[index] = strings.TrimSpace(string(data))
	}

	observerPort := freeMonitorTCPPort(t)
	for index, name := range containers {
		peers := make([]string, 0, 2)
		for peerIndex, peerName := range containers {
			if peerIndex != index {
				peers = append(peers, nodeIDs[peerIndex]+"@"+peerName+":26656")
			}
		}
		args := []string{
			"run", "-d", "--name", name,
			"--network", network,
		}
		if index == 0 {
			args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:26657", observerPort))
		}
		args = append(args,
			"-v", startPath+":/start.sh:ro",
			"-v", clusterPath+":/cluster",
			"-e", fmt.Sprintf("INDEX=%d", index),
			"-e", fmt.Sprintf("HALT_HEIGHT=%d", haltHeight),
			"-e", "PEERS="+strings.Join(peers, ","),
			"--entrypoint", "/bin/sh",
		)
		args = append(args, image, "/start.sh")
		runDocker(t, args...)
	}

	rpc := fmt.Sprintf("http://127.0.0.1:%d", observerPort)
	waitForChain(t, containers[0], rpc)
	return monitorUpgradeCluster{rpc: rpc, containers: containers}
}

func runDocker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func freeMonitorTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve observer RPC port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release observer RPC port reservation: %v", err)
	}
	return port
}

func subscribeMonitorConsensus(t *testing.T, client *monitorrpc.Client, timeout time.Duration) *monitorrpc.ConsensusSubscription {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithCancel(t.Context())
		subscription, err := client.SubscribeConsensusState(ctx)
		if err == nil {
			t.Cleanup(cancel)
			return subscription
		}
		cancel()
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("subscribe monitor consensus: %v", lastErr)
	return nil
}

func awaitMonitorSnapshot(
	t *testing.T,
	subscription *monitorrpc.ConsensusSubscription,
	timeout time.Duration,
	accept func(monitorrpc.ConsensusSnapshot) bool,
) monitorrpc.ConsensusSnapshot {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case snapshot, ok := <-subscription.Snapshots:
			if !ok {
				t.Fatal("monitor consensus subscription closed before the expected snapshot")
			}
			if accept(snapshot) {
				return snapshot
			}
		case <-timer.C:
			t.Fatal("timed out waiting for the expected monitor consensus snapshot")
		}
	}
}

func awaitMonitorSubscriptionClose(t *testing.T, subscription *monitorrpc.ConsensusSubscription, timeout time.Duration) {
	t.Helper()
	select {
	case <-subscription.Done():
	case <-time.After(timeout):
		t.Fatal("monitor consensus subscription did not observe the coordinated halt")
	}
}

func completeMonitorUpgradeHalt(t *testing.T, containers []string, haltHeight int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	haltMessage := fmt.Sprintf("halt per configuration height %d", haltHeight)
	for time.Now().Before(deadline) {
		allHalted := true
		for _, container := range containers {
			logs, err := exec.Command("docker", "logs", container).CombinedOutput()
			if err != nil || !strings.Contains(string(logs), haltMessage) {
				allHalted = false
				break
			}
		}
		if allHalted {
			// This node version halts its consensus engine but intentionally
			// leaves the RPC process alive. Mark the one-shot halt complete and
			// stop each process as an operator would before replacing a binary.
			for index, container := range containers {
				runDocker(t, "exec", container, "touch", fmt.Sprintf("/cluster/validator%d/halt-complete", index))
			}
			args := append([]string{"stop", "--time", "5"}, containers...)
			runDocker(t, args...)
			return
		}
		time.Sleep(250 * time.Millisecond)
	}

	for _, container := range containers {
		logs, _ := exec.Command("docker", "logs", container).CombinedOutput()
		t.Logf("%s logs:\n%s", container, logs)
	}
	t.Fatalf("validators did not all enter the configured halt at height %d", haltHeight)
}

func restartMonitorValidator(t *testing.T, container string) {
	t.Helper()
	runDocker(t, "restart", container)
}

func waitForMonitorRPC(t *testing.T, rpc, container string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := monitorChainHeight(rpc); err == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	state, _ := exec.Command("docker", "inspect", "--format", "{{json .State}}", container).CombinedOutput()
	logs, _ := exec.Command("docker", "logs", container).CombinedOutput()
	t.Fatalf("restarted observer validator did not restore RPC\nstate: %s\nlogs:\n%s", state, logs)
}

func monitorChainHeight(rpc string) (int64, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(rpc, "/") + "/status")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	var status struct {
		Result struct {
			SyncInfo struct {
				Height string `json:"latest_block_height"`
			} `json:"sync_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return 0, err
	}
	return strconv.ParseInt(status.Result.SyncInfo.Height, 10, 64)
}

func assertMonitorHeightStalled(t *testing.T, rpc string, want int64, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		height, err := monitorChainHeight(rpc)
		if err != nil {
			t.Fatalf("read minority height: %v", err)
		}
		if height != want {
			t.Fatalf("60%% voting power advanced height from %d to %d", want, height)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
