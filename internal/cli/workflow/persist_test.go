package workflow

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
	wf "pkg.akt.dev/akt/internal/workflow"
)

const persistOwner = "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu"

func openPersistStore(t *testing.T) sstore.Store {
	t.Helper()

	s, err := bbolt.OpenContext(context.Background(), t.TempDir(), "prod")
	if err != nil {
		t.Fatalf("OpenContext: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	return s
}

// deployRunState builds the run state a successful chain-rail `akt deploy`
// leaves behind, matching the outputs the adapters actually produce.
func deployRunState(t *testing.T, sdlPath string) *wf.RunState {
	t.Helper()

	state := wf.NewRunState("run-1", "deploy", persistOwner, map[string]any{
		"sdl-file": sdlPath,
		"deposit":  "auto",
	})
	state.SetStepResult("create-deployment", &wf.StepResult{
		Name:   "create-deployment",
		Status: "success",
		TxHash: "CREATEHASH",
		Height: 4649141,
		Output: map[string]any{"dseq": "4649141", "owner": persistOwner},
	})
	state.SetStepResult("wait-for-bids", &wf.StepResult{
		Name:   "wait-for-bids",
		Status: "success",
		Output: map[string]any{"bids": []any{
			map[string]any{"bid": map[string]any{
				"id": map[string]any{
					"owner": persistOwner, "dseq": "4649141",
					"gseq": float64(1), "oseq": float64(1), "provider": "akash1cheap",
				},
				"price": map[string]any{"denom": "uakt", "amount": "10"},
			}},
			map[string]any{"bid": map[string]any{
				"id": map[string]any{
					"owner": persistOwner, "dseq": "4649141",
					"gseq": float64(1), "oseq": float64(1), "provider": "akash1exp",
				},
				"price": map[string]any{"denom": "uakt", "amount": "25"},
			}},
		}, "provider_metadata": map[string]any{
			"akash1cheap": map[string]any{
				"attributes": map[string]any{"region": "us-west"},
				"audited":    true,
			},
			"akash1exp": map[string]any{
				"attributes": map[string]any{},
				"audited":    false,
			},
		}},
	})
	state.SetStepResult("select-bid", &wf.StepResult{
		Name:   "select-bid",
		Status: "success",
		Output: map[string]any{
			"owner": persistOwner, "dseq": "4649141", "gseq": "1", "oseq": "1",
			"provider": "akash1cheap", "price": "10uakt",
		},
	})
	state.SetStepResult("create-lease", &wf.StepResult{
		Name:   "create-lease",
		Status: "success",
		TxHash: "LEASEHASH",
		Height: 4649150,
		Output: map[string]any{
			"dseq": "4649141", "gseq": "1", "oseq": "1",
			"provider": "akash1cheap", "owner": persistOwner,
		},
	})
	state.SetStepResult("send-manifest", &wf.StepResult{Name: "send-manifest", Status: "success"})

	return state
}

func writeSDL(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "d1.yaml")
	if err := os.WriteFile(path, []byte("version: \"2.0\"\n"), 0o600); err != nil {
		t.Fatalf("write sdl: %v", err)
	}

	return path
}

// TestPersistDeployRecordsWhatTheRunObserved is the regression guard on the
// reported bug: a successful `akt deploy` left `akt store status` reporting
// zero deployments, zero leases, and zero bids.
func TestPersistDeployRecordsWhatTheRunObserved(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)
	sdlPath := writeSDL(t)

	if err := persistWorkflowOutcome(ctx, s, deployRunState(t, sdlPath), 1700000000); err != nil {
		t.Fatalf("persistWorkflowOutcome: %v", err)
	}

	dep, err := s.GetDeployment(ctx, persistOwner, 4649141)
	if err != nil {
		t.Fatalf("GetDeployment: %v", err)
	}
	if dep == nil {
		t.Fatal("a completed deploy recorded no deployment")
		return
	}
	if dep.State != "active" {
		t.Errorf("state = %q, want active", dep.State)
	}
	if dep.CreatedHeight != 4649141 {
		t.Errorf("created height = %d, want the create-deployment tx height", dep.CreatedHeight)
	}
	if dep.CreatedAt != 1700000000 || dep.UpdatedAt != 1700000000 {
		t.Errorf("timestamps = %d/%d, want the supplied now", dep.CreatedAt, dep.UpdatedAt)
	}
	if dep.SDLPath != sdlPath {
		t.Errorf("sdl path = %q, want %q", dep.SDLPath, sdlPath)
	}
	if !strings.HasPrefix(dep.SDLHash, "sha256:") || len(dep.SDLHash) != len("sha256:")+64 {
		t.Errorf("sdl hash = %q, want sha256:<64 hex>", dep.SDLHash)
	}
	// "auto" is a request for the chain minimum, not an amount.
	if dep.Deposit != "" {
		t.Errorf("deposit = %q, want empty for an auto deposit", dep.Deposit)
	}
	// Fields the run cannot observe stay zero rather than being guessed.
	if dep.EscrowBalance != "" || dep.Transferred != "" {
		t.Errorf("unobservable escrow figures were invented: %q/%q", dep.EscrowBalance, dep.Transferred)
	}

	leases, err := s.ListLeases(ctx, sstore.LeaseFilter{Owner: persistOwner, DSeq: 4649141})
	if err != nil {
		t.Fatalf("ListLeases: %v", err)
	}
	if len(leases) != 1 {
		t.Fatalf("leases = %d, want the one that was won", len(leases))
	}
	lease := leases[0]
	if lease.ID.Provider != "akash1cheap" || lease.ID.GSeq != 1 || lease.ID.OSeq != 1 {
		t.Errorf("lease identity = %+v", lease.ID)
	}
	if lease.State != "active" {
		t.Errorf("lease state = %q, want active", lease.State)
	}
	if lease.Price == "" {
		t.Error("lease price was not recorded from the selected bid")
	}
	if lease.ProviderURI != "" || len(lease.Endpoints) != 0 {
		t.Errorf("unobservable provider details were invented: %q %+v", lease.ProviderURI, lease.Endpoints)
	}

	bids, err := s.ListBids(ctx, sstore.BidFilter{Owner: persistOwner, DSeq: 4649141})
	if err != nil {
		t.Fatalf("ListBids: %v", err)
	}
	if len(bids) != 2 {
		t.Fatalf("bids = %d, want every bid seen", len(bids))
	}

	states := map[string]string{}
	for _, b := range bids {
		states[b.ID.Provider] = b.State
		if b.Price == "" {
			t.Errorf("bid from %s has no price", b.ID.Provider)
		}
	}
	for _, b := range bids {
		switch b.ID.Provider {
		case "akash1cheap":
			if b.ProviderAttributes["region"] != "us-west" || !b.ProviderAudited {
				t.Errorf("winning provider metadata = %#v audited=%v", b.ProviderAttributes, b.ProviderAudited)
			}
		case "akash1exp":
			if b.ProviderAttributes == nil || b.ProviderAudited {
				t.Errorf("losing provider metadata = %#v audited=%v", b.ProviderAttributes, b.ProviderAudited)
			}
		}
	}
	if states["akash1cheap"] != "matched" {
		t.Errorf("winning bid state = %q, want matched", states["akash1cheap"])
	}
	if states["akash1exp"] != "lost" {
		t.Errorf("losing bid state = %q, want lost", states["akash1exp"])
	}
}

