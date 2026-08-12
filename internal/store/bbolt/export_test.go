package bbolt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
	"gopkg.in/yaml.v3"

	"pkg.akt.dev/akt/internal/store"
)

// seedStore populates a store with test data: 2 deployments, 1 lease, 1 bid, 1 sync state.
func seedStore(t *testing.T, s *BoltStore) {
	t.Helper()
	ctx := context.Background()

	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:   "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:    1,
		State:   "active",
		SDLHash: "sha256:aaa",
		Deposit: "5000000uakt",
		Labels:  map[string]string{"env": "prod"},
		Tags:    []string{"web"},
	}))
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:   "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:    2,
		State:   "closed",
		SDLHash: "sha256:bbb",
		Deposit: "1000000uakt",
	}))
	require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
		ID: store.LeaseID{
			Owner:    "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
			DSeq:     1,
			GSeq:     1,
			OSeq:     1,
			Provider: "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
		},
		State:       "active",
		Price:       "100uakt",
		ProviderURI: "https://provider.example.com",
	}))
	require.NoError(t, s.PutBid(ctx, &store.BidRecord{
		ID: store.BidID{
			Owner:    "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
			DSeq:     1,
			GSeq:     1,
			OSeq:     1,
			Provider: "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
		},
		State: "open",
		Price: "50uakt",
	}))
	require.NoError(t, s.PutSyncState(ctx, &store.SyncState{
		LastBlockHeight: 18234567,
		LastSyncTime:    1742724932,
		TrackedAccounts: []string{"akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"},
		SchemaVersion:   1,
	}))
}

func TestExportYAML(t *testing.T) {
	s := openTestStore(t)
	seedStore(t, s)

	var buf bytes.Buffer
	err := s.Export(context.Background(), &buf, store.FormatYAML, "testctx")
	require.NoError(t, err)

	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "---\n"), "YAML output should start with document marker")
	assert.Contains(t, out, "version: 1")
	assert.Contains(t, out, "schema_version: 1")
	assert.Contains(t, out, "exported_at:")
	assert.Contains(t, out, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx")
	assert.Contains(t, out, "dseq: 1")
	assert.Contains(t, out, "dseq: 2")
	assert.Contains(t, out, "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl")
	assert.Contains(t, out, "last_block_height: 18234567")
}

func TestExportJSON(t *testing.T) {
	s := openTestStore(t)
	seedStore(t, s)

	var buf bytes.Buffer
	err := s.Export(context.Background(), &buf, store.FormatJSON, "testctx")
	require.NoError(t, err)

	out := buf.Bytes()

	// Verify it's valid JSON.
	var env ExportEnvelope
	require.NoError(t, json.Unmarshal(out, &env))

	assert.Equal(t, 1, env.Version)
	assert.Equal(t, uint64(1), env.SchemaVersion)
	assert.NotEmpty(t, env.ExportedAt)
	assert.Len(t, env.Deployments, 2)
	assert.Len(t, env.Leases, 1)
	assert.Len(t, env.Bids, 1)
	assert.Equal(t, uint64(1), env.Deployments[0].RecordVersion)
	assert.Equal(t, uint64(1), env.Leases[0].RecordVersion)
	assert.Equal(t, uint64(1), env.Bids[0].RecordVersion)
	require.NotNil(t, env.SyncState)
	assert.Equal(t, int64(18234567), env.SyncState.LastBlockHeight)
}

func TestEmptyExportCollectionsAreArraysInJSONAndYAML(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		format store.ExportFormat
		decode func([]byte, *map[string]any) error
	}{
		{
			name:   "json",
			format: store.FormatJSON,
			decode: func(data []byte, out *map[string]any) error { return json.Unmarshal(data, out) },
		},
		{
			name:   "yaml",
			format: store.FormatYAML,
			decode: func(data []byte, out *map[string]any) error { return yaml.Unmarshal(data, out) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, s.Export(ctx, &buf, tc.format, "testctx"))

			var document map[string]any
			require.NoError(t, tc.decode(buf.Bytes(), &document))
			for _, field := range []string{"deployments", "leases", "bids"} {
				value, ok := document[field]
				require.True(t, ok, "missing %s", field)
				require.IsType(t, []any{}, value, "%s must be an array", field)
				require.Empty(t, value)
			}
		})
	}
}

func TestExportFailsInsteadOfOmittingCorruptSnapshotRows(t *testing.T) {
	for _, bucket := range [][]byte{bucketDeployments, bucketLeases, bucketBids, bucketSync} {
		t.Run(string(bucket), func(t *testing.T) {
			s := openTestStore(t)
			seedStore(t, s)
			recordKey := []byte("corrupt-record")
			if string(bucket) == string(bucketSync) {
				recordKey = keySyncState
			}
			require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(bucket).Put(recordKey, []byte("not-json"))
			}))

			var output bytes.Buffer
			err := s.Export(context.Background(), &output, store.FormatJSON, "testctx")
			require.Error(t, err)
			require.Contains(t, err.Error(), string(bucket))
			require.Empty(t, output.Bytes(), "a failed backup must not look like a valid partial export")
		})
	}
}

func TestExportRejectsMissingRequiredBucketsWithoutPanic(t *testing.T) {
	for _, bucket := range [][]byte{bucketMeta, bucketSync, bucketDeployments, bucketLeases, bucketBids} {
		t.Run(string(bucket), func(t *testing.T) {
			s := openTestStore(t)
			require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
				return tx.DeleteBucket(bucket)
			}))

			var output bytes.Buffer
			var exportErr error
			require.NotPanics(t, func() {
				exportErr = s.Export(context.Background(), &output, store.FormatJSON, "testctx")
			})
			require.ErrorContains(t, exportErr, "database corruption")
			require.ErrorContains(t, exportErr, string(bucket))
			require.Empty(t, output.Bytes())
		})
	}
}

func TestExportRejectsSnapshotItsImporterWouldReject(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(context.Background(), &store.DeploymentRecord{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:  1,
		State: "pending",
	}))

	var output bytes.Buffer
	err := s.Export(context.Background(), &output, store.FormatJSON, "testctx")
	require.ErrorContains(t, err, "validate export snapshot")
	require.ErrorContains(t, err, `invalid state "pending"`)
	require.Empty(t, output.Bytes())
}

func TestExportSizeLimitRejectsByteBeyond64MiB(t *testing.T) {
	w := exportSizeWriter{limit: maxEncodedEnvelopeBytes}
	chunk := make([]byte, 32<<10)
	for remaining := int64(maxEncodedEnvelopeBytes); remaining > 0; remaining -= int64(len(chunk)) {
		n, err := w.Write(chunk)
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
	}
	require.Equal(t, int64(maxEncodedEnvelopeBytes), w.written)

	n, err := w.Write([]byte{0})
	require.Zero(t, n)
	require.ErrorContains(t, err, "export envelope exceeds 67108864 bytes")
}

