package context

import (
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
	aktctx "pkg.akt.dev/akt/internal/context"
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