// TestPersistDeployRecordsPaidPartialState covers the failure the user is most
// likely to hit: the deployment exists on chain but a later step failed. The
// DSEQ the recovery advice tells them to close has to be in the store.
func TestPersistDeployRecordsPaidPartialState(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)

	state := deployRunState(t, writeSDL(t))
	state.Steps["send-manifest"] = &wf.StepResult{
		Name:   "send-manifest",
		Status: "failed",
		Error:  "gateway timeout",
	}

	if err := persistWorkflowOutcome(ctx, s, state, 1700000000); err != nil {
		t.Fatalf("persistWorkflowOutcome: %v", err)
	}

	dep, err := s.GetDeployment(ctx, persistOwner, 4649141)
	if err != nil || dep == nil {
		t.Fatalf("a paid partial deployment was not recorded: %v", err)
	}
}

// TestPersistDeploySkipsFailedCreate proves nothing is written when nothing
// reached the chain: a phantom record would be worse than an empty store.
func TestPersistDeploySkipsFailedCreate(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)

	state := wf.NewRunState("run-1", "deploy", persistOwner, nil)
	state.SetStepResult("create-deployment", &wf.StepResult{
		Name:   "create-deployment",
		Status: "failed",
		Error:  "insufficient funds",
	})

	if err := persistWorkflowOutcome(ctx, s, state, 1700000000); err != nil {
		t.Fatalf("persistWorkflowOutcome: %v", err)
	}

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Deployments != 0 || stats.Leases != 0 || stats.Bids != 0 {
		t.Errorf("a failed create wrote records: %+v", stats)
	}
}

