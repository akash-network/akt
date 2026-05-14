package bbolt

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// withMigrations replaces the package-level migrations slice for the duration of fn,
// then restores the original. This prevents test cases from interfering with each other.
func withMigrations(ms []Migration, fn func()) {
	old := migrations
	migrations = ms
	defer func() { migrations = old }()
	fn()
}

func TestMigrateFromV1(t *testing.T) {
	withMigrations([]Migration{
		{
			Version:     2,
			Description: "add test_v2 bucket",
			Fn: func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists([]byte("test_v2"))
				return err
			},
		},
	}, func() {
		s := openTestStore(t)
		ctx := context.Background()

		// Store starts at v1.
		require.Equal(t, uint64(1), s.SchemaVersion())

		// Run migration.
		err := s.Migrate(ctx)
		require.NoError(t, err)

		// Schema version should now be 2.
		assert.Equal(t, uint64(2), s.SchemaVersion())

		// The bucket created by the migration should exist.
		err = s.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("test_v2"))
			if b == nil {
				t.Fatal("expected test_v2 bucket to exist after migration")
			}
			return nil
		})
		require.NoError(t, err)
	})
}

func TestMigrateNoOp(t *testing.T) {
	// No migrations registered — store is already at latest.
	withMigrations(nil, func() {
		s := openTestStore(t)
		ctx := context.Background()

		require.Equal(t, uint64(1), s.SchemaVersion())

		err := s.Migrate(ctx)
		require.NoError(t, err)

		// Version unchanged.
		assert.Equal(t, uint64(1), s.SchemaVersion())
	})
}

func TestMigrateMultipleVersions(t *testing.T) {
	withMigrations([]Migration{
		{
			Version:     2,
			Description: "add bucket_v2",
			Fn: func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists([]byte("bucket_v2"))
				return err
			},
		},
		{
			Version:     3,
			Description: "add bucket_v3",
			Fn: func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists([]byte("bucket_v3"))
				return err
			},
		},
	}, func() {
		s := openTestStore(t)
		ctx := context.Background()

		require.Equal(t, uint64(1), s.SchemaVersion())

		err := s.Migrate(ctx)
		require.NoError(t, err)

		// Should be at v3 after both migrations.
		assert.Equal(t, uint64(3), s.SchemaVersion())

		// Both buckets should exist.
		err = s.db.View(func(tx *bolt.Tx) error {
			if tx.Bucket([]byte("bucket_v2")) == nil {
				t.Fatal("expected bucket_v2 to exist")
			}
			if tx.Bucket([]byte("bucket_v3")) == nil {
				t.Fatal("expected bucket_v3 to exist")
			}
			return nil
		})
		require.NoError(t, err)
	})
}

func TestMigrationError(t *testing.T) {
	errBoom := errors.New("boom")

	withMigrations([]Migration{
		{
			Version:     2,
			Description: "failing migration",
			Fn: func(tx *bolt.Tx) error {
				return errBoom
			},
		},
	}, func() {
		s := openTestStore(t)
		ctx := context.Background()

		require.Equal(t, uint64(1), s.SchemaVersion())

		err := s.Migrate(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, errBoom)

		// Schema version must NOT have been updated (transaction rolled back).
		assert.Equal(t, uint64(1), s.SchemaVersion())
	})
}

func TestMigrationErrorRollsBackPartial(t *testing.T) {
	// v2 succeeds, v3 fails — entire transaction should roll back,
	// leaving schema at v1 with no v2 side effects.
	errFail := errors.New("v3 failed")

	withMigrations([]Migration{
		{
			Version:     2,
			Description: "add partial_bucket",
			Fn: func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists([]byte("partial_bucket"))
				return err
			},
		},
		{
			Version:     3,
			Description: "fails",
			Fn: func(tx *bolt.Tx) error {
				return errFail
			},
		},
	}, func() {
		s := openTestStore(t)
		ctx := context.Background()

		err := s.Migrate(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, errFail)

		// Version stays at 1 — entire tx rolled back.
		assert.Equal(t, uint64(1), s.SchemaVersion())

		// The bucket from v2 should NOT exist (rolled back).
		_ = s.db.View(func(tx *bolt.Tx) error {
			assert.Nil(t, tx.Bucket([]byte("partial_bucket")),
				"partial_bucket should not exist after rollback")
			return nil
		})
	})
}

func TestRegisterMigrationDuplicatePanics(t *testing.T) {
	withMigrations(nil, func() {
		RegisterMigration(Migration{Version: 99, Description: "first", Fn: func(tx *bolt.Tx) error { return nil }})
		assert.Panics(t, func() {
			RegisterMigration(Migration{Version: 99, Description: "dup", Fn: func(tx *bolt.Tx) error { return nil }})
		})
	})
}

func TestLatestVersion(t *testing.T) {
	withMigrations(nil, func() {
		// No migrations: latest is currentSchemaVersion.
		assert.Equal(t, currentSchemaVersion, latestVersion())
	})

	withMigrations([]Migration{
		{Version: 5, Description: "v5"},
		{Version: 3, Description: "v3"},
	}, func() {
		assert.Equal(t, uint64(5), latestVersion())
	})
}

func TestSchemaVersionFromTx(t *testing.T) {
	s := openTestStore(t)

	_ = s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		v := schemaVersionFromTx(meta)
		assert.Equal(t, uint64(1), v)
		return nil
	})

	// Write a different version and verify.
	_ = s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, 42)
		return meta.Put(keySchemaVersion, buf)
	})

	_ = s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		v := schemaVersionFromTx(meta)
		assert.Equal(t, uint64(42), v)
		return nil
	})
}
