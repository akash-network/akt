package provider

import (
	"context"

	aktprovider "pkg.akt.dev/akt/internal/provider"
)

// recordProviderAction writes a type=provider entry (SPEC §5.6) for a
// state-changing provider gateway operation. Logging is best-effort: a
// failure never blocks the command itself.
func recordProviderAction(ctx context.Context, action, provider string, dseq uint64, opErr error) {
	aktprovider.RecordAction(ctx, action, provider, dseq, opErr)
}
