package bbolt

import (
	"encoding/binary"
	"fmt"
	"slices"

	bolt "go.etcd.io/bbolt"
)

// Migration represents a schema migration to a specific version.
type Migration struct {
	Version     uint64
	Description string
	Fn          func(tx *bolt.Tx) error
}

// migrations holds all registered migrations, ordered by version.
// This is a package-level variable (the only justified one — it's a static registry
// populated at init-time by the package itself, not by callers).
var migrations []Migration

// RegisterMigration adds a migration to the registry.
// Migrations must have unique versions; duplicates cause a panic.
func RegisterMigration(m Migration) {
	for _, existing := range migrations {
		if existing.Version == m.Version {
			panic(fmt.Sprintf("duplicate migration version %d", m.Version))
		}
	}
	migrations = append(migrations, m)
	slices.SortFunc(migrations, func(a, b Migration) int {
		if a.Version < b.Version {
			return -1
		}
		if a.Version > b.Version {
			return 1
		}
		return 0
	})
}

// latestVersion returns the highest registered migration version,
// or currentSchemaVersion if no migrations are registered.
func latestVersion() uint64 {
	max := currentSchemaVersion
	for _, m := range migrations {
		if m.Version > max {
			max = m.Version
		}
	}
	return max
}

// migrate applies all pending migrations within a single bbolt transaction.
// It reads the current schema version, applies migrations with Version > current
// in order, and updates the stored schema version after each one.
// Returns nil if already at the latest version.
func (s *BoltStore) migrate() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		current := schemaVersionFromTx(meta)

		for _, m := range migrations {
			if m.Version <= current {
				continue
			}
			if err := m.Fn(tx); err != nil {
				return fmt.Errorf("migration to v%d (%s): %w", m.Version, m.Description, err)
			}
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, m.Version)
			if err := meta.Put(keySchemaVersion, buf); err != nil {
				return fmt.Errorf("update schema_version to %d: %w", m.Version, err)
			}
		}
		return nil
	})
}

// schemaVersionFromTx reads the schema version from the meta bucket within a transaction.
func schemaVersionFromTx(meta *bolt.Bucket) uint64 {
	data := meta.Get(keySchemaVersion)
	if data == nil || len(data) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(data)
}