// TestPersistDeployFallsBackToMarketOwner covers the console rail, whose
// deployment response carries no owner: the bid identity returned by the
// market supplies it, so console deploys are recorded too.
func TestPersistDeployFallsBackToMarketOwner(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)

	state := deployRunState(t, writeSDL(t))
	state.Account = "" // console contexts need no keyring account
	state.Steps["create-deployment"].Output = map[string]any{"dseq": "4649141"}

	if err := persistWorkflowOutcome(ctx, s, state, 1700000000); err != nil {
		t.Fatalf("persistWorkflowOutcome: %v", err)
	}

	dep, err := s.GetDeployment(ctx, persistOwner, 4649141)
	if err != nil || dep == nil {
		t.Fatalf("console-shaped deploy was not recorded: %v", err)
	}
}

// TestPersistDeployRefusesToWriteWithoutAnOwner pins the honest-skip rule of
// SPEC §6.6. Records are keyed <owner>:<dseq>; a record written under an empty
// owner is unfindable and corrupts the key space, so the run is reported as
// unrecorded instead.
func TestPersistDeployRefusesToWriteWithoutAnOwner(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)

	state := deployRunState(t, writeSDL(t))
	// A keyring key *name*, which is not an address and must not be used as one.
	state.Account = "alice"
	state.Steps["create-deployment"].Output = map[string]any{"dseq": "4649141"}
	delete(state.Steps["select-bid"].Output, "owner")
	delete(state.Steps["create-lease"].Output, "owner")
	for _, item := range state.Steps["wait-for-bids"].Output["bids"].([]any) {
		delete(item.(map[string]any)["bid"].(map[string]any)["id"].(map[string]any), "owner")
	}

	err := persistWorkflowOutcome(ctx, s, state, 1700000000)
	if err == nil {
		t.Fatal("an ownerless run was recorded instead of being reported")
	}
	if !strings.Contains(err.Error(), "akt store sync") {
		t.Errorf("the warning does not name the fix: %v", err)
	}

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Deployments != 0 || stats.Leases != 0 || stats.Bids != 0 {
		t.Errorf("records were written under an empty owner: %+v", stats)
	}
}

