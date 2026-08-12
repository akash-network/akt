package bbolt

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cosmos/cosmos-sdk/codec/address"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
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

// MarshalJSON keeps required collection fields arrays even when a caller builds
// an envelope directly instead of going through export's initialized snapshot.
func (e ExportEnvelope) MarshalJSON() ([]byte, error) {
	type wireEnvelope ExportEnvelope
	normalized := e
	normalized.initializeCollections()
	return json.Marshal(wireEnvelope(normalized))
}

// MarshalYAML gives YAML exports the same collection-shape guarantee as JSON.
func (e ExportEnvelope) MarshalYAML() (any, error) {
	type wireEnvelope ExportEnvelope
	normalized := e
	normalized.initializeCollections()
	return wireEnvelope(normalized), nil
}

func (e *ExportEnvelope) initializeCollections() {
	if e.Deployments == nil {
		e.Deployments = make([]*store.DeploymentRecord, 0)
	}
	if e.Leases == nil {
		e.Leases = make([]*store.LeaseRecord, 0)
	}
	if e.Bids == nil {
		e.Bids = make([]*store.BidRecord, 0)
	}
}

const (
	exportEnvelopeVersion    = 1
	storeSnapshotLockTimeout = 250 * time.Millisecond
	maxEncodedEnvelopeBytes  = 64 << 20
)

// export serializes the store contents to the given writer in the specified format.
func (s *BoltStore) export(ctx context.Context, w io.Writer, format store.ExportFormat, contextName string) error {
	env := ExportEnvelope{
		Version:     exportEnvelopeVersion,
		ExportedAt:  time.Now().UTC().Format(time.RFC3339),
		Deployments: make([]*store.DeploymentRecord, 0),
		Leases:      make([]*store.LeaseRecord, 0),
		Bids:        make([]*store.BidRecord, 0),
		// Serialised all along but never set, so every export claimed to come
		// from context "" and an import could not verify it was being restored
		// into the right place.
		Context: contextName,
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		return readExportSnapshot(ctx, tx, &env)
	}); err != nil {
		return fmt.Errorf("read export snapshot: %w", err)
	}
	if err := validateImportEnvelope(&env); err != nil {
		return fmt.Errorf("validate export snapshot: %w", err)
	}

	size := exportSizeWriter{limit: maxEncodedEnvelopeBytes}
	if err := encodeExportEnvelope(&size, format, &env); err != nil {
		return err
	}

	return encodeExportEnvelope(w, format, &env)
}

func encodeExportEnvelope(w io.Writer, format store.ExportFormat, env *ExportEnvelope) error {
	switch format {
	case store.FormatYAML:
		if _, err := io.WriteString(w, "---\n"); err != nil {
			return fmt.Errorf("write YAML document start: %w", err)
		}
		enc := yaml.NewEncoder(w)
		if err := enc.Encode(env); err != nil {
			_ = enc.Close()
			return fmt.Errorf("encode YAML: %w", err)
		}
		// Close flushes the encoder; a failure here means a truncated export.
		if err := enc.Close(); err != nil {
			return fmt.Errorf("flush YAML export: %w", err)
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

// exportSizeWriter counts an encoded envelope without retaining it. Export
// runs the encoder through this writer before exposing the same immutable
// snapshot to its caller, so an oversized backup cannot leave partial output.
type exportSizeWriter struct {
	written int64
	limit   int64
}

func (w *exportSizeWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining < 0 || int64(len(p)) > remaining {
		return 0, fmt.Errorf("export envelope exceeds %d bytes", w.limit)
	}
	w.written += int64(len(p))

	return len(p), nil
}

func readExportSnapshot(ctx context.Context, tx *bolt.Tx, env *ExportEnvelope) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	meta, err := requiredExportBucket(tx, bucketMeta)
	if err != nil {
		return err
	}
	syncBucket, err := requiredExportBucket(tx, bucketSync)
	if err != nil {
		return err
	}
	deployments, err := requiredExportBucket(tx, bucketDeployments)
	if err != nil {
		return err
	}
	leases, err := requiredExportBucket(tx, bucketLeases)
	if err != nil {
		return err
	}
	bids, err := requiredExportBucket(tx, bucketBids)
	if err != nil {
		return err
	}

	schema := meta.Get(keySchemaVersion)
	if len(schema) != 8 {
		return fmt.Errorf("schema version metadata is missing or malformed")
	}
	env.SchemaVersion = binary.BigEndian.Uint64(schema)

	if data := syncBucket.Get(keySyncState); data != nil {
		var syncState store.SyncState
		if err := json.Unmarshal(data, &syncState); err != nil {
			return fmt.Errorf("decode sync record %q: %w", keySyncState, err)
		}
		env.SyncState = &syncState
	}

	if err := deployments.ForEach(func(key, value []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var record store.DeploymentRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return fmt.Errorf("decode deployments record %q: %w", key, err)
		}
		env.Deployments = append(env.Deployments, &record)
		return nil
	}); err != nil {
		return err
	}

	if err := leases.ForEach(func(key, value []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var record store.LeaseRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return fmt.Errorf("decode leases record %q: %w", key, err)
		}
		env.Leases = append(env.Leases, &record)
		return nil
	}); err != nil {
		return err
	}

	if err := bids.ForEach(func(key, value []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var record store.BidRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return fmt.Errorf("decode bids record %q: %w", key, err)
		}
		env.Bids = append(env.Bids, &record)
		return nil
	}); err != nil {
		return err
	}

	return ctx.Err()
}

