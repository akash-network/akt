package bbolt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	bolt "go.etcd.io/bbolt"
	"gopkg.in/yaml.v3"

	"pkg.akt.dev/akt/internal/store"
)

// ExportEnvelope is the top-level structure for store export/import.
type ExportEnvelope struct {
	Version       int                       `json:"version"        yaml:"version"`
	Context       string                    `json:"context"        yaml:"context"`
	SchemaVersion uint64                    `json:"schema_version" yaml:"schema_version"`
	ExportedAt    string                    `json:"exported_at"    yaml:"exported_at"`
	SyncState     *store.SyncState          `json:"sync_state"     yaml:"sync_state"`
	Deployments   []*store.DeploymentRecord `json:"deployments"    yaml:"deployments"`
	Leases        []*store.LeaseRecord      `json:"leases"         yaml:"leases"`
	Bids          []*store.BidRecord        `json:"bids"           yaml:"bids"`
}

// export serializes the store contents to the given writer in the specified format.
func (s *BoltStore) export(ctx context.Context, w io.Writer, format store.ExportFormat) error {
	deployments, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	leases, err := s.ListLeases(ctx, store.LeaseFilter{})
	if err != nil {
		return fmt.Errorf("list leases: %w", err)
	}

	bids, err := s.ListBids(ctx, store.BidFilter{})
	if err != nil {
		return fmt.Errorf("list bids: %w", err)
	}

	syncState, err := s.GetSyncState(ctx)
	if err != nil {
		return fmt.Errorf("get sync state: %w", err)
	}

	env := ExportEnvelope{
		Version:       1,
		SchemaVersion: s.SchemaVersion(),
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		SyncState:     syncState,
		Deployments:   deployments,
		Leases:        leases,
		Bids:          bids,
	}

	switch format {
	case store.FormatYAML:
		if _, err := w.Write([]byte("---\n")); err != nil {
			return fmt.Errorf("write YAML document start: %w", err)
		}
		enc := yaml.NewEncoder(w)
		defer enc.Close()
		if err := enc.Encode(env); err != nil {
			return fmt.Errorf("encode YAML: %w", err)
		}
	case store.FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(env); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	default:
		return fmt.Errorf("unsupported export format: %d", format)
	}

	return nil
}

// importData reads store contents from the given reader and loads them into the store.
// If merge is false, all existing data buckets are cleared first (replace mode).
func (s *BoltStore) importData(ctx context.Context, r io.Reader, format store.ExportFormat, merge bool) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	var env ExportEnvelope
	switch format {
	case store.FormatYAML:
		if err := yaml.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("unmarshal YAML: %w", err)
		}
	case store.FormatJSON:
		if err := json.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("unmarshal JSON: %w", err)
		}
	default:
		return fmt.Errorf("unsupported import format: %d", format)
	}

	// In replace mode, clear all data buckets first.
	if !merge {
		if err := s.clearDataBuckets(); err != nil {
			return fmt.Errorf("clear data buckets: %w", err)
		}
	}

	// Import all records.
	for _, d := range env.Deployments {
		if err := s.PutDeployment(ctx, d); err != nil {
			return fmt.Errorf("put deployment %s/%d: %w", d.Owner, d.DSeq, err)
		}
	}

	for _, l := range env.Leases {
		if err := s.PutLease(ctx, l); err != nil {
			return fmt.Errorf("put lease: %w", err)
		}
	}

	for _, b := range env.Bids {
		if err := s.PutBid(ctx, b); err != nil {
			return fmt.Errorf("put bid: %w", err)
		}
	}

	if env.SyncState != nil {
		if err := s.PutSyncState(ctx, env.SyncState); err != nil {
			return fmt.Errorf("put sync state: %w", err)
		}
	}

	return nil
}

// clearDataBuckets removes all keys from the deployments, leases, and bids buckets.
func (s *BoltStore) clearDataBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucketDeployments, bucketLeases, bucketBids} {
			// Delete and recreate the bucket to clear all keys atomically.
			if err := tx.DeleteBucket(name); err != nil {
				return fmt.Errorf("delete bucket %s: %w", name, err)
			}
			if _, err := tx.CreateBucket(name); err != nil {
				return fmt.Errorf("recreate bucket %s: %w", name, err)
			}
		}
		return nil
	})
}
