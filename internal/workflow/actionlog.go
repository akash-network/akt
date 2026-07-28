package workflow

import (
	"pkg.akt.dev/akt/internal/actionlog"
)

// ActionLogAdapter implements Logger by writing one type=workflow entry per
// completed step to the per-context action log (SPEC §5.6). A nil adapter or
// nil underlying logger is a no-op, so wiring is always safe.
type ActionLogAdapter struct {
	log      *actionlog.Logger
	workflow string
}

// NewActionLogAdapter creates an adapter that records steps of the named
// workflow to l. Returns nil when l is nil, which the engine treats as
// "no logging".
func NewActionLogAdapter(l *actionlog.Logger, workflow string) *ActionLogAdapter {
	if l == nil {
		return nil
	}

	return &ActionLogAdapter{log: l, workflow: workflow}
}

// LogStep records a completed workflow step. Logging is best-effort and
// never interrupts workflow execution.
func (a *ActionLogAdapter) LogStep(workflowID string, stepIndex int, result *StepResult) {
	if a == nil || a.log == nil || result == nil {
		return
	}

	_ = a.log.Log(actionlog.Entry{
		Type:       actionlog.TypeWorkflow,
		Action:     a.workflow,
		WorkflowID: workflowID,
		Step:       stepIndex,
		StepName:   result.Name,
		Status:     result.Status,
		Error:      result.Error,
		TxHash:     result.TxHash,
		Height:     result.Height,
	})
}