// TestPersistCloseClosesTheDeploymentAndItsLeases covers the close workflow's
// state transition, including the leases that die with the deployment.
func TestPersistCloseClosesTheDeploymentAndItsLeases(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)
	sdlPath := writeSDL(t)

	if err := persistWorkflowOutcome(ctx, s, deployRunState(t, sdlPath), 1700000000); err != nil {
		t.Fatalf("seed deploy: %v", err)
	}

	state := wf.NewRunState("run-2", "close", persistOwner, map[string]any{"dseq": 4649141})
	state.SetStepResult("close-deployment", &wf.StepResult{
		Name:   "close-deployment",
		Status: "success",
		TxHash: "CLOSEHASH",
		Output: map[string]any{"dseq": "4649141", "owner": persistOwner},
	})

	if err := persistWorkflowOutcome(ctx, s, state, 1700009999); err != nil {
		t.Fatalf("persistWorkflowOutcome: %v", err)
	}

	dep, err := s.GetDeployment(ctx, persistOwner, 4649141)
	if err != nil || dep == nil {
		t.Fatalf("GetDeployment: %v", err)
	}
	if dep.State != "closed" || dep.ClosedAt != 1700009999 {
		t.Errorf("deployment = %q closed_at %d, want closed at the run time", dep.State, dep.ClosedAt)
	}
	// The SDL identity recorded by the deploy survives the close.
	if dep.SDLPath != sdlPath {
		t.Errorf("sdl path lost on close: %q", dep.SDLPath)
	}

	leases, err := s.ListLeases(ctx, sstore.LeaseFilter{Owner: persistOwner, DSeq: 4649141})
	if err != nil {
		t.Fatalf("ListLeases: %v", err)
	}
	for _, l := range leases {
		if l.State != "closed" || l.ClosedAt == 0 {
			t.Errorf("lease %+v survived the deployment close", l)
		}
	}
}

func TestPersistConsoleCloseInfersOwnerFromUniqueStoredDSeq(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)
	if err := persistWorkflowOutcome(ctx, s, deployRunState(t, writeSDL(t)), 1700000000); err != nil {
		t.Fatalf("seed deploy: %v", err)
	}

	state := wf.NewRunState("run-console-close", "close", "", map[string]any{"dseq": 4649141})
	state.SetStepResult("close-deployment", &wf.StepResult{
		Name:   "close-deployment",
		Status: "success",
		Output: map[string]any{"dseq": "4649141"},
	})

	if err := persistWorkflowOutcome(ctx, s, state, 1700010000); err != nil {
		t.Fatalf("ownerless Console close was not matched to its local deployment: %v", err)
	}

	dep, err := s.GetDeployment(ctx, persistOwner, 4649141)
	if err != nil || dep == nil || dep.State != "closed" {
		t.Fatalf("deployment after Console close = %+v, err %v", dep, err)
	}
	leases, err := s.ListLeases(ctx, sstore.LeaseFilter{Owner: persistOwner, DSeq: 4649141})
	if err != nil {
		t.Fatalf("ListLeases: %v", err)
	}
	for _, lease := range leases {
		if lease.State != "closed" {
			t.Errorf("lease remained %q after Console close: %+v", lease.State, lease.ID)
		}
	}
}

func TestPersistConsoleCloseRefusesAmbiguousStoredDSeq(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)
	for _, owner := range []string{persistOwner, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"} {
		if err := s.PutDeployment(ctx, &sstore.DeploymentRecord{Owner: owner, DSeq: 42, State: "active"}); err != nil {
			t.Fatalf("seed deployment: %v", err)
		}
	}

	state := wf.NewRunState("run-console-close", "close", "", map[string]any{"dseq": 42})
	state.SetStepResult("close-deployment", &wf.StepResult{
		Name:   "close-deployment",
		Status: "success",
		Output: map[string]any{"dseq": "42"},
	})

	err := persistWorkflowOutcome(ctx, s, state, 1700010000)
	if err == nil || !strings.Contains(err.Error(), "multiple local owners") {
		t.Fatalf("ambiguous close error = %v", err)
	}
	for _, owner := range []string{persistOwner, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"} {
		dep, getErr := s.GetDeployment(ctx, owner, 42)
		if getErr != nil || dep == nil || dep.State != "active" {
			t.Fatalf("ambiguous close changed owner %s: %+v, err %v", owner, dep, getErr)
		}
	}
}