func TestExportRejectsMissingSchemaMetadataAndCancellation(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Delete(keySchemaVersion)
	}))

	var output bytes.Buffer
	err := s.Export(context.Background(), &output, store.FormatJSON, "testctx")
	require.ErrorContains(t, err, "schema version")
	require.Empty(t, output.Bytes())

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	err = s.Export(cancelled, &output, store.FormatJSON, "testctx")
	require.ErrorIs(t, err, context.Canceled)
}

func TestExportRejectsMalformedSchemaAndWriterFailures(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put(keySchemaVersion, []byte{1})
	}))

	var output bytes.Buffer
	err := s.Export(context.Background(), &output, store.FormatJSON, "testctx")
	require.ErrorContains(t, err, "schema version metadata is missing or malformed")
	require.Empty(t, output.Bytes())

	// Restore valid metadata so the remaining cases fail at serialization,
	// after the snapshot has been read successfully.
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		var schema [8]byte
		schema[7] = byte(currentSchemaVersion)
		return tx.Bucket(bucketMeta).Put(keySchemaVersion, schema[:])
	}))

	err = s.Export(context.Background(), &failAfterWriter{err: errors.New("write failed")}, store.FormatJSON, "testctx")
	require.ErrorContains(t, err, "encode JSON: write failed")

	err = s.Export(context.Background(), &failAfterWriter{err: errors.New("write failed")}, store.FormatYAML, "testctx")
	require.ErrorContains(t, err, "write YAML document start: write failed")

	err = s.Export(context.Background(), &failAfterWriter{
		remaining: len("---\n"),
		err:       errors.New("write failed"),
	}, store.FormatYAML, "testctx")
	require.ErrorContains(t, err, "encode YAML")
	require.ErrorContains(t, err, "write failed")

	err = s.Export(context.Background(), &output, store.ExportFormat(99), "testctx")
	require.ErrorContains(t, err, "unsupported export format: 99")
}

type failAfterWriter struct {
	remaining int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	if len(p) > w.remaining {
		written := w.remaining
		w.remaining = 0
		return written, w.err
	}
	w.remaining -= len(p)
	return len(p), nil
}

func TestExportCancellationDuringSnapshotProducesNoOutput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		remaining int
	}{
		{name: "deployments", remaining: 2},
		{name: "leases", remaining: 3},
		{name: "bids", remaining: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			seedStore(t, s)

			ctx := &cancelAfterContextChecks{
				Context:   context.Background(),
				remaining: tc.remaining,
			}
			var output bytes.Buffer
			err := s.Export(ctx, &output, store.FormatJSON, "testctx")
			require.ErrorIs(t, err, context.Canceled)
			require.Empty(t, output.Bytes())
		})
	}
}