func requiredExportBucket(tx *bolt.Tx, name []byte) (*bolt.Bucket, error) {
	bucket := tx.Bucket(name)
	if bucket == nil {
		return nil, fmt.Errorf("database corruption: required bucket %q is missing", name)
	}

	return bucket, nil
}

// importData reads store contents from the given reader and loads them into the store.
// If merge is false, all existing data buckets are cleared first (replace mode).
func (s *BoltStore) importData(ctx context.Context, r io.Reader, format store.ExportFormat, merge bool) error {
	env, err := decodeImportEnvelope(r, format)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		return applyImportTx(ctx, tx, env, merge)
	})
}

// validateImportData applies an import in a transaction that is deliberately
// rolled back. It therefore exercises the same version checks and writes as a
// real import without committing any logical store changes.
func (s *BoltStore) validateImportData(ctx context.Context, r io.Reader, format store.ExportFormat, merge bool) error {
	env, err := decodeImportEnvelope(r, format)
	if err != nil {
		return err
	}

	dryRunRollback := errors.New("dry-run rollback")
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := applyImportTx(ctx, tx, env, merge); err != nil {
			return err
		}

		return dryRunRollback
	})
	if errors.Is(err, dryRunRollback) {
		return nil
	}

	return err
}

// ValidateImportSnapshot validates an import against a disposable copy of the
// store at sourcePath. It exercises migrations and merge-time write conflicts
// without opening, creating, or modifying the selected context's database.
func ValidateImportSnapshot(ctx context.Context, sourcePath string, r io.Reader, format store.ExportFormat, merge bool) error {
	return validateImportSnapshot(
		ctx,
		sourcePath,
		r,
		format,
		merge,
		func() (importSnapshotFile, error) {
			return os.CreateTemp("", "akt-store-import-dry-run-*.db")
		},
		func(path string) (importSnapshotStore, error) {
			return Open(path)
		},
	)
}

type importSnapshotFile interface {
	io.Writer
	Name() string
	Close() error
}

type importSnapshotStore interface {
	Migrate(context.Context) error
	ValidateImport(context.Context, io.Reader, store.ExportFormat, bool) error
	Close() error
}

