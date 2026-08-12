package workflow

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	wf "pkg.akt.dev/akt/internal/workflow"
)

type workflowDiagnosticBoundaryWriter struct {
	err   error
	short bool
}

func (w workflowDiagnosticBoundaryWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.short && len(p) > 0 {
		return len(p) - 1, nil
	}

	return len(p), nil
}

func TestFilterProviderStepsPropagatesDestinationFailures(t *testing.T) {
	def := &wf.WorkflowDef{
		Name: "deploy",
		Steps: []wf.StepDef{
			{Name: "create", Type: wf.StepTx},
			{Name: "send-manifest", Type: wf.StepProvider},
		},
	}
	hardErr := errors.New("workflow diagnostic destination failed")
	tests := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: workflowDiagnosticBoundaryWriter{err: hardErr}, want: hardErr},
		{name: "short write", writer: workflowDiagnosticBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filtered, err := filterProviderSteps(def, test.writer)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want destination failure %v", err, test.want)
			}
			if filtered != nil {
				t.Fatalf("filtered definition = %#v, want nil after a diagnostic failure", filtered)
			}
		})
	}
}

func TestExecuteConsoleWorkflowPropagatesSkippedStepDiagnosticFailure(t *testing.T) {
	t.Setenv(aktctx.EnvConsoleAPIKey, "test-console-key")
	home := t.TempDir()
	m := newTestManager(t, home, "console", aktctx.AuthMethodConsoleAPI)
	def := &wf.WorkflowDef{
		Name: "deploy",
		Steps: []wf.StepDef{
			{Name: "create", Type: wf.StepTx},
			{Name: "send-manifest", Type: wf.StepProvider},
		},
	}

	writeErr := errors.New("workflow diagnostic destination failed")
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	cmd.SetErr(workflowDiagnosticBoundaryWriter{err: writeErr})

	err := executeWorkflow(
		cmd,
		def,
		map[string]any{},
		func() *aktctx.Manager { return m },
		func() string { return "console" },
		false,
		nil,
	)
	if !errors.Is(err, writeErr) {
		t.Fatalf("execute error = %v, want destination failure", err)
	}
	if err == nil || !strings.Contains(err.Error(), "report skipped Console workflow step") {
		t.Fatalf("execute error = %v, want workflow boundary context", err)
	}
	if len(def.Steps) != 2 {
		t.Fatalf("failed execution mutated shared definition: %#v", def.Steps)
	}
}
