package bbolt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// OpenContext opens (creating if needed) the deployment store belonging to a
// named context and applies any pending schema migrations (SPEC §4.5).
//
// It is the only way production code opens a store. Resolving the path here
// rather than in each caller keeps `akt store *` and the workflow persistence
// of SPEC §6.6 pointed at the same file, and running Migrate on open keeps a
// store that was last written by an older binary from being read at a stale
// schema.
func OpenContext(ctx context.Context, root, ctxName string) (*BoltStore, error) {
	path := aktctx.StoreDBPath(root, ctxName)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}

	s, err := Open(path)
	if err != nil {
		return nil, err
	}

	if err := s.Migrate(ctx); err != nil {
		_ = s.Close()

		return nil, fmt.Errorf("migrate store %s: %w", path, err)
	}

	return s, nil
}