func validateImportSnapshot(
	ctx context.Context,
	sourcePath string,
	r io.Reader,
	format store.ExportFormat,
	merge bool,
	createTemp func() (importSnapshotFile, error),
	openStore func(string) (importSnapshotStore, error),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tempFile, err := createTemp()
	if err != nil {
		return fmt.Errorf("create disposable store: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()

	info, statErr := os.Stat(sourcePath)
	switch {
	case statErr == nil && info.Size() > 0:
		if err := copyStoreSnapshot(ctx, sourcePath, tempFile); err != nil {
			_ = tempFile.Close()
			return err
		}
	case statErr == nil:
		// A zero-length file is initialized by a real Open call. Leave the
		// disposable file empty so the same initialization happens below.
	case os.IsNotExist(statErr):
		// A missing selected-context database is represented by a disposable
		// empty store. In particular, do not create its parent directory.
	default:
		_ = tempFile.Close()
		return fmt.Errorf("inspect store %s: %w", sourcePath, statErr)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close disposable store: %w", err)
	}

	snapshot, err := openStore(tempPath)
	if err != nil {
		return fmt.Errorf("open disposable store: %w", err)
	}
	if err := snapshot.Migrate(ctx); err != nil {
		_ = snapshot.Close()
		return fmt.Errorf("migrate disposable store: %w", err)
	}

	validateErr := snapshot.ValidateImport(ctx, r, format, merge)
	closeErr := snapshot.Close()
	if validateErr != nil {
		return validateErr
	}
	if closeErr != nil {
		return fmt.Errorf("close disposable store: %w", closeErr)
	}

	return nil
}

func copyStoreSnapshot(ctx context.Context, sourcePath string, destination io.Writer) error {
	return copyStoreSnapshotWithOpen(
		ctx,
		sourcePath,
		destination,
		func(path string, timeout time.Duration) (readOnlySnapshot, error) {
			return bolt.Open(path, 0600, &bolt.Options{ReadOnly: true, Timeout: timeout})
		},
	)
}

type readOnlySnapshot interface {
	View(func(*bolt.Tx) error) error
	Close() error
}

func copyStoreSnapshotWithOpen(
	ctx context.Context,
	sourcePath string,
	destination io.Writer,
	openSource func(string, time.Duration) (readOnlySnapshot, error),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	lockTimeout := storeSnapshotLockTimeout
	deadlineBounded := false
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			// A deadline may elapse just before a context implementation closes
			// Done and updates Err. Returning ctx.Err() in that interval would
			// report a false success without copying a snapshot.
			return context.DeadlineExceeded
		}
		if remaining < lockTimeout {
			lockTimeout = remaining
			deadlineBounded = true
		}
	}

	source, err := openSource(sourcePath, lockTimeout)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if deadlineBounded && errors.Is(err, bolterrors.ErrTimeout) {
			return fmt.Errorf("open store snapshot %s before context deadline: %w", sourcePath, context.DeadlineExceeded)
		}
		return fmt.Errorf("open store snapshot %s: %w", sourcePath, err)
	}

	copyErr := source.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := tx.WriteTo(destination); err != nil {
			return fmt.Errorf("copy store snapshot: %w", err)
		}
		return ctx.Err()
	})
	closeErr := source.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return fmt.Errorf("close store snapshot: %w", closeErr)
	}

	return nil
}

func decodeImportEnvelope(r io.Reader, format store.ExportFormat) (*ExportEnvelope, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxEncodedEnvelopeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if len(data) > maxEncodedEnvelopeBytes {
		return nil, fmt.Errorf("import input exceeds %d bytes", maxEncodedEnvelopeBytes)
	}

	var env ExportEnvelope
	switch format {
	case store.FormatYAML:
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&env); err != nil {
			return nil, fmt.Errorf("unmarshal YAML: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err == nil {
			return nil, fmt.Errorf("trailing YAML document")
		} else if !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("decode trailing YAML data: %w", err)
		}
	case store.FormatJSON:
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&env); err != nil {
			return nil, fmt.Errorf("unmarshal JSON: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err == nil {
			return nil, fmt.Errorf("trailing JSON document")
		} else if !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("decode trailing JSON data: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported import format: %d", format)
	}
	if err := validateRequiredImportCollections(data, format); err != nil {
		return nil, err
	}

	if err := validateImportEnvelope(&env); err != nil {
		return nil, fmt.Errorf("validate import: %w", err)
	}

	return &env, nil
}