func TestImportMerge(t *testing.T) {
	ctx := context.Background()

	// Store A: has deployment dseq=1.
	storeA := openTestStore(t)
	require.NoError(t, storeA.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:  1,
		State: "active",
	}))

	// Export store A.
	var buf bytes.Buffer
	require.NoError(t, storeA.Export(ctx, &buf, store.FormatJSON, "testctx"))

	// Store B: has deployment dseq=2.
	storeB := openTestStore(t)
	require.NoError(t, storeB.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:  2,
		State: "active",
	}))

	// Import A's export into B with merge=true.
	require.NoError(t, storeB.Import(ctx, &buf, store.FormatJSON, true))

	// B should have both dseq=1 and dseq=2.
	deps, err := storeB.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	assert.Len(t, deps, 2)

	d1, err := storeB.GetDeployment(ctx, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", 1)
	require.NoError(t, err)
	assert.NotNil(t, d1)

	d2, err := storeB.GetDeployment(ctx, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", 2)
	require.NoError(t, err)
	assert.NotNil(t, d2)
}

func TestImportReplace(t *testing.T) {
	ctx := context.Background()

	// Store has deployment dseq=1.
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:  1,
		State: "active",
	}))

	// Build an import payload with only dseq=2.
	env := ExportEnvelope{
		Version:       1,
		SchemaVersion: 1,
		Deployments: []*store.DeploymentRecord{
			{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "closed"},
		},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	// Import with merge=false (replace).
	require.NoError(t, s.Import(ctx, bytes.NewReader(data), store.FormatJSON, false))

	// Store should have ONLY dseq=2.
	deps, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	assert.Len(t, deps, 1)
	assert.Equal(t, uint64(2), deps[0].DSeq)

	d1, err := s.GetDeployment(ctx, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", 1)
	require.NoError(t, err)
	assert.Nil(t, d1)
}

func TestRoundTripYAML(t *testing.T) {
	ctx := context.Background()

	// Populate and export from store 1.
	s1 := openTestStore(t)
	seedStore(t, s1)

	var export1 bytes.Buffer
	require.NoError(t, s1.Export(ctx, &export1, store.FormatYAML, "testctx"))

	// Import into store 2.
	s2 := openTestStore(t)
	require.NoError(t, s2.Import(ctx, bytes.NewReader(export1.Bytes()), store.FormatYAML, false))

	// Export from store 2.
	var export2 bytes.Buffer
	require.NoError(t, s2.Export(ctx, &export2, store.FormatYAML, "testctx"))

	// Parse both exports and compare data (ignoring exported_at timestamp).
	var env1, env2 ExportEnvelope
	require.NoError(t, decodeYAMLEnvelope(export1.Bytes(), &env1))
	require.NoError(t, decodeYAMLEnvelope(export2.Bytes(), &env2))

	assert.Equal(t, env1.Version, env2.Version)
	assert.Equal(t, env1.SchemaVersion, env2.SchemaVersion)
	assert.Equal(t, env1.Deployments, env2.Deployments)
	assert.Equal(t, env1.Leases, env2.Leases)
	assert.Equal(t, env1.Bids, env2.Bids)
	assert.Equal(t, env1.SyncState, env2.SyncState)
}

func TestRoundTripJSON(t *testing.T) {
	ctx := context.Background()

	// Populate and export from store 1.
	s1 := openTestStore(t)
	seedStore(t, s1)

	var export1 bytes.Buffer
	require.NoError(t, s1.Export(ctx, &export1, store.FormatJSON, "testctx"))

	// Import into store 2.
	s2 := openTestStore(t)
	require.NoError(t, s2.Import(ctx, bytes.NewReader(export1.Bytes()), store.FormatJSON, false))

	// Export from store 2.
	var export2 bytes.Buffer
	require.NoError(t, s2.Export(ctx, &export2, store.FormatJSON, "testctx"))

	// Parse both and compare data (ignoring exported_at).
	var env1, env2 ExportEnvelope
	require.NoError(t, json.Unmarshal(export1.Bytes(), &env1))
	require.NoError(t, json.Unmarshal(export2.Bytes(), &env2))

	assert.Equal(t, env1.Version, env2.Version)
	assert.Equal(t, env1.SchemaVersion, env2.SchemaVersion)
	assert.Equal(t, env1.Deployments, env2.Deployments)
	assert.Equal(t, env1.Leases, env2.Leases)
	assert.Equal(t, env1.Bids, env2.Bids)
	assert.Equal(t, env1.SyncState, env2.SyncState)
}

func TestImportEmptyStore(t *testing.T) {
	ctx := context.Background()

	// Populate and export from a seeded store.
	src := openTestStore(t)
	seedStore(t, src)

	var buf bytes.Buffer
	require.NoError(t, src.Export(ctx, &buf, store.FormatJSON, "testctx"))

	// Import into a fresh empty store.
	dst := openTestStore(t)
	require.NoError(t, dst.Import(ctx, bytes.NewReader(buf.Bytes()), store.FormatJSON, false))

	// Verify all records are present.
	deps, err := dst.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	assert.Len(t, deps, 2)

	leases, err := dst.ListLeases(ctx, store.LeaseFilter{})
	require.NoError(t, err)
	assert.Len(t, leases, 1)

	bids, err := dst.ListBids(ctx, store.BidFilter{})
	require.NoError(t, err)
	assert.Len(t, bids, 1)

	syncState, err := dst.GetSyncState(ctx)
	require.NoError(t, err)
	require.NotNil(t, syncState)
	assert.Equal(t, int64(18234567), syncState.LastBlockHeight)
	assert.Equal(t, int64(1742724932), syncState.LastSyncTime)
}

func TestImportRejectsNilRecordWithoutMutation(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	seedStore(t, s)

	env := ExportEnvelope{
		Version:       1,
		SchemaVersion: currentSchemaVersion,
		Deployments:   []*store.DeploymentRecord{nil},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	var importErr error
	require.NotPanics(t, func() {
		importErr = s.Import(ctx, bytes.NewReader(data), store.FormatJSON, false)
	})
	require.ErrorContains(t, importErr, "deployment 0")

	deployments, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	require.Len(t, deployments, 2)

	syncState, err := s.GetSyncState(ctx)
	require.NoError(t, err)
	require.NotNil(t, syncState)
	require.Equal(t, int64(18234567), syncState.LastBlockHeight)
}

func TestImportRejectsMissingOrNullCollectionsWithoutMutation(t *testing.T) {
	jsonEnvelope := `{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`
	yamlEnvelope := `version: 1
schema_version: 1
deployments: []
leases: []
bids: []
`

	for _, format := range []struct {
		name     string
		format   store.ExportFormat
		envelope string
	}{
		{name: "JSON", format: store.FormatJSON, envelope: jsonEnvelope},
		{name: "YAML", format: store.FormatYAML, envelope: yamlEnvelope},
	} {
		for _, collection := range []string{"deployments", "leases", "bids"} {
			for _, mutation := range []string{"missing", "null"} {
				t.Run(format.name+"/"+collection+"/"+mutation, func(t *testing.T) {
					input := format.envelope
					if format.format == store.FormatJSON {
						field := `"` + collection + `":[]`
						if mutation == "missing" {
							input = strings.Replace(input, field+",", "", 1)
							input = strings.Replace(input, ","+field, "", 1)
						} else {
							input = strings.Replace(input, field, `"`+collection+`":null`, 1)
						}
					} else {
						field := collection + ": []"
						if mutation == "missing" {
							input = strings.Replace(input, field+"\n", "", 1)
						} else {
							input = strings.Replace(input, field, collection+": null", 1)
						}
					}

					s := openTestStore(t)
					seedStore(t, s)
					err := s.Import(context.Background(), strings.NewReader(input), format.format, false)
					require.ErrorContains(t, err, collection+" is required and must be an array")
					assertSeededStoreUnchanged(t, s)
				})
			}
		}
	}
}

func TestImportRejectsUnknownFieldsAndTrailingDocumentsWithoutMutation(t *testing.T) {
	tests := []struct {
		name      string
		format    store.ExportFormat
		input     string
		wantError string
	}{
		{
			name:      "JSON unknown envelope field",
			format:    store.FormatJSON,
			input:     `{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[],"unexpected":true}`,
			wantError: `unknown field "unexpected"`,
		},
		{
			name:   "JSON unknown record field",
			format: store.FormatJSON,
			input: `{"version":1,"schema_version":1,"deployments":[
  {"owner":"akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk","dseq":9,"state":"active","unexpected":true}
],"leases":[],"bids":[]}`,
			wantError: `unknown field "unexpected"`,
		},
		{
			name:      "JSON trailing document",
			format:    store.FormatJSON,
			input:     `{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]} {"version":1}`,
			wantError: "trailing JSON document",
		},
		{
			name:   "YAML unknown envelope field",
			format: store.FormatYAML,
			input: `version: 1
schema_version: 1
deployments: []
leases: []
bids: []
unexpected: true
`,
			wantError: "field unexpected not found",
		},
		{
			name:   "YAML unknown record field",
			format: store.FormatYAML,
			input: `version: 1
schema_version: 1
deployments:
  - owner: akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk
    dseq: 9
    state: active
    unexpected: true
leases: []
bids: []
`,
			wantError: "field unexpected not found",
		},
		{
			name:   "YAML trailing document",
			format: store.FormatYAML,
			input: `version: 1
schema_version: 1
deployments: []
leases: []
bids: []
---
version: 1
`,
			wantError: "trailing YAML document",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			seedStore(t, s)

			err := s.Import(ctx, strings.NewReader(tc.input), tc.format, false)
			require.ErrorContains(t, err, tc.wantError)

			assertSeededStoreUnchanged(t, s)
		})
	}
}

func TestDecodeImportEnvelopeRejectsUnreadableUnsupportedAndMalformedTrailingInput(t *testing.T) {
	_, err := decodeImportEnvelope(iotest.ErrReader(errors.New("read failed")), store.FormatJSON)
	require.ErrorContains(t, err, "read input: read failed")

	_, err = decodeImportEnvelope(strings.NewReader(`{}`), store.ExportFormat(99))
	require.ErrorContains(t, err, "unsupported import format: 99")

	_, err = decodeImportEnvelope(strings.NewReader(
		`{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]} {`,
	), store.FormatJSON)
	require.ErrorContains(t, err, "decode trailing JSON data")

	_, err = decodeImportEnvelope(strings.NewReader(`version: 1
schema_version: 1
deployments: []
leases: []
bids: []
---
invalid: [
`), store.FormatYAML)
	require.ErrorContains(t, err, "decode trailing YAML data")

	_, err = decodeImportEnvelope(io.LimitReader(repeatingByteReader('x'), maxEncodedEnvelopeBytes+1), store.FormatJSON)
	require.ErrorContains(t, err, "import input exceeds 67108864 bytes")
}

func TestRequiredImportCollectionInspectionRejectsMalformedDocuments(t *testing.T) {
	err := validateRequiredImportCollections([]byte(`[`), store.FormatJSON)
	require.ErrorContains(t, err, "inspect JSON envelope")

	err = validateRequiredImportCollections([]byte("deployments: [\n"), store.FormatYAML)
	require.ErrorContains(t, err, "inspect YAML envelope")

	err = validateRequiredImportCollections([]byte("[]\n"), store.FormatYAML)
	require.ErrorContains(t, err, "import envelope must be a mapping")
}

type repeatingByteReader byte

func (r repeatingByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}

	return len(p), nil
}

