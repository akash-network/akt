package workflow

import (
	"path/filepath"
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
)

func TestActionLogAdapterRecordsSteps(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	defer l.Close()

	a := NewActionLogAdapter(l, "deploy")

	a.LogStep("wf_123", 0, &StepResult{
		Name:   "create-deployment",
		Type:   StepTx,
		Status: "success",
		TxHash: "ABC",
		Height: 10,
	})
	a.LogStep("wf_123", 1, &StepResult{
		Name:   "send-manifest",
		Type:   StepProvider,
		Status: "failed",
		Error:  "provider unreachable",
	})

	entries, err := l.Read(actionlog.Filter{Type: actionlog.TypeWorkflow})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Newest first.
	failed := entries[0]
	if failed.StepName != "send-manifest" || failed.Status != "failed" || failed.Error != "provider unreachable" {
		t.Errorf("failed step entry wrong: %+v", failed)
	}

	ok := entries[1]
	if ok.Action != "deploy" || ok.WorkflowID != "wf_123" || ok.Step != 0 {
		t.Errorf("workflow identity fields wrong: %+v", ok)
	}
	if ok.TxHash != "ABC" || ok.Height != 10 {
		t.Errorf("tx fields not carried into entry: %+v", ok)
	}
}

func TestActionLogAdapterNilSafety(t *testing.T) {
	if NewActionLogAdapter(nil, "deploy") != nil {
		t.Fatal("nil logger must produce a nil adapter")
	}

	var a *ActionLogAdapter
	a.LogStep("wf", 0, &StepResult{Name: "x"}) // must not panic
}