// TestPersistCloseRecordsUnknownDeployments covers closing a deployment this
// store never saw (created elsewhere): recording the closure beats leaving it
// invisible.
func TestPersistCloseRecordsUnknownDeployments(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)

	state := wf.NewRunState("run-2", "close", persistOwner, map[string]any{"dseq": 777})
	state.SetStepResult("close-deployment", &wf.StepResult{
		Name:   "close-deployment",
		Status: "success",
		// The dseq comes from the run parameter, not the output.
		Output: map[string]any{"owner": persistOwner},
	})

	if err := persistWorkflowOutcome(ctx, s, state, 1700009999); err != nil {
		t.Fatalf("persistWorkflowOutcome: %v", err)
	}

	dep, err := s.GetDeployment(ctx, persistOwner, 777)
	if err != nil || dep == nil {
		t.Fatalf("close of an unknown deployment was not recorded: %v", err)
	}
	if dep.State != "closed" {
		t.Errorf("state = %q, want closed", dep.State)
	}
}

// TestPersistUpdateRefreshesTheSDLIdentity covers the update workflow: the
// stored SDL path and hash must follow the deployment, or the store keeps
// claiming the deployment runs a file it no longer runs.
func TestPersistUpdateRefreshesTheSDLIdentity(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)

	if err := persistWorkflowOutcome(ctx, s, deployRunState(t, writeSDL(t)), 1700000000); err != nil {
		t.Fatalf("seed deploy: %v", err)
	}

	before, err := s.GetDeployment(ctx, persistOwner, 4649141)
	if err != nil || before == nil {
		t.Fatalf("seed deployment missing: %v", err)
	}

	newSDL := filepath.Join(t.TempDir(), "d2.yaml")
	if err := os.WriteFile(newSDL, []byte("version: \"2.0\"\n# revised\n"), 0o600); err != nil {
		t.Fatalf("write sdl: %v", err)
	}

	state := wf.NewRunState("run-3", "update", persistOwner, map[string]any{
		"sdl-file": newSDL,
		"dseq":     4649141,
	})
	state.SetStepResult("update-deployment", &wf.StepResult{
		Name:   "update-deployment",
		Status: "success",
		TxHash: "UPDATEHASH",
		Height: 4649200,
		Output: map[string]any{"dseq": "4649141", "owner": persistOwner},
	})
	state.SetStepResult("send-manifest", &wf.StepResult{Name: "send-manifest", Status: "success"})

	if err := persistWorkflowOutcome(ctx, s, state, 1700005000); err != nil {
		t.Fatalf("persistWorkflowOutcome: %v", err)
	}

	after, err := s.GetDeployment(ctx, persistOwner, 4649141)
	if err != nil || after == nil {
		t.Fatalf("GetDeployment: %v", err)
	}
	if after.SDLPath != newSDL {
		t.Errorf("sdl path = %q, want the updated file", after.SDLPath)
	}
	if after.SDLHash == before.SDLHash {
		t.Error("sdl hash still names the previous SDL")
	}
	if after.UpdatedAt != 1700005000 {
		t.Errorf("updated at = %d, want the update run time", after.UpdatedAt)
	}
	// The creation facts recorded by the deploy are not rewritten.
	if after.CreatedHeight != before.CreatedHeight || after.CreatedAt != before.CreatedAt {
		t.Errorf("update rewrote creation facts: %+v vs %+v", after, before)
	}
}

// TestPersistIgnoresUnknownWorkflows keeps user-defined workflows out of the
// store: their steps have no defined mapping onto the record types, and
// guessing one would write nonsense.
func TestPersistIgnoresUnknownWorkflows(t *testing.T) {
	ctx := context.Background()
	s := openPersistStore(t)

	state := wf.NewRunState("run-4", "my-custom-flow", persistOwner, nil)
	state.SetStepResult("create-deployment", &wf.StepResult{
		Name:   "create-deployment",
		Status: "success",
		Output: map[string]any{"dseq": "999", "owner": persistOwner},
	})

	if err := persistWorkflowOutcome(ctx, s, state, 1700000000); err != nil {
		t.Fatalf("persistWorkflowOutcome: %v", err)
	}

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Deployments != 0 {
		t.Errorf("an unknown workflow wrote records: %+v", stats)
	}
}