func assertSeededStoreUnchanged(t *testing.T, s *BoltStore) {
	t.Helper()
	ctx := context.Background()

	deployments, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	require.Len(t, deployments, 2)
	require.Equal(t, "active", deployments[0].State)
	require.Equal(t, "closed", deployments[1].State)

	leases, err := s.ListLeases(ctx, store.LeaseFilter{})
	require.NoError(t, err)
	require.Len(t, leases, 1)
	require.Equal(t, "active", leases[0].State)

	bids, err := s.ListBids(ctx, store.BidFilter{})
	require.NoError(t, err)
	require.Len(t, bids, 1)
	require.Equal(t, "open", bids[0].State)

	syncState, err := s.GetSyncState(ctx)
	require.NoError(t, err)
	require.NotNil(t, syncState)
	require.Equal(t, int64(18234567), syncState.LastBlockHeight)
}

func TestImportRollsBackEarlierRecordsWhenLaterWriteFails(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:  1,
		State: "active",
	}))
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:         "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:          2,
		State:         "active",
		RecordVersion: math.MaxUint64,
	}))

	env := ExportEnvelope{
		Version:       1,
		SchemaVersion: currentSchemaVersion,
		Deployments: []*store.DeploymentRecord{
			{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 1, State: "closed"},
			{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "closed", RecordVersion: math.MaxUint64},
		},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	err = s.Import(ctx, bytes.NewReader(data), store.FormatJSON, true)
	require.ErrorContains(t, err, "record revision cannot advance")

	unchanged, err := s.GetDeployment(ctx, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", 1)
	require.NoError(t, err)
	require.NotNil(t, unchanged)
	require.Equal(t, "active", unchanged.State)
	require.Equal(t, uint64(1), unchanged.RecordVersion)
}

func TestImportReplaceClearsMissingSyncState(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	require.NoError(t, s.PutSyncState(ctx, &store.SyncState{
		LastBlockHeight: 99,
		SchemaVersion:   currentSchemaVersion,
	}))

	env := ExportEnvelope{
		Version:       1,
		SchemaVersion: currentSchemaVersion,
		Deployments:   []*store.DeploymentRecord{},
		Leases:        []*store.LeaseRecord{},
		Bids:          []*store.BidRecord{},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)
	require.NoError(t, s.Import(ctx, bytes.NewReader(data), store.FormatJSON, false))

	syncState, err := s.GetSyncState(ctx)
	require.NoError(t, err)
	require.Nil(t, syncState)
}

func TestImportRejectsUnsupportedVersionsWithoutMutation(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name          string
		version       int
		schemaVersion uint64
		wantError     string
	}{
		{
			name:          "envelope",
			version:       2,
			schemaVersion: currentSchemaVersion,
			wantError:     "unsupported export version",
		},
		{
			name:          "schema",
			version:       1,
			schemaVersion: currentSchemaVersion + 1,
			wantError:     "unsupported schema version",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
				Owner: "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu",
				DSeq:  1,
				State: "active",
			}))

			env := ExportEnvelope{
				Version:       tc.version,
				SchemaVersion: tc.schemaVersion,
				Deployments: []*store.DeploymentRecord{
					{Owner: "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", DSeq: 2, State: "closed"},
				},
			}
			data, err := json.Marshal(env)
			require.NoError(t, err)

			err = s.Import(ctx, bytes.NewReader(data), store.FormatJSON, false)
			require.ErrorContains(t, err, tc.wantError)

			existing, err := s.GetDeployment(ctx, "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu", 1)
			require.NoError(t, err)
			require.NotNil(t, existing)
		})
	}
}

func TestValidateImportEnvelopeAcceptsEveryPersistedState(t *testing.T) {
	const (
		owner    = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"
		provider = "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl"
	)

	t.Run("deployment", func(t *testing.T) {
		for _, state := range []string{"active", "closed"} {
			t.Run(state, func(t *testing.T) {
				env := &ExportEnvelope{
					Version:       exportEnvelopeVersion,
					SchemaVersion: currentSchemaVersion,
					Deployments: []*store.DeploymentRecord{{
						Owner: owner, DSeq: 1, State: state,
					}},
				}
				require.NoError(t, validateImportEnvelope(env))
			})
		}
	})

	t.Run("lease", func(t *testing.T) {
		for _, state := range []string{"active", "closed", "insufficient_funds"} {
			t.Run(state, func(t *testing.T) {
				env := &ExportEnvelope{
					Version:       exportEnvelopeVersion,
					SchemaVersion: currentSchemaVersion,
					Leases: []*store.LeaseRecord{{
						ID:    store.LeaseID{Owner: owner, DSeq: 1, GSeq: 1, OSeq: 1, Provider: provider},
						State: state,
					}},
				}
				require.NoError(t, validateImportEnvelope(env))
			})
		}
	})

	t.Run("bid", func(t *testing.T) {
		for _, state := range []string{"open", "matched", "closed", "lost"} {
			t.Run(state, func(t *testing.T) {
				env := &ExportEnvelope{
					Version:       exportEnvelopeVersion,
					SchemaVersion: currentSchemaVersion,
					Bids: []*store.BidRecord{{
						ID:    store.BidID{Owner: owner, DSeq: 1, GSeq: 1, OSeq: 1, Provider: provider},
						State: state,
					}},
				}
				require.NoError(t, validateImportEnvelope(env))
			})
		}
	})
}

