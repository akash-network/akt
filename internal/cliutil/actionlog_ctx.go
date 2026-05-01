package cliutil

import (
	"context"

	"pkg.akt.dev/akt/internal/actionlog"
)

type actionLogKey struct{}

// WithActionLog stores an *actionlog.Logger in the context.
func WithActionLog(ctx context.Context, l *actionlog.Logger) context.Context {
	return context.WithValue(ctx, actionLogKey{}, l)
}

// ActionLogFromContext returns the *actionlog.Logger stored in the context,
// or nil if none was set.
func ActionLogFromContext(ctx context.Context) *actionlog.Logger {
	l, _ := ctx.Value(actionLogKey{}).(*actionlog.Logger)
	return l
}