// TestRecordWorkflowOutcomeWarnsWithoutFailing is the core best-effort
// guarantee of SPEC §6.6: the deployment is already real on chain, so a store
// that cannot even be opened must produce a warning and nothing else.
func TestRecordWorkflowOutcomeWarnsWithoutFailing(t *testing.T) {
	root := t.TempDir()

	// Occupy the store directory path with a regular file so opening the
	// database cannot succeed.
	storeDir := aktctx.StoreDir(root, "prod")
	if err := os.MkdirAll(filepath.Dir(storeDir), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(storeDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	var stderr bytes.Buffer
	cmd := &cobra.Command{Use: "deploy"}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	cmd.SetContext(context.Background())

	rc := &aktctx.Context{Name: "prod", Root: root}

	recordWorkflowOutcome(cmd, rc, deployRunState(t, writeSDL(t)))

	if !strings.Contains(stderr.String(), "warning:") {
		t.Fatalf("an unwritable store produced no warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "local store was not updated") {
		t.Errorf("warning does not say what failed: %q", stderr.String())
	}
}

// TestRecordWorkflowOutcomeRespectsQuiet keeps -q meaning what it says:
// informational output is suppressed, and a bookkeeping warning is
// informational.
func TestRecordWorkflowOutcomeRespectsQuiet(t *testing.T) {
	root := t.TempDir()
	storeDir := aktctx.StoreDir(root, "prod")
	if err := os.MkdirAll(filepath.Dir(storeDir), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(storeDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	var stderr bytes.Buffer
	cmd := &cobra.Command{Use: "deploy"}
	cmd.Flags().BoolP(flagdefs.FlagQuiet, "q", true, "")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	cmd.SetContext(context.Background())

	recordWorkflowOutcome(cmd, &aktctx.Context{Name: "prod", Root: root}, deployRunState(t, writeSDL(t)))

	if stderr.String() != "" {
		t.Errorf("quiet mode still printed: %q", stderr.String())
	}
}

// TestRecordWorkflowOutcomeSurvivesACancelledRun covers the Ctrl-C path: the
// transactions that already landed still have to be recorded, so the store
// write must not inherit the run's cancellation.
func TestRecordWorkflowOutcomeSurvivesACancelledRun(t *testing.T) {
	root := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stderr bytes.Buffer
	cmd := &cobra.Command{Use: "deploy"}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	cmd.SetContext(ctx)

	recordWorkflowOutcome(cmd, &aktctx.Context{Name: "prod", Root: root}, deployRunState(t, writeSDL(t)))

	if stderr.String() != "" {
		t.Fatalf("unexpected warning: %q", stderr.String())
	}

	s, err := bbolt.OpenContext(context.Background(), root, "prod")
	if err != nil {
		t.Fatalf("OpenContext: %v", err)
	}
	defer func() { _ = s.Close() }()

	dep, err := s.GetDeployment(context.Background(), persistOwner, 4649141)
	if err != nil || dep == nil {
		t.Fatalf("a cancelled run lost its already-broadcast deployment: %v", err)
	}
}

// TestRecordWorkflowOutcomeNeedsAContext covers the guards: without a resolved
// context there is no store to write to, and reaching for one would panic.
func TestRecordWorkflowOutcomeNeedsAContext(t *testing.T) {
	var stderr bytes.Buffer
	cmd := &cobra.Command{Use: "deploy"}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	cmd.SetContext(context.Background())

	recordWorkflowOutcome(cmd, nil, deployRunState(t, writeSDL(t)))
	recordWorkflowOutcome(cmd, &aktctx.Context{Name: "prod"}, deployRunState(t, writeSDL(t)))
	recordWorkflowOutcome(cmd, &aktctx.Context{Root: t.TempDir()}, deployRunState(t, writeSDL(t)))
	recordWorkflowOutcome(cmd, &aktctx.Context{Name: "prod", Root: t.TempDir()}, nil)

	if stderr.String() != "" {
		t.Errorf("a missing context should be silent, got %q", stderr.String())
	}
}
