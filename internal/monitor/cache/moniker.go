package cache

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// Bucket name for monikers
var bucketMonikers = []byte("monikers")

// MonikerCache stores validator pubkey -> moniker mappings in bbolt.
type MonikerCache struct {
	db *bolt.DB
}

// OpenMonikerCache opens or creates the moniker cache in the given bbolt database.
func OpenMonikerCache(db *bolt.DB) (*MonikerCache, error) {
	err := db.Update(ensureMonikerBucket)
	if err != nil {
		return nil, err
	}
	return &MonikerCache{db: db}, nil
}

func ensureMonikerBucket(tx *bolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(bucketMonikers)
	return err
}

// Get returns all monikers as a map.
func (c *MonikerCache) Get() map[string]string {
	result := make(map[string]string)
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMonikers)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			result[string(k)] = string(v)
			return nil
		})
	})
	return result
}

// Set replaces all monikers with the given map.
func (c *MonikerCache) Set(monikers map[string]string) {
	_ = c.db.Update(func(tx *bolt.Tx) error {
		// Delete and recreate the bucket to clear old entries
		_ = tx.DeleteBucket(bucketMonikers)
		b, err := tx.CreateBucket(bucketMonikers)
		if err != nil {
			return err
		}
		for k, v := range monikers {
			if err := b.Put([]byte(k), []byte(v)); err != nil {
				return err
			}
		}
		return nil
	})
}

// HasMonikers returns true if the cache has any entries.
func (c *MonikerCache) HasMonikers() bool {
	var has bool
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMonikers)
		if b == nil {
			return nil
		}
		has = b.Stats().KeyN > 0
		return nil
	})
	return has
}

// Save is a no-op for bbolt — data is already persisted on each write.
func (c *MonikerCache) Save() error {
	return nil
}

// OpenDB opens or creates a bbolt database at the given path.
// Callers are responsible for closing the returned database.
func OpenDB(path string) (*bolt.DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout:    1,
		NoGrowSync: false,
	})
	if err != nil {
		return nil, fmt.Errorf("open bbolt database %s: %w", path, err)
	}
	return db, nil
}