func TestImportRejectsInvalidRecordsBeforeMutation(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu",
		DSeq:  1,
		State: "active",
	}))

	validLeaseID := store.LeaseID{
		Owner:    "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:     2,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
	}
	validBidID := store.BidID(validLeaseID)

	tests := []struct {
		name      string
		configure func(*ExportEnvelope)
		wantError string
	}{
		{
			name: "deployment identity",
			configure: func(env *ExportEnvelope) {
				env.Deployments = []*store.DeploymentRecord{{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"}}
			},
			wantError: "deployment 0 has invalid identity",
		},
		{
			name: "deployment malformed owner",
			configure: func(env *ExportEnvelope) {
				env.Deployments = []*store.DeploymentRecord{{Owner: "akash1not-valid", DSeq: 2, State: "active"}}
			},
			wantError: "deployment 0 has invalid identity: owner",
		},
		{
			name: "duplicate deployment",
			configure: func(env *ExportEnvelope) {
				env.Deployments = []*store.DeploymentRecord{
					{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "active"},
					{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "active"},
				}
			},
			wantError: "deployment 1 duplicates",
		},
		{
			name: "deployment state",
			configure: func(env *ExportEnvelope) {
				env.Deployments = []*store.DeploymentRecord{{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "pending"}}
			},
			wantError: "deployment 0 has invalid state",
		},
		{
			name: "deployment negative created timestamp",
			configure: func(env *ExportEnvelope) {
				env.Deployments = []*store.DeploymentRecord{{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "active", CreatedAt: -1}}
			},
			wantError: "deployment 0 has negative timestamp or height",
		},
		{
			name: "deployment negative updated timestamp",
			configure: func(env *ExportEnvelope) {
				env.Deployments = []*store.DeploymentRecord{{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "active", UpdatedAt: -1}}
			},
			wantError: "deployment 0 has negative timestamp or height",
		},
		{
			name: "deployment negative closed timestamp",
			configure: func(env *ExportEnvelope) {
				env.Deployments = []*store.DeploymentRecord{{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "closed", ClosedAt: -1}}
			},
			wantError: "deployment 0 has negative timestamp or height",
		},
		{
			name: "deployment negative creation height",
			configure: func(env *ExportEnvelope) {
				env.Deployments = []*store.DeploymentRecord{{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 2, State: "active", CreatedHeight: -1}}
			},
			wantError: "deployment 0 has negative timestamp or height",
		},
		{
			name: "null lease",
			configure: func(env *ExportEnvelope) {
				env.Leases = []*store.LeaseRecord{nil}
			},
			wantError: "lease 0 is null",
		},
		{
			name: "lease identity",
			configure: func(env *ExportEnvelope) {
				id := validLeaseID
				id.DSeq = 0
				env.Leases = []*store.LeaseRecord{{ID: id}}
			},
			wantError: "lease 0 has invalid identity",
		},
		{
			name: "lease malformed owner",
			configure: func(env *ExportEnvelope) {
				id := validLeaseID
				id.Owner = "akash1not-valid"
				env.Leases = []*store.LeaseRecord{{ID: id, State: "active"}}
			},
			wantError: "lease 0 has invalid identity: owner",
		},
		{
			name: "lease malformed provider",
			configure: func(env *ExportEnvelope) {
				id := validLeaseID
				id.Provider = "akash1not-valid"
				env.Leases = []*store.LeaseRecord{{ID: id, State: "active"}}
			},
			wantError: "lease 0 has invalid identity: provider",
		},
		{
			name: "duplicate lease",
			configure: func(env *ExportEnvelope) {
				env.Leases = []*store.LeaseRecord{{ID: validLeaseID, State: "active"}, {ID: validLeaseID, State: "active"}}
			},
			wantError: "lease 1 duplicates",
		},
		{
			name: "lease state",
			configure: func(env *ExportEnvelope) {
				env.Leases = []*store.LeaseRecord{{ID: validLeaseID, State: "open"}}
			},
			wantError: "lease 0 has invalid state",
		},
		{
			name: "lease negative created timestamp",
			configure: func(env *ExportEnvelope) {
				env.Leases = []*store.LeaseRecord{{ID: validLeaseID, State: "active", CreatedAt: -1}}
			},
			wantError: "lease 0 has negative timestamp",
		},
		{
			name: "lease negative closed timestamp",
			configure: func(env *ExportEnvelope) {
				env.Leases = []*store.LeaseRecord{{ID: validLeaseID, State: "closed", ClosedAt: -1}}
			},
			wantError: "lease 0 has negative timestamp",
		},
		{
			name: "null bid",
			configure: func(env *ExportEnvelope) {
				env.Bids = []*store.BidRecord{nil}
			},
			wantError: "bid 0 is null",
		},
		{
			name: "bid identity",
			configure: func(env *ExportEnvelope) {
				id := validBidID
				id.OSeq = 0
				env.Bids = []*store.BidRecord{{ID: id}}
			},
			wantError: "bid 0 has invalid identity",
		},
		{
			name: "bid malformed owner",
			configure: func(env *ExportEnvelope) {
				id := validBidID
				id.Owner = "akash1not-valid"
				env.Bids = []*store.BidRecord{{ID: id, State: "open"}}
			},
			wantError: "bid 0 has invalid identity: owner",
		},
		{
			name: "bid malformed provider",
			configure: func(env *ExportEnvelope) {
				id := validBidID
				id.Provider = "akash1not-valid"
				env.Bids = []*store.BidRecord{{ID: id, State: "open"}}
			},
			wantError: "bid 0 has invalid identity: provider",
		},
		{
			name: "duplicate bid",
			configure: func(env *ExportEnvelope) {
				env.Bids = []*store.BidRecord{{ID: validBidID, State: "open"}, {ID: validBidID, State: "open"}}
			},
			wantError: "bid 1 duplicates",
		},
		{
			name: "bid state",
			configure: func(env *ExportEnvelope) {
				env.Bids = []*store.BidRecord{{ID: validBidID, State: "active"}}
			},
			wantError: "bid 0 has invalid state",
		},
		{
			name: "bid negative created timestamp",
			configure: func(env *ExportEnvelope) {
				env.Bids = []*store.BidRecord{{ID: validBidID, State: "open", CreatedAt: -1}}
			},
			wantError: "bid 0 has negative timestamp",
		},
		{
			name: "negative sync state",
			configure: func(env *ExportEnvelope) {
				env.SyncState = &store.SyncState{LastBlockHeight: -1}
			},
			wantError: "sync state has negative height or timestamp",
		},
		{
			name: "future sync schema",
			configure: func(env *ExportEnvelope) {
				env.SyncState = &store.SyncState{SchemaVersion: currentSchemaVersion + 1}
			},
			wantError: "sync state schema version",
		},
		{
			name: "malformed tracked account",
			configure: func(env *ExportEnvelope) {
				env.SyncState = &store.SyncState{TrackedAccounts: []string{"akash1not-valid"}}
			},
			wantError: "sync state tracked account 0 has invalid identity",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := ExportEnvelope{
				Version:       exportEnvelopeVersion,
				SchemaVersion: currentSchemaVersion,
			}
			tc.configure(&env)
			data, err := json.Marshal(env)
			require.NoError(t, err)

			err = s.Import(ctx, bytes.NewReader(data), store.FormatJSON, false)
			require.ErrorContains(t, err, tc.wantError)

			existing, err := s.GetDeployment(ctx, "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu", 1)
			require.NoError(t, err)
			require.NotNil(t, existing)
			require.Equal(t, "active", existing.State)
		})
	}
}

func TestValidateImportAccountAddressRequiresCanonicalAkashAccounts(t *testing.T) {
	accountPayload := bytes.Repeat([]byte{1}, 20)
	valid, err := address.NewBech32Codec("akash").BytesToString(accountPayload)
	require.NoError(t, err)
	wrongPrefix, err := address.NewBech32Codec("cosmos").BytesToString(accountPayload)
	require.NoError(t, err)
	short, err := address.NewBech32Codec("akash").BytesToString([]byte{1})
	require.NoError(t, err)
	contract, err := address.NewBech32Codec("akash").BytesToString(bytes.Repeat([]byte{1}, 32))
	require.NoError(t, err)

	require.NoError(t, validateImportAccountAddress(valid))
	for _, invalid := range []string{
		"",
		"akash1not-valid",
		wrongPrefix,
		short,
		contract,
		strings.ToUpper(valid),
	} {
		require.Error(t, validateImportAccountAddress(invalid), "accepted invalid account address %q", invalid)
	}
}

