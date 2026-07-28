package context

import (
	"encoding/json"

	"pkg.akt.dev/akt/internal/actionlog"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// recordContextAction appends a context-management entry (SPEC §5.6) to the
// action log of the context named target. Context commands often run without
// an open logger for the affected context (e.g. create, or switching away
// from the current one), so the target log is opened directly. Logging is
// best-effort: a failure never blocks the command itself.
func recordContextAction(root, target, action string, details map[string]string) {
	if root == "" || target == "" {
		return
	}

	l, err := actionlog.Open(aktctx.ActionLogPath(root, target))
	if err != nil {
		return
	}
	defer func() { _ = l.Close() }()

	var params json.RawMessage
	if len(details) > 0 {
		params, _ = json.Marshal(details)
	}

	_ = l.Log(actionlog.Entry{
		Type:   actionlog.TypeContext,
		Action: action,
		Status: "success",
		Params: params,
	})
}
