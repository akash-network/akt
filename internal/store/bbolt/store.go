package bbolt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	bolt "go.etcd.io/bbolt"

	"pkg.akt.dev/akt/internal/store"
)

// Bucket names for the store.
var (
	bucketDeployments = []byte("deployments")
	bucketLeases      = []byte("leases")
	bucketBids        = []byte("bids")
	bucketSync        = []byte("sync")
	bucketMeta        = []byte("meta")

	keySyncState     = []byte("state")
	keySchemaVersion = []byte("schema_version")
)

// currentSchemaVersion is the schema version written on first open.
const currentSchemaVersion uint64 = 1

// BoltStore implements store.Store backed by a bbolt database.
type BoltStore struct {
	db   *bolt.DB
	path string
}

// Compile-time check that BoltStore implements store.Store.
var _ store.Store = (*BoltStore)(nil)

// Open opens (or creates) a bbolt-backed store at path.
// It creates all required buckets and initializes schema_version to 1 if not set.
func Open(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open bbolt database %s: %w", path, err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{
			bucketDeployments,
			bucketLeases,
			bucketBids,
			bucketSync,
			bucketMeta,
		} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("create bucket %s: %w", name, err)
			}
		}

		// Initialize schema_version if not set.
		meta := tx.Bucket(bucketMeta)
		if meta.Get(keySchemaVersion) == nil {
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, currentSchemaVersion)
			if err := meta.Put(keySchemaVersion, buf); err != nil {
				return fmt.Errorf("set schema_version: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &BoltStore{db: db, path: path}, nil
}

// Close closes the underlying bbolt database.
func (s *BoltStore) Close() error {
	return s.db.Close()
}

// --- Deployment operations ---

// PutDeployment stores a deployment record.
func (s *BoltStore) PutDeployment(_ context.Context, d *store.DeploymentRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketDeployments)
		data, err := json.Marshal(d)
		if err != nil {
			return fmt.Errorf("marshal deployment: %w", err)
		}
		key := store.DeploymentKey(d.Owner, d.DSeq)
		return b.Put([]byte(key), data)
	})
}

// GetDeployment retrieves a deployment by owner and dseq.
// Returns (nil, nil) if not found.
func (s *BoltStore) GetDeployment(_ context.Context, owner string, dseq uint64) (*store.DeploymentRecord, error) {
	var rec *store.DeploymentRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketDeployments)
		key := store.DeploymentKey(owner, dseq)
		data := b.Get([]byte(key))
		if data == nil {
			return nil
		}
		rec = &store.DeploymentRecord{}
		if err := json.Unmarshal(data, rec); err != nil {
			return fmt.Errorf("unmarshal deployment: %w", err)
		}
		return nil
	})
	return rec, err
}

// ListDeployments returns deployments matching the given filter.
func (s *BoltStore) ListDeployments(_ context.Context, filter store.DeploymentFilter) ([]*store.DeploymentRecord, error) {
	var results []*store.DeploymentRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketDeployments)
		return b.ForEach(func(k, v []byte) error {
			var d store.DeploymentRecord
			if err := json.Unmarshal(v, &d); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			if matchDeployment(&d, filter) {
				cp := d
				results = append(results, &cp)
			}
			return nil
		})
	})
	return results, err
}

// DeleteDeployment removes a deployment by owner and dseq.
func (s *BoltStore) DeleteDeployment(_ context.Context, owner string, dseq uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketDeployments)
		key := store.DeploymentKey(owner, dseq)
		return b.Delete([]byte(key))
	})
}

// --- Lease operations ---

// PutLease stores a lease record.
func (s *BoltStore) PutLease(_ context.Context, l *store.LeaseRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketLeases)
		data, err := json.Marshal(l)
		if err != nil {
			return fmt.Errorf("marshal lease: %w", err)
		}
		key := store.LeaseKey(l.ID)
		return b.Put([]byte(key), data)
	})
}

// GetLease retrieves a lease by its ID.
// Returns (nil, nil) if not found.
func (s *BoltStore) GetLease(_ context.Context, id store.LeaseID) (*store.LeaseRecord, error) {
	var rec *store.LeaseRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketLeases)
		key := store.LeaseKey(id)
		data := b.Get([]byte(key))
		if data == nil {
			return nil
		}
		rec = &store.LeaseRecord{}
		if err := json.Unmarshal(data, rec); err != nil {
			return fmt.Errorf("unmarshal lease: %w", err)
		}
		return nil
	})
	return rec, err
}

// ListLeases returns leases matching the given filter.
func (s *BoltStore) ListLeases(_ context.Context, filter store.LeaseFilter) ([]*store.LeaseRecord, error) {
	var results []*store.LeaseRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketLeases)
		return b.ForEach(func(k, v []byte) error {
			var l store.LeaseRecord
			if err := json.Unmarshal(v, &l); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			if matchLease(&l, filter) {
				cp := l
				results = append(results, &cp)
			}
			return nil
		})
	})
	return results, err
}

// DeleteLease removes a lease by its ID.
func (s *BoltStore) DeleteLease(_ context.Context, id store.LeaseID) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketLeases)
		key := store.LeaseKey(id)
		return b.Delete([]byte(key))
	})
}

// --- Bid operations ---

// PutBid stores a bid record.
func (s *BoltStore) PutBid(_ context.Context, b *store.BidRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketBids)
		data, err := json.Marshal(b)
		if err != nil {
			return fmt.Errorf("marshal bid: %w", err)
		}
		key := store.BidKey(b.ID)
		return bucket.Put([]byte(key), data)
	})
}