func validateRequiredImportCollections(data []byte, format store.ExportFormat) error {
	const requiredArrayError = "%s is required and must be an array"

	switch format {
	case store.FormatJSON:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return fmt.Errorf("inspect JSON envelope: %w", err)
		}
		for _, name := range []string{"deployments", "leases", "bids"} {
			raw, ok := fields[name]
			if !ok || len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || bytes.TrimSpace(raw)[0] != '[' {
				return fmt.Errorf(requiredArrayError, name)
			}
		}
	case store.FormatYAML:
		var document yaml.Node
		if err := yaml.Unmarshal(data, &document); err != nil {
			return fmt.Errorf("inspect YAML envelope: %w", err)
		}
		if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
			return errors.New("import envelope must be a mapping")
		}
		root := document.Content[0]
		for _, name := range []string{"deployments", "leases", "bids"} {
			found := false
			for i := 0; i+1 < len(root.Content); i += 2 {
				if root.Content[i].Value == name {
					found = root.Content[i+1].Kind == yaml.SequenceNode
					break
				}
			}
			if !found {
				return fmt.Errorf(requiredArrayError, name)
			}
		}
	default:
		return fmt.Errorf("unsupported import format: %d", format)
	}

	return nil
}

func applyImportTx(ctx context.Context, tx *bolt.Tx, env *ExportEnvelope, merge bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if !merge {
		if err := clearDataBucketsTx(tx); err != nil {
			return fmt.Errorf("clear data buckets: %w", err)
		}
	}

	for _, d := range env.Deployments {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := putDeploymentTx(tx, d); err != nil {
			return fmt.Errorf("put deployment %s/%d: %w", d.Owner, d.DSeq, err)
		}
	}

	for _, l := range env.Leases {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := putLeaseTx(tx, l); err != nil {
			return fmt.Errorf("put lease %s: %w", store.LeaseKey(l.ID), err)
		}
	}

	for _, b := range env.Bids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := putBidTx(tx, b); err != nil {
			return fmt.Errorf("put bid %s: %w", store.BidKey(b.ID), err)
		}
	}

	if env.SyncState != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := putSyncStateTx(tx, env.SyncState); err != nil {
			return fmt.Errorf("put sync state: %w", err)
		}
	}

	return nil
}

func validateImportEnvelope(env *ExportEnvelope) error {
	if env.Version != exportEnvelopeVersion {
		return fmt.Errorf("unsupported export version %d (want %d)", env.Version, exportEnvelopeVersion)
	}
	if env.SchemaVersion == 0 || env.SchemaVersion > currentSchemaVersion {
		return fmt.Errorf("unsupported schema version %d (current %d)", env.SchemaVersion, currentSchemaVersion)
	}

	deployments := make(map[string]struct{}, len(env.Deployments))
	for i, d := range env.Deployments {
		if d == nil {
			return fmt.Errorf("deployment %d is null", i)
		}
		if d.DSeq == 0 {
			return fmt.Errorf("deployment %d has invalid identity: dseq is required", i)
		}
		if err := validateImportAccountAddress(d.Owner); err != nil {
			return fmt.Errorf("deployment %d has invalid identity: owner: %w", i, err)
		}
		switch d.State {
		case "active", "closed":
		default:
			return fmt.Errorf("deployment %d has invalid state %q", i, d.State)
		}
		if d.CreatedAt < 0 || d.UpdatedAt < 0 || d.ClosedAt < 0 || d.CreatedHeight < 0 {
			return fmt.Errorf("deployment %d has negative timestamp or height", i)
		}
		key := store.DeploymentKey(d.Owner, d.DSeq)
		if _, exists := deployments[key]; exists {
			return fmt.Errorf("deployment %d duplicates %s", i, key)
		}
		deployments[key] = struct{}{}
	}

	leases := make(map[string]struct{}, len(env.Leases))
	for i, l := range env.Leases {
		if l == nil {
			return fmt.Errorf("lease %d is null", i)
		}
		if l.ID.DSeq == 0 || l.ID.GSeq == 0 || l.ID.OSeq == 0 {
			return fmt.Errorf("lease %d has invalid identity: dseq, gseq, and oseq are required", i)
		}
		if err := validateImportAccountAddress(l.ID.Owner); err != nil {
			return fmt.Errorf("lease %d has invalid identity: owner: %w", i, err)
		}
		if err := validateImportAccountAddress(l.ID.Provider); err != nil {
			return fmt.Errorf("lease %d has invalid identity: provider: %w", i, err)
		}
		switch l.State {
		case "active", "closed", "insufficient_funds":
		default:
			return fmt.Errorf("lease %d has invalid state %q", i, l.State)
		}
		if l.CreatedAt < 0 || l.ClosedAt < 0 {
			return fmt.Errorf("lease %d has negative timestamp", i)
		}
		key := store.LeaseKey(l.ID)
		if _, exists := leases[key]; exists {
			return fmt.Errorf("lease %d duplicates %s", i, key)
		}
		leases[key] = struct{}{}
	}

	bids := make(map[string]struct{}, len(env.Bids))
	for i, b := range env.Bids {
		if b == nil {
			return fmt.Errorf("bid %d is null", i)
		}
		if b.ID.DSeq == 0 || b.ID.GSeq == 0 || b.ID.OSeq == 0 {
			return fmt.Errorf("bid %d has invalid identity: dseq, gseq, and oseq are required", i)
		}
		if err := validateImportAccountAddress(b.ID.Owner); err != nil {
			return fmt.Errorf("bid %d has invalid identity: owner: %w", i, err)
		}
		if err := validateImportAccountAddress(b.ID.Provider); err != nil {
			return fmt.Errorf("bid %d has invalid identity: provider: %w", i, err)
		}
		switch b.State {
		case "open", "matched", "closed", "lost":
		default:
			return fmt.Errorf("bid %d has invalid state %q", i, b.State)
		}
		if b.CreatedAt < 0 {
			return fmt.Errorf("bid %d has negative timestamp", i)
		}
		key := store.BidKey(b.ID)
		if _, exists := bids[key]; exists {
			return fmt.Errorf("bid %d duplicates %s", i, key)
		}
		bids[key] = struct{}{}
	}

	if env.SyncState != nil {
		if env.SyncState.LastBlockHeight < 0 || env.SyncState.LastSyncTime < 0 {
			return fmt.Errorf("sync state has negative height or timestamp")
		}
		if env.SyncState.SchemaVersion > currentSchemaVersion {
			return fmt.Errorf("sync state schema version %d exceeds current %d", env.SyncState.SchemaVersion, currentSchemaVersion)
		}
		for i, account := range env.SyncState.TrackedAccounts {
			if err := validateImportAccountAddress(account); err != nil {
				return fmt.Errorf("sync state tracked account %d has invalid identity: %w", i, err)
			}
		}
	}

	return nil
}

