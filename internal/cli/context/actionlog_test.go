package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"

	"pkg.akt.dev/akt/internal/actionlog"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func newTestManager(t *testing.T) *aktctx.Manager {
	t.Helper()

	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	// DefaultConfig already seeds the "default" keyring.
	return m
}

func readLog(t *testing.T, root, ctxName string) []actionlog.Entry {
	t.Helper()

	l, err := actionlog.Open(aktctx.ActionLogPath(root, ctxName))
	if err != nil {
		t.Fatalf("open action log for %s: %v", ctxName, err)
	}
	defer l.Close()

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read action log for %s: %v", ctxName, err)
	}

	return entries
}

func runCmd(t *testing.T, cmd interface {
	SetArgs([]string)
	Execute() error
}, args ...string) {
	t.Helper()

	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
}

func TestCreateAndSwitchRecordedInActionLog(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runCmd(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	entries := readLog(t, m.Root(), "prod")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (create + switch), got %d: %+v", len(entries), entries)
	}

	// Newest first: switch, then create.
	if entries[0].Type != actionlog.TypeContext || entries[0].Action != "switch" {
		t.Errorf("entry[0] = %s/%s, want context/switch", entries[0].Type, entries[0].Action)
	}
	if entries[1].Action != "create" {
		t.Errorf("entry[1] action = %s, want create", entries[1].Action)
	}
}

func TestUseRecordedInActionLog(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runCmd(t, createCmd(mgrFn), "prod", "--network", "mainnet")
	runCmd(t, useCmd(mgrFn), "prod")

	entries := readLog(t, m.Root(), "prod")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Action != "switch" {
		t.Errorf("newest action = %s, want switch", entries[0].Action)
	}
}

func TestEditRecordedInActionLog(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runCmd(t, createCmd(mgrFn), "prod", "--network", "mainnet")
	runCmd(t, editCmd(mgrFn), "prod", "--default-account", "alice")

	entries := readLog(t, m.Root(), "prod")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Action != "edit" {
		t.Errorf("newest action = %s, want edit", entries[0].Action)
	}
	if string(entries[0].Params) == "" {
		t.Error("edit entry should record the changed fields in params")
	}
}

func TestRenameRecordedInActionLog(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runCmd(t, createCmd(mgrFn), "prod", "--network", "mainnet")
	runCmd(t, renameCmd(mgrFn), "prod", "prod2")

	// The log moves with the renamed context directory.
	entries := readLog(t, m.Root(), "prod2")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (create carried over + rename), got %d", len(entries))
	}
	if entries[0].Action != "rename" {
		t.Errorf("newest action = %s, want rename", entries[0].Action)
	}
}