// GetBid retrieves a bid by its ID.
// Returns (nil, nil) if not found.
func (s *BoltStore) GetBid(_ context.Context, id store.BidID) (*store.BidRecord, error) {
	var rec *store.BidRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBids)
		key := store.BidKey(id)
		data := b.Get([]byte(key))
		if data == nil {
			return nil
		}
		rec = &store.BidRecord{}
		if err := json.Unmarshal(data, rec); err != nil {
			return fmt.Errorf("unmarshal bid: %w", err)
		}
		return nil
	})
	return rec, err
}

// ListBids returns bids matching the given filter.
func (s *BoltStore) ListBids(_ context.Context, filter store.BidFilter) ([]*store.BidRecord, error) {
	var results []*store.BidRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBids)
		return b.ForEach(func(k, v []byte) error {
			var bid store.BidRecord
			if err := json.Unmarshal(v, &bid); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			if matchBid(&bid, filter) {
				cp := bid
				results = append(results, &cp)
			}
			return nil
		})
	})
	return results, err
}

// --- Sync state ---

// GetSyncState retrieves the sync state.
// Returns (nil, nil) if no sync state has been stored.
func (s *BoltStore) GetSyncState(_ context.Context) (*store.SyncState, error) {
	var state *store.SyncState
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSync)
		data := b.Get(keySyncState)
		if data == nil {
			return nil
		}
		state = &store.SyncState{}
		if err := json.Unmarshal(data, state); err != nil {
			return fmt.Errorf("unmarshal sync state: %w", err)
		}
		return nil
	})
	return state, err
}

// PutSyncState stores the sync state.
func (s *BoltStore) PutSyncState(_ context.Context, st *store.SyncState) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSync)
		data, err := json.Marshal(st)
		if err != nil {
			return fmt.Errorf("marshal sync state: %w", err)
		}
		return b.Put(keySyncState, data)
	})
}

// --- Schema management ---

// SchemaVersion reads the schema version from the meta bucket.
func (s *BoltStore) SchemaVersion() uint64 {
	var version uint64
	_ = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMeta)
		data := b.Get(keySchemaVersion)
		if len(data) < 8 {
			return nil
		}
		version = binary.BigEndian.Uint64(data)
		return nil
	})
	return version
}

// Migrate runs any pending schema migrations.
func (s *BoltStore) Migrate(_ context.Context) error {
	return s.migrate()
}

// --- Import/Export ---

// Export serializes the store contents to the given writer.
func (s *BoltStore) Export(ctx context.Context, w io.Writer, format store.ExportFormat) error {
	return s.export(ctx, w, format)
}

// Import deserializes store contents from the given reader.
func (s *BoltStore) Import(ctx context.Context, r io.Reader, format store.ExportFormat, merge bool) error {
	return s.importData(ctx, r, format, merge)
}

// --- Stats ---

// Stats returns aggregate statistics about the store contents.
func (s *BoltStore) Stats(_ context.Context) (*store.StoreStats, error) {
	stats := &store.StoreStats{}

	err := s.db.View(func(tx *bolt.Tx) error {
		// Count deployments and categorize by state.
		depBucket := tx.Bucket(bucketDeployments)
		if err := depBucket.ForEach(func(k, v []byte) error {
			var d store.DeploymentRecord
			if err := json.Unmarshal(v, &d); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			stats.Deployments++
			switch d.State {
			case "active":
				stats.ActiveDeployments++
			case "closed":
				stats.ClosedDeployments++
			}
			return nil
		}); err != nil {
			return err
		}

		// Count leases.
		leaseBucket := tx.Bucket(bucketLeases)
		if err := leaseBucket.ForEach(func(_, _ []byte) error {
			stats.Leases++
			return nil
		}); err != nil {
			return err
		}

		// Count bids.
		bidBucket := tx.Bucket(bucketBids)
		if err := bidBucket.ForEach(func(_, _ []byte) error {
			stats.Bids++
			return nil
		}); err != nil {
			return err
		}

		return nil
	})

	return stats, err
}

// --- Filter matching helpers ---

func matchDeployment(d *store.DeploymentRecord, f store.DeploymentFilter) bool {
	if f.Owner != "" && d.Owner != f.Owner {
		return false
	}
	if f.State != "" && d.State != f.State {
		return false
	}
	// Tags: AND logic — all filter tags must be present.
	for _, tag := range f.Tags {
		found := false
		for _, dt := range d.Tags {
			if dt == tag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// Label: key=value match in Labels map.
	if f.Label != "" {
		parts := strings.SplitN(f.Label, "=", 2)
		if len(parts) == 2 {
			if d.Labels == nil {
				return false
			}
			if d.Labels[parts[0]] != parts[1] {
				return false
			}
		}
	}
	return true
}

func matchLease(l *store.LeaseRecord, f store.LeaseFilter) bool {
	if f.Owner != "" && l.ID.Owner != f.Owner {
		return false
	}
	if f.DSeq != 0 && l.ID.DSeq != f.DSeq {
		return false
	}
	if f.Provider != "" && l.ID.Provider != f.Provider {
		return false
	}
	if f.State != "" && l.State != f.State {
		return false
	}
	return true
}

func matchBid(b *store.BidRecord, f store.BidFilter) bool {
	if f.Owner != "" && b.ID.Owner != f.Owner {
		return false
	}
	if f.DSeq != 0 && b.ID.DSeq != f.DSeq {
		return false
	}
	if f.Provider != "" && b.ID.Provider != f.Provider {
		return false
	}
	if f.State != "" && b.State != f.State {
		return false
	}
	return true
}

// Path returns the filesystem path of the underlying database file.
// Exported for use by os.Stat in callers that need file-level info.
func (s *BoltStore) Path() string {
	return s.path
}