func validateImportAccountAddress(value string) error {
	return validateImportAccountAddressWithCodec(value, address.NewBech32Codec("akash"))
}

type importAccountAddressCodec interface {
	StringToBytes(string) ([]byte, error)
	BytesToString([]byte) (string, error)
}

func validateImportAccountAddressWithCodec(value string, codec importAccountAddressCodec) error {
	decoded, err := codec.StringToBytes(value)
	if err != nil {
		return fmt.Errorf("invalid akash bech32 address: %w", err)
	}
	if len(decoded) != 20 {
		return fmt.Errorf("invalid akash account address length %d (want 20)", len(decoded))
	}
	canonical, err := codec.BytesToString(decoded)
	if err != nil {
		return fmt.Errorf("re-encode akash address: %w", err)
	}
	if canonical != value {
		return fmt.Errorf("address is not canonical (want %s)", canonical)
	}
	return nil
}

// clearDataBucketsTx removes all imported logical state within the caller's
// transaction. The metadata bucket is retained because it describes the local
// database schema, not the imported snapshot.
func clearDataBucketsTx(tx *bolt.Tx) error {
	return clearDataBucketsTxWithCreate(tx, func(tx *bolt.Tx, name []byte) (*bolt.Bucket, error) {
		return tx.CreateBucket(name)
	})
}

func clearDataBucketsTxWithCreate(
	tx *bolt.Tx,
	createBucket func(*bolt.Tx, []byte) (*bolt.Bucket, error),
) error {
	for _, name := range [][]byte{bucketDeployments, bucketLeases, bucketBids, bucketSync} {
		if err := tx.DeleteBucket(name); err != nil {
			return fmt.Errorf("delete bucket %s: %w", name, err)
		}
		if _, err := createBucket(tx, name); err != nil {
			return fmt.Errorf("recreate bucket %s: %w", name, err)
		}
	}

	return nil
}
