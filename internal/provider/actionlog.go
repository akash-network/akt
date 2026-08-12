package provider

import (
	"context"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
)

// RecordAction writes a type=provider entry for a completed state-changing
// provider gateway operation. Logging is best-effort and a nil logger is a
// no-op, so audit availability never changes the operation's result.
func RecordAction(ctx context.Context, action, provider string, dseq uint64, opErr error) {
	l := cliutil.ActionLogFromContext(ctx)
	if l == nil {
		return
	}

	entry := actionlog.Entry{
		Type:     actionlog.TypeProvider,
		Action:   action,
		Provider: provider,
		DSeq:     dseq,
		Status:   "success",
	}

	if opErr != nil {
		entry.Status = "failed"
		entry.Error = opErr.Error()
	}

	_ = l.Log(entry)
}
