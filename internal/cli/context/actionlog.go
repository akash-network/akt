package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"pkg.akt.dev/akt/internal/actionlog"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// recordContextAction appends a successful context-management entry (SPEC
// §5.6) to the action log of the context named target.
func recordContextAction(root, target, action string, details map[string]string) {
	recordContextResult(root, target, action, nil, details)
}

// recordContextResult appends a context-management entry (SPEC §5.6) to the
// action log of the context named target, recording success or the failure
// carried by actionErr. Context commands often run without an open logger for
// the affected context (e.g. create, or switching away from the current one),
// so the target log is opened directly. Logging is best-effort: a failure
// never blocks the command itself.
func recordContextResult(root, target, action string, actionErr error, details map[string]string) {
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

	entry := actionlog.Entry{
		Type:   actionlog.TypeContext,
		Action: action,
		Status: "success",
		Params: params,
	}

	if actionErr != nil {
		entry.Status = "failed"
		entry.Error = actionErr.Error()
	}

	_ = l.Log(entry)
}

// entrySummary composes the SUMMARY cell of `akt context log` (SPEC §2.2).
// The action name alone does not identify an entry — one workflow run writes
// one entry per step, all under the same workflow name — so the summary adds
// whatever distinguishes the individual entry: the step and run for workflow
// entries, the provider and deployment for gateway and chain entries, and the
// recorded parameters for context entries.
func entrySummary(e actionlog.Entry) string {
	head := e.Action
	if e.Type == actionlog.TypeWorkflow && e.StepName != "" {
		head += "/" + e.StepName
	}

	// Addresses are never shortened (AGENTS.md), so the provider goes in
	// full.
	if e.Provider != "" {
		head += " -> " + e.Provider
	}

	var details []string

	if e.DSeq != 0 {
		details = append(details, fmt.Sprintf("dseq: %d", e.DSeq))
	}

	if e.Type == actionlog.TypeWorkflow && e.WorkflowID != "" {
		details = append(details, "run "+e.WorkflowID)
	}

	details = append(details, paramPairs(e.Params)...)

	if len(details) == 0 {
		return head
	}

	return head + " (" + strings.Join(details, ", ") + ")"
}

// paramPairs renders a recorded params object as "key: value" pairs sorted by
// key. Params that are not a JSON object are left out of the table rather than
// dumped into it; -o json still carries them verbatim.
func paramPairs(params json.RawMessage) []string {
	if len(params) == 0 {
		return nil
	}

	decoder := json.NewDecoder(bytes.NewReader(params))
	decoder.UseNumber()

	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return nil
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+": "+paramValue(fields[key]))
	}

	return pairs
}

func paramValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return ""
		}

		return string(encoded)
	}
}