func TestDeleteRecordedInCurrentContextLog(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runCmd(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")
	runCmd(t, createCmd(mgrFn), "staging", "--network", "mainnet")
	runCmd(t, deleteCmd(mgrFn), "staging", "--yes")

	entries := readLog(t, m.Root(), "prod")

	var found bool
	for _, e := range entries {
		if e.Action == "delete" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a delete entry in the current context's log, got %+v", entries)
	}
}

// TestLogDistinguishesWorkflowSteps covers the reason a workflow run was
// unreadable: the engine writes one entry per step, all carrying the workflow
// name as their action, so a table keyed on the action alone showed six
// identical rows and could not say which step failed.
func TestLogDistinguishesWorkflowSteps(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runCmd(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	steps := []string{
		"create-deployment",
		"wait-for-bids",
		"select-bid",
		"create-lease",
		"send-manifest",
		"display-result",
	}

	l, err := actionlog.Open(aktctx.ActionLogPath(m.Root(), "prod"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	for i, step := range steps {
		entry := actionlog.Entry{
			Type:       actionlog.TypeWorkflow,
			Action:     "deploy",
			WorkflowID: "9f2c1ab34d55e017",
			Step:       i,
			StepName:   step,
			Status:     "success",
		}
		// A middle step fails: the run stopped at the provider gateway.
		if step == "send-manifest" {
			entry.Status = "failed"
			entry.Error = "provider gateway unreachable"
		}
		if err := l.Log(entry); err != nil {
			t.Fatalf("log %s: %v", step, err)
		}
	}
	// A second run of the same workflow, interleaved in the same log.
	if err := l.Log(actionlog.Entry{
		Type:       actionlog.TypeWorkflow,
		Action:     "deploy",
		WorkflowID: "0011223344556677",
		Step:       0,
		StepName:   "create-deployment",
		Status:     "success",
	}); err != nil {
		t.Fatalf("log second run: %v", err)
	}
	_ = l.Close()

	out := runOutput(t, logCmd(mgrFn), "--type", "workflow")

	if !strings.Contains(out, "SUMMARY") {
		t.Errorf("log header = %q, want a SUMMARY column", out)
	}

	lines := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		for _, step := range steps {
			if strings.Contains(line, "deploy/"+step+" ") {
				lines[step] = line
			}
		}
	}
	if len(lines) != len(steps) {
		t.Fatalf("only %d of %d steps rendered distinguishably:\n%s", len(lines), len(steps), out)
	}

	// The failed step is identifiable by name, which is the whole point.
	if failed := lines["send-manifest"]; !strings.Contains(failed, "provider gateway unreachable") {
		t.Errorf("failed step row = %q, want the failure on the send-manifest row", failed)
	}
	if ok := lines["wait-for-bids"]; strings.Contains(ok, "provider gateway unreachable") {
		t.Errorf("successful step row carries the failure: %q", ok)
	}

	// The run id separates two runs of the same workflow.
	if !strings.Contains(out, "run 9f2c1ab34d55e017") || !strings.Contains(out, "run 0011223344556677") {
		t.Errorf("log output does not identify the runs:\n%s", out)
	}

	filtered := runOutput(t, logCmd(mgrFn), "--workflow-id", "0011223344556677")
	if !strings.Contains(filtered, "run 0011223344556677") {
		t.Errorf("--workflow-id dropped its own run:\n%s", filtered)
	}
	if strings.Contains(filtered, "9f2c1ab34d55e017") {
		t.Errorf("--workflow-id kept the other run:\n%s", filtered)
	}
	if strings.Contains(filtered, "send-manifest") {
		t.Errorf("--workflow-id kept steps of the other run:\n%s", filtered)
	}
}

// TestLogSummarizesEntryDetails covers the other rails the summary column
// serves: a chain transaction shows its deployment, a provider operation its
// provider address in full, and a context entry its parameters.
func TestLogSummarizesEntryDetails(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runCmd(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	const provider = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"

	l, err := actionlog.Open(aktctx.ActionLogPath(m.Root(), "prod"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	if err := l.Log(actionlog.Entry{
		Type:   actionlog.TypeTx,
		Action: "deployment.MsgCreateDeployment",
		DSeq:   12345,
		Status: "success",
	}); err != nil {
		t.Fatalf("log tx: %v", err)
	}
	if err := l.Log(actionlog.Entry{
		Type:     actionlog.TypeProvider,
		Action:   "send-manifest",
		Provider: provider,
		DSeq:     12345,
		Status:   "success",
	}); err != nil {
		t.Fatalf("log provider op: %v", err)
	}
	_ = l.Close()

	out := runOutput(t, logCmd(mgrFn))

	if !strings.Contains(out, "deployment.MsgCreateDeployment (dseq: 12345)") {
		t.Errorf("tx summary missing the deployment:\n%s", out)
	}
	if !strings.Contains(out, "send-manifest -> "+provider) {
		t.Errorf("provider summary missing the full provider address:\n%s", out)
	}
	// Context entries carry their recorded parameters.
	if !strings.Contains(out, "create (keyring:") || !strings.Contains(out, "network: mainnet") {
		t.Errorf("context summary missing its parameters:\n%s", out)
	}
}

// TestKeysCommandsRecordIntoTheContextLog covers the wiring that makes key
// management visible at all: the keys package cannot open a context's log, so
// `context` injects the recorder when it builds the command tree. Creating a
// key used to leave no trace while every other mutation was recorded.
func TestKeysCommandsRecordIntoTheContextLog(t *testing.T) {
	const mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	source := filepath.Join(t.TempDir(), "mnemonic.txt")
	if err := os.WriteFile(source, []byte(mnemonic+"\n"), 0o600); err != nil {
		t.Fatalf("write mnemonic fixture: %v", err)
	}

	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	keyringFn := func() (sdkkeyring.Keyring, error) { return kr, nil }

	runOK(t, Commands(mgrFn, keyringFn), "keys", "add", "alice", "--recover", "--source", source)

	record, err := kr.Key("alice")
	if err != nil {
		t.Fatalf("added key not in the keyring: %v", err)
	}
	address, err := record.GetAddress()
	if err != nil {
		t.Fatalf("get address: %v", err)
	}

	entries := readLog(t, m.Root(), "prod")
	if entries[0].Type != actionlog.TypeContext || entries[0].Action != "keys.recover" {
		t.Fatalf("newest entry = %s/%s, want context/keys.recover", entries[0].Type, entries[0].Action)
	}
	if entries[0].Status != "success" {
		t.Errorf("status = %q, want success", entries[0].Status)
	}

	params := string(entries[0].Params)
	// The address is recorded in full, never shortened.
	if !strings.Contains(params, address.String()) {
		t.Errorf("params = %s, want the full address %s", params, address)
	}
	if !strings.Contains(params, `"name":"alice"`) || !strings.Contains(params, `"type":"local"`) {
		t.Errorf("params = %s, want the key name and type", params)
	}

	runOK(t, Commands(mgrFn, keyringFn), "keys", "delete", "alice", "--yes")

	entries = readLog(t, m.Root(), "prod")
	if entries[0].Action != "keys.delete" {
		t.Errorf("newest entry after delete = %s, want keys.delete", entries[0].Action)
	}
	if !strings.Contains(string(entries[0].Params), address.String()) {
		t.Errorf("delete params = %s, want the deleted address", entries[0].Params)
	}

	// No secret material anywhere in the log, in any form.
	raw, err := os.ReadFile(aktctx.ActionLogPath(m.Root(), "prod"))
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if strings.Contains(string(raw), "abandon") {
		t.Errorf("the mnemonic reached the action log:\n%s", raw)
	}
}
