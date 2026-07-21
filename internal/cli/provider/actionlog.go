package provider

import (
	"context"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
)

// recordProviderAction writes a type=provider entry (SPEC §5.6) for a
// state-changing provider gateway operation. Logging is best-effort: a
// failure never blocks the command itself.
func recordProviderAction(ctx context.Context, action, provider string, dseq uint64, opErr error) {
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