func TestValidateImportExercisesWritesWithoutCommitting(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:         "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:          1,
		State:         "active",
		RecordVersion: math.MaxUint64,
	}))

	env := ExportEnvelope{
		Version:       exportEnvelopeVersion,
		SchemaVersion: currentSchemaVersion,
		Deployments: []*store.DeploymentRecord{
			{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 1, State: "closed", RecordVersion: math.MaxUint64},
		},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	err = s.ValidateImport(ctx, bytes.NewReader(data), store.FormatJSON, true)
	require.ErrorContains(t, err, "record revision cannot advance")

	existing, err := s.GetDeployment(ctx, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", 1)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.Equal(t, "active", existing.State)
	require.Equal(t, uint64(math.MaxUint64), existing.RecordVersion)
}

func TestValidateImportRejectsMalformedInputWithoutMutation(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	seedStore(t, s)

	err := s.ValidateImport(
		ctx,
		strings.NewReader(`{"version":1,"schema_version":1,"deployments":[`),
		store.FormatJSON,
		false,
	)
	require.ErrorContains(t, err, "unmarshal JSON")
	assertSeededStoreUnchanged(t, s)
}

func TestValidateImportSnapshotExercisesExistingStateWithoutMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "existing.db")
	s, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:         "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:          1,
		State:         "active",
		RecordVersion: math.MaxUint64,
	}))
	require.NoError(t, s.Close())

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	env := ExportEnvelope{
		Version:       exportEnvelopeVersion,
		SchemaVersion: currentSchemaVersion,
		Deployments: []*store.DeploymentRecord{
			{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 1, State: "closed", RecordVersion: math.MaxUint64},
		},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	err = ValidateImportSnapshot(ctx, path, bytes.NewReader(data), store.FormatJSON, true)
	require.ErrorContains(t, err, "record revision cannot advance")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestValidateImportSnapshotDoesNotCreateMissingSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing", "store.db")
	data := []byte(`{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`)

	require.NoError(t, ValidateImportSnapshot(
		context.Background(),
		path,
		bytes.NewReader(data),
		store.FormatJSON,
		true,
	))

	_, err := os.Stat(filepath.Dir(path))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCopyStoreSnapshotReportsCancellationOpenAndWriteFailures(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	err := copyStoreSnapshot(cancelled, filepath.Join(t.TempDir(), "missing.db"), &bytes.Buffer{})
	require.ErrorIs(t, err, context.Canceled)

	err = copyStoreSnapshot(context.Background(), filepath.Join(t.TempDir(), "missing.db"), &bytes.Buffer{})
	require.ErrorContains(t, err, "open store snapshot")

	path := filepath.Join(t.TempDir(), "source.db")
	s, err := Open(path)
	require.NoError(t, err)
	seedStore(t, s)
	require.NoError(t, s.Close())

	err = copyStoreSnapshot(
		context.Background(),
		path,
		&failAfterWriter{err: errors.New("disk full")},
	)
	require.ErrorContains(t, err, "copy store snapshot")
	require.ErrorContains(t, err, "disk full")

	var snapshot bytes.Buffer
	require.NoError(t, copyStoreSnapshot(context.Background(), path, &snapshot))
	require.NotEmpty(t, snapshot.Bytes())
}

func TestCopyStoreSnapshotLockWaitIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.db")
	holder, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, holder.Close()) })
	seedStore(t, holder)

	t.Run("caller deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
		defer cancel()

		started := time.Now()
		err := copyStoreSnapshot(ctx, path, &bytes.Buffer{})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(started), time.Second)
	})

	t.Run("fixed fallback", func(t *testing.T) {
		started := time.Now()
		err := copyStoreSnapshot(context.Background(), path, &bytes.Buffer{})
		require.ErrorContains(t, err, "open store snapshot")
		require.ErrorIs(t, err, bolterrors.ErrTimeout)
		require.Less(t, time.Since(started), time.Second)
	})
}

func TestValidateImportSnapshotHandlesZeroLengthAndUninspectableSources(t *testing.T) {
	validImport := []byte(`{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`)

	zeroLength := filepath.Join(t.TempDir(), "empty.db")
	require.NoError(t, os.WriteFile(zeroLength, nil, 0o600))
	require.NoError(t, ValidateImportSnapshot(
		context.Background(),
		zeroLength,
		bytes.NewReader(validImport),
		store.FormatJSON,
		true,
	))
	info, err := os.Stat(zeroLength)
	require.NoError(t, err)
	require.Zero(t, info.Size(), "validation must not initialize the selected store")

	notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(notDirectory, []byte("file"), 0o600))
	err = ValidateImportSnapshot(
		context.Background(),
		filepath.Join(notDirectory, "store.db"),
		bytes.NewReader(validImport),
		store.FormatJSON,
		true,
	)
	require.ErrorContains(t, err, "inspect store")

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	err = ValidateImportSnapshot(
		cancelled,
		zeroLength,
		bytes.NewReader(validImport),
		store.FormatJSON,
		true,
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestValidateImportSnapshotReportsTemporaryCopyAndMigrationFailures(t *testing.T) {
	validImport := []byte(`{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`)

	t.Run("temporary file", func(t *testing.T) {
		notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(notDirectory, []byte("file"), 0o600))
		t.Setenv("TMPDIR", notDirectory)

		err := ValidateImportSnapshot(
			context.Background(),
			filepath.Join(t.TempDir(), "missing.db"),
			bytes.NewReader(validImport),
			store.FormatJSON,
			true,
		)
		require.ErrorContains(t, err, "create disposable store")
	})

	t.Run("copy corrupt source", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "corrupt.db")
		require.NoError(t, os.WriteFile(path, []byte("not a bbolt database"), 0o600))

		err := ValidateImportSnapshot(
			context.Background(),
			path,
			bytes.NewReader(validImport),
			store.FormatJSON,
			true,
		)
		require.ErrorContains(t, err, "open store snapshot")
	})

	t.Run("migration", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "source.db")
		s, err := Open(path)
		require.NoError(t, err)
		require.NoError(t, s.Close())

		migrationErr := errors.New("migration failed")
		withMigrations([]Migration{{
			Version:     currentSchemaVersion + 1,
			Description: "test failure",
			Fn: func(*bolt.Tx) error {
				return migrationErr
			},
		}}, func() {
			err = ValidateImportSnapshot(
				context.Background(),
				path,
				bytes.NewReader(validImport),
				store.FormatJSON,
				true,
			)
			require.ErrorContains(t, err, "migrate disposable store")
			require.ErrorIs(t, err, migrationErr)
		})
	})
}

