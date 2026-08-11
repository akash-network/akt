package bbolt

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"pkg.akt.dev/akt/internal/store"
)

// seedStore populates a store with test data: 2 deployments, 1 lease, 1 bid, 1 sync state.
func seedStore(t *testing.T, s *BoltStore) {
	t.Helper()
	ctx := context.Background()

	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:   "akash1abc",
		DSeq:    1,
		State:   "active",
		SDLHash: "sha256:aaa",
		Deposit: "5000000uakt",
		Labels:  map[string]string{"env": "prod"},
		Tags:    []string{"web"},
	}))
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:   "akash1abc",
		DSeq:    2,
		State:   "closed",
		SDLHash: "sha256:bbb",
		Deposit: "1000000uakt",
	}))
	require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
		ID: store.LeaseID{
			Owner:    "akash1abc",
			DSeq:     1,
			GSeq:     1,
			OSeq:     1,
			Provider: "akash1prov",
		},
		State:       "active",
		Price:       "100uakt",
		ProviderURI: "https://provider.example.com",
	}))
	require.NoError(t, s.PutBid(ctx, &store.BidRecord{
		ID: store.BidID{
			Owner:    "akash1abc",
			DSeq:     1,
			GSeq:     1,
			OSeq:     1,
			Provider: "akash1prov",
		},
		State: "open",
		Price: "50uakt",
	}))
	require.NoError(t, s.PutSyncState(ctx, &store.SyncState{
		LastBlockHeight: 18234567,
		LastSyncTime:    1742724932,
		TrackedAccounts: []string{"akash1abc"},
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
	assert.Contains(t, out, "akash1abc")
	assert.Contains(t, out, "dseq: 1")
	assert.Contains(t, out, "dseq: 2")
	assert.Contains(t, out, "akash1prov")
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

func TestImportMerge(t *testing.T) {
	ctx := context.Background()

	// Store A: has deployment dseq=1.
	storeA := openTestStore(t)
	require.NoError(t, storeA.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1abc",
		DSeq:  1,
		State: "active",
	}))

	// Export store A.
	var buf bytes.Buffer
	require.NoError(t, storeA.Export(ctx, &buf, store.FormatJSON, "testctx"))

	// Store B: has deployment dseq=2.
	storeB := openTestStore(t)
	require.NoError(t, storeB.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1abc",
		DSeq:  2,
		State: "active",
	}))

	// Import A's export into B with merge=true.
	require.NoError(t, storeB.Import(ctx, &buf, store.FormatJSON, true))

	// B should have both dseq=1 and dseq=2.
	deps, err := storeB.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	assert.Len(t, deps, 2)

	d1, err := storeB.GetDeployment(ctx, "akash1abc", 1)
	require.NoError(t, err)
	assert.NotNil(t, d1)

	d2, err := storeB.GetDeployment(ctx, "akash1abc", 2)
	require.NoError(t, err)
	assert.NotNil(t, d2)
}

func TestImportReplace(t *testing.T) {
	ctx := context.Background()

	// Store has deployment dseq=1.
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1abc",
		DSeq:  1,
		State: "active",
	}))

	// Build an import payload with only dseq=2.
	env := ExportEnvelope{
		Version:       1,
		SchemaVersion: 1,
		Deployments: []*store.DeploymentRecord{
			{Owner: "akash1abc", DSeq: 2, State: "closed"},
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

	d1, err := s.GetDeployment(ctx, "akash1abc", 1)
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

// decodeYAMLEnvelope unmarshals a YAML export (which may have a --- prefix) into an ExportEnvelope.
func decodeYAMLEnvelope(data []byte, env *ExportEnvelope) error {
	// yaml.Unmarshal handles the --- document start marker automatically.
	return yaml.Unmarshal(data, env)
}