func TestValidateImportSnapshotPropagatesLifecycleFailures(t *testing.T) {
	validImport := []byte(`{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`)
	sourcePath := filepath.Join(t.TempDir(), "missing.db")

	t.Run("temporary file close", func(t *testing.T) {
		closeErr := errors.New("temporary close failed")
		file := &importSnapshotFileStub{
			name:     filepath.Join(t.TempDir(), "temporary.db"),
			closeErr: closeErr,
		}
		err := validateImportSnapshot(
			context.Background(),
			sourcePath,
			bytes.NewReader(validImport),
			store.FormatJSON,
			true,
			func() (importSnapshotFile, error) { return file, nil },
			func(string) (importSnapshotStore, error) {
				t.Fatal("store must not be opened after the temporary file fails to close")
				return nil, nil
			},
		)
		require.ErrorContains(t, err, "close disposable store")
		require.ErrorIs(t, err, closeErr)
	})

	t.Run("store open", func(t *testing.T) {
		openErr := errors.New("snapshot open failed")
		file := &importSnapshotFileStub{name: filepath.Join(t.TempDir(), "temporary.db")}
		err := validateImportSnapshot(
			context.Background(),
			sourcePath,
			bytes.NewReader(validImport),
			store.FormatJSON,
			true,
			func() (importSnapshotFile, error) { return file, nil },
			func(string) (importSnapshotStore, error) { return nil, openErr },
		)
		require.ErrorContains(t, err, "open disposable store")
		require.ErrorIs(t, err, openErr)
	})

	t.Run("store close", func(t *testing.T) {
		closeErr := errors.New("snapshot close failed")
		file := &importSnapshotFileStub{name: filepath.Join(t.TempDir(), "temporary.db")}
		snapshot := &importSnapshotStoreStub{closeErr: closeErr}
		err := validateImportSnapshot(
			context.Background(),
			sourcePath,
			bytes.NewReader(validImport),
			store.FormatJSON,
			true,
			func() (importSnapshotFile, error) { return file, nil },
			func(string) (importSnapshotStore, error) { return snapshot, nil },
		)
		require.ErrorContains(t, err, "close disposable store")
		require.ErrorIs(t, err, closeErr)
		require.True(t, snapshot.migrated)
		require.True(t, snapshot.validated)
		require.True(t, snapshot.closed)
	})
}

func TestCopyStoreSnapshotCancellationInsideReadTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.db")
	s, err := Open(path)
	require.NoError(t, err)
	seedStore(t, s)
	require.NoError(t, s.Close())

	ctx := &cancelAfterContextChecks{
		Context:   context.Background(),
		remaining: 1,
	}
	var destination bytes.Buffer
	err = copyStoreSnapshot(ctx, path, &destination)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, destination.Bytes())
}

func TestCopyStoreSnapshotRejectsElapsedDeadlineBeforeOpening(t *testing.T) {
	ctx := elapsedDeadlineContext{Context: context.Background()}
	err := copyStoreSnapshot(ctx, filepath.Join(t.TempDir(), "missing.db"), &bytes.Buffer{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestCopyStoreSnapshotReturnsCancellationThatWinsOpenFailure(t *testing.T) {
	ctx := &cancelAfterContextChecks{Context: context.Background(), remaining: 1}
	err := copyStoreSnapshot(ctx, filepath.Join(t.TempDir(), "missing.db"), &bytes.Buffer{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestCopyStoreSnapshotPropagatesCloseFailureAfterCompleteCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.db")
	s, err := Open(path)
	require.NoError(t, err)
	seedStore(t, s)
	require.NoError(t, s.Close())

	closeErr := errors.New("source close failed")
	var destination bytes.Buffer
	err = copyStoreSnapshotWithOpen(
		context.Background(),
		path,
		&destination,
		func(path string, timeout time.Duration) (readOnlySnapshot, error) {
			db, openErr := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: timeout})
			if openErr != nil {
				return nil, openErr
			}
			return &closeErrorReadOnlySnapshot{DB: db, closeErr: closeErr}, nil
		},
	)
	require.ErrorContains(t, err, "close store snapshot")
	require.ErrorIs(t, err, closeErr)
	require.NotEmpty(t, destination.Bytes(), "the close failure must occur after the snapshot was copied")
}

func TestValidateImportAccountAddressPropagatesReencodingFailure(t *testing.T) {
	reencodeErr := errors.New("re-encode failed")
	err := validateImportAccountAddressWithCodec("akash1test", reencodeErrorCodec{err: reencodeErr})
	require.ErrorContains(t, err, "re-encode akash address")
	require.ErrorIs(t, err, reencodeErr)
}

func TestClearDataBucketsPropagatesRecreationFailureAndRollsBack(t *testing.T) {
	s := openTestStore(t)
	seedStore(t, s)
	recreateErr := errors.New("recreate failed")

	err := s.db.Update(func(tx *bolt.Tx) error {
		return clearDataBucketsTxWithCreate(tx, func(*bolt.Tx, []byte) (*bolt.Bucket, error) {
			return nil, recreateErr
		})
	})
	require.ErrorContains(t, err, "recreate bucket deployments")
	require.ErrorIs(t, err, recreateErr)

	deployments, err := s.ListDeployments(context.Background(), store.DeploymentFilter{})
	require.NoError(t, err)
	require.Len(t, deployments, 2, "the failed clear transaction must retain existing records")
}

func TestImportRollsBackWhenLeaseOrBidRevisionCannotAdvance(t *testing.T) {
	ctx := context.Background()
	id := store.LeaseID{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 7, GSeq: 1, OSeq: 1, Provider: "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
	}

	for _, tc := range []struct {
		name      string
		configure func(*testing.T, *BoltStore, *ExportEnvelope)
		verify    func(*testing.T, *BoltStore)
	}{
		{
			name: "lease",
			configure: func(t *testing.T, s *BoltStore, env *ExportEnvelope) {
				t.Helper()
				require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
					ID: id, State: "active", RecordVersion: math.MaxUint64,
				}))
				env.Leases = []*store.LeaseRecord{
					{ID: id, State: "closed", RecordVersion: math.MaxUint64},
				}
			},
			verify: func(t *testing.T, s *BoltStore) {
				t.Helper()
				lease, err := s.GetLease(ctx, id)
				require.NoError(t, err)
				require.Equal(t, "active", lease.State)
				require.Equal(t, uint64(math.MaxUint64), lease.RecordVersion)
			},
		},
		{
			name: "bid",
			configure: func(t *testing.T, s *BoltStore, env *ExportEnvelope) {
				t.Helper()
				bidID := store.BidID(id)
				require.NoError(t, s.PutBid(ctx, &store.BidRecord{
					ID: bidID, State: "open", RecordVersion: math.MaxUint64,
				}))
				env.Bids = []*store.BidRecord{
					{ID: bidID, State: "closed", RecordVersion: math.MaxUint64},
				}
			},
			verify: func(t *testing.T, s *BoltStore) {
				t.Helper()
				bid, err := s.GetBid(ctx, store.BidID(id))
				require.NoError(t, err)
				require.Equal(t, "open", bid.State)
				require.Equal(t, uint64(math.MaxUint64), bid.RecordVersion)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			require.NoError(t, s.PutSyncState(ctx, &store.SyncState{LastBlockHeight: 99}))
			env := ExportEnvelope{
				Version:       exportEnvelopeVersion,
				SchemaVersion: currentSchemaVersion,
				Deployments: []*store.DeploymentRecord{
					{Owner: "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh", DSeq: 1, State: "active"},
				},
			}
			tc.configure(t, s, &env)
			data, err := json.Marshal(env)
			require.NoError(t, err)

			err = s.Import(ctx, bytes.NewReader(data), store.FormatJSON, true)
			require.ErrorContains(t, err, "record revision cannot advance")

			inserted, err := s.GetDeployment(ctx, "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh", 1)
			require.NoError(t, err)
			require.Nil(t, inserted, "an earlier record from the failed transaction was committed")
			tc.verify(t, s)
			syncState, err := s.GetSyncState(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(99), syncState.LastBlockHeight)
		})
	}
}

func TestReplaceImportCancellationRollsBackClearsAndWrites(t *testing.T) {
	s := openTestStore(t)
	seedStore(t, s)

	env := ExportEnvelope{
		Version:       exportEnvelopeVersion,
		SchemaVersion: currentSchemaVersion,
		Deployments: []*store.DeploymentRecord{
			{Owner: "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", DSeq: 10, State: "active"},
			{Owner: "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", DSeq: 11, State: "closed"},
		},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)
	ctx := &cancelAfterContextChecks{
		Context:   context.Background(),
		remaining: 2,
	}

	err = s.Import(ctx, bytes.NewReader(data), store.FormatJSON, false)
	require.ErrorIs(t, err, context.Canceled)
	assertSeededStoreUnchanged(t, s)
}

func TestMergeImportCancellationAtEveryRecordStageRollsBack(t *testing.T) {
	leaseID := store.LeaseID{
		Owner: "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", DSeq: 10, GSeq: 1, OSeq: 1, Provider: "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
	}
	env := ExportEnvelope{
		Version:       exportEnvelopeVersion,
		SchemaVersion: currentSchemaVersion,
		Deployments: []*store.DeploymentRecord{
			{Owner: "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", DSeq: 10, State: "active"},
		},
		Leases: []*store.LeaseRecord{
			{ID: leaseID, State: "active"},
		},
		Bids: []*store.BidRecord{
			{ID: store.BidID(leaseID), State: "open"},
		},
		SyncState: &store.SyncState{LastBlockHeight: 10},
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	for _, tc := range []struct {
		name      string
		remaining int
	}{
		{name: "initial", remaining: 0},
		{name: "lease", remaining: 2},
		{name: "bid", remaining: 3},
		{name: "sync state", remaining: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			seedStore(t, s)
			ctx := &cancelAfterContextChecks{
				Context:   context.Background(),
				remaining: tc.remaining,
			}

			err := s.Import(ctx, bytes.NewReader(data), store.FormatJSON, true)
			require.ErrorIs(t, err, context.Canceled)
			assertSeededStoreUnchanged(t, s)

			inserted, err := s.GetDeployment(context.Background(), "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", 10)
			require.NoError(t, err)
			require.Nil(t, inserted)
		})
	}
}

func TestApplyImportTxPropagatesSyncStorageFailure(t *testing.T) {
	s := openTestStore(t)
	env := &ExportEnvelope{
		Version:       exportEnvelopeVersion,
		SchemaVersion: currentSchemaVersion,
		SyncState:     &store.SyncState{LastBlockHeight: 10},
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		return applyImportTx(context.Background(), tx, env, true)
	})
	require.ErrorContains(t, err, "put sync state")
	require.ErrorIs(t, err, bolterrors.ErrTxNotWritable)

	syncState, getErr := s.GetSyncState(context.Background())
	require.NoError(t, getErr)
	require.Nil(t, syncState)
}

func TestReplaceImportWithMissingBucketRollsBackEarlierClears(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	seedStore(t, s)
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(bucketBids)
	}))

	data := []byte(`{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`)
	err := s.Import(ctx, bytes.NewReader(data), store.FormatJSON, false)
	require.ErrorContains(t, err, "clear data buckets: delete bucket bids")

	deployments, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	require.Len(t, deployments, 2)
	leases, err := s.ListLeases(ctx, store.LeaseFilter{})
	require.NoError(t, err)
	require.Len(t, leases, 1)
}

type cancelAfterContextChecks struct {
	context.Context
	remaining int
}

type elapsedDeadlineContext struct {
	context.Context
}

func (elapsedDeadlineContext) Deadline() (time.Time, bool) {
	return time.Now().Add(-time.Second), true
}

func (elapsedDeadlineContext) Err() error {
	return nil
}

type importSnapshotFileStub struct {
	bytes.Buffer
	name     string
	closeErr error
}

func (f *importSnapshotFileStub) Name() string {
	return f.name
}

func (f *importSnapshotFileStub) Close() error {
	return f.closeErr
}

type importSnapshotStoreStub struct {
	migrated  bool
	validated bool
	closed    bool
	closeErr  error
}

func (s *importSnapshotStoreStub) Migrate(context.Context) error {
	s.migrated = true
	return nil
}

func (s *importSnapshotStoreStub) ValidateImport(context.Context, io.Reader, store.ExportFormat, bool) error {
	s.validated = true
	return nil
}

func (s *importSnapshotStoreStub) Close() error {
	s.closed = true
	return s.closeErr
}

type closeErrorReadOnlySnapshot struct {
	*bolt.DB
	closeErr error
}

func (s *closeErrorReadOnlySnapshot) Close() error {
	if err := s.DB.Close(); err != nil {
		return err
	}
	return s.closeErr
}

type reencodeErrorCodec struct {
	err error
}

func (reencodeErrorCodec) StringToBytes(string) ([]byte, error) {
	return make([]byte, 20), nil
}

func (c reencodeErrorCodec) BytesToString([]byte) (string, error) {
	return "", c.err
}

func (c *cancelAfterContextChecks) Err() error {
	if c.remaining == 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func FuzzImportFailureIsAtomic(f *testing.F) {
	f.Add([]byte(`{"version":1,"schema_version":1,"deployments":[null]}`), uint8(store.FormatJSON), false)
	f.Add([]byte(`version: 1
schema_version: 1
deployments: []
leases: []
bids: []
`), uint8(store.FormatYAML), true)
	f.Add([]byte(`{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`), uint8(store.FormatJSON), false)

	f.Fuzz(func(t *testing.T, data []byte, rawFormat uint8, merge bool) {
		if len(data) > 64*1024 {
			t.Skip()
		}

		format := store.ExportFormat(rawFormat % 2)
		ctx := context.Background()
		s := openTestStore(t)
		require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
			Owner: "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu",
			DSeq:  1,
			State: "active",
		}))

		err := s.Import(ctx, bytes.NewReader(data), format, merge)
		if err == nil {
			return
		}

		existing, getErr := s.GetDeployment(ctx, "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu", 1)
		require.NoError(t, getErr)
		require.NotNil(t, existing, "failed import removed pre-existing data: %v", err)
		require.Equal(t, "active", existing.State)
		require.Equal(t, uint64(1), existing.RecordVersion)
	})
}

// decodeYAMLEnvelope unmarshals a YAML export (which may have a --- prefix) into an ExportEnvelope.
func decodeYAMLEnvelope(data []byte, env *ExportEnvelope) error {
	// yaml.Unmarshal handles the --- document start marker automatically.
	return yaml.Unmarshal(data, env)
}
