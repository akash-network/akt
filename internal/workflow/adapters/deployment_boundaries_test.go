package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/store"
	storedb "pkg.akt.dev/akt/internal/store/bbolt"
)

func TestChainDefinitionInputFailures(t *testing.T) {
	ctx := context.Background()
	inputs := NewDeploymentInputs(t.TempDir(), "chain", nil)
	base := writeTestSDL(t, validConsoleSDL)
	for _, tc := range []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"invalid source", map[string]string{"source-dseq": "bad"}, "positive integer"},
		{"missing source", map[string]string{"source-dseq": "42"}, "--sdl-file"},
		{"bad patch", map[string]string{"sdl": "[", "patch": "true"}, "invalid deployment patch"},
		{"name patch", map[string]string{"sdl": "name: renamed", "patch": "true"}, "Console context"},
		{"empty patch", map[string]string{"sdl": "{}", "patch": "true"}, "no changes"},
		{"missing base", map[string]string{"sdl": "services: {web: {image: changed}}", "patch": "true", "dseq": "42"}, "--base-sdl"},
		{"unreadable base", map[string]string{"sdl": "services: {web: {image: changed}}", "patch": "true", "dseq": "42", "base-sdl": "missing.yaml"}, "does not exist"},
		{"unknown service", map[string]string{"sdl": "services: {unknown: {image: changed}}", "patch": "true", "dseq": "42", "base-sdl": base}, "not declared"},
		{"invalid YAML", map[string]string{"sdl": "["}, "valid YAML"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := inputs.prepareChain(ctx, "", tc.params)
			require.ErrorContains(t, err, tc.want)
		})
	}
	_, err := inputs.sourcePath(ctx, "", "42", "", "base-sdl")
	require.Error(t, err)
	var absent *DeploymentInputs
	_, err = absent.sourcePath(ctx, "", "42", "", "base-sdl")
	require.ErrorContains(t, err, "--base-sdl")
	require.Error(t, inputs.ValidateChain(ctx, "", map[string]string{"sdl": "services: {}"}))
	require.ErrorContains(t, inputs.ValidateChain(ctx, "", map[string]string{"sdl": validConsoleSDL, "deposit": "invalid"}), "parse resolved")
	require.NoError(t, inputs.ValidateChain(ctx, "", map[string]string{"sdl": validConsoleSDL}))
	tx := &fakeTxClient{}
	client := newFakeChainClient(tx, testOwner())
	_, err = client.BroadcastTx(ctx, msgUpdateDeployment, map[string]string{"sdl": "[", "dseq": "42"})
	require.Error(t, err)
	require.Empty(t, tx.msgs)
	_, _, err = buildUpdateDeploymentMsg(testOwner(), map[string]string{"sdl": validConsoleSDL, "dseq": "42"}, validConsoleSDL)
	require.NoError(t, err)
}

func TestSourceStoreFailuresAndOwnerAmbiguity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	inputs := NewDeploymentInputs(root, "chain", nil)
	dbpath := aktctx.StoreDBPath(root, "chain")
	require.NoError(t, os.MkdirAll(filepath.Dir(dbpath), 0700))
	require.NoError(t, os.WriteFile(dbpath, []byte("broken database"), 0600))
	_, err := inputs.sourcePath(ctx, "owner", "42", "", "sdl-file")
	require.Error(t, err)
	require.NoError(t, os.Remove(dbpath))
	db, err := storedb.OpenContext(ctx, root, "chain")
	require.NoError(t, err)
	for _, owner := range []string{"one", "two"} {
		require.NoError(t, db.PutDeployment(ctx, &store.DeploymentRecord{Owner: owner, DSeq: 42, SDLPath: "source.yaml"}))
	}
	require.NoError(t, db.Close())
	_, err = inputs.sourcePath(ctx, "", "42", "", "sdl-file")
	require.ErrorContains(t, err, "multiple local owners")
	// Corrupt a serialized record without corrupting the database container.
	raw, err := bolt.Open(dbpath, 0600, nil)
	require.NoError(t, err)
	require.NoError(t, raw.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("deployments")).Put([]byte(store.DeploymentKey("one", 42)), []byte("{"))
	}))
	require.NoError(t, raw.Close())
	_, err = inputs.sourcePath(ctx, "one", "42", "", "sdl-file")
	require.Error(t, err)
	// A non-directory context path fails the stat boundary before opening a store.
	blockedRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(blockedRoot, "contexts"), []byte("file"), 0600))
	_, err = NewDeploymentInputs(blockedRoot, "chain", nil).sourcePath(ctx, "one", "42", "", "sdl-file")
	require.ErrorContains(t, err, "read deployment store")
}

func TestConsoleWorkflowInputFailuresDoNotSubmit(t *testing.T) {
	for _, tc := range []struct {
		name, msg string
		params    map[string]string
		want      string
	}{
		{"missing source", msgCreateDeployment, map[string]string{"source-dseq": "41"}, "read source"},
		{"create secrets", msgCreateDeployment, map[string]string{"sdl": validConsoleSDL, "secrets-file": "missing.yaml"}, "regular file"},
		{"update secrets", msgUpdateDeployment, map[string]string{"sdl": validConsoleSDL, "dseq": "42", "secrets-file": "missing.yaml"}, "regular file"},
		{"bad patch", msgUpdateDeployment, map[string]string{"sdl": "[", "dseq": "42", "patch": "true"}, "invalid deployment patch"},
		{"refused patch", msgUpdateDeployment, map[string]string{"sdl": "name: renamed", "dseq": "42", "patch": "true"}, "closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"deployment": map[string]any{"id": map[string]string{"dseq": "42"}, "state": "closed"}}})
			})
			_, err := c.BroadcastTx(context.Background(), tc.msg, tc.params)
			require.ErrorContains(t, err, tc.want)
			require.Zero(t, writes)
		})
	}
}

func TestPreparedChainReadFailures(t *testing.T) {
	inputs := NewDeploymentInputs("", "", nil)
	require.ErrorContains(t, inputs.ValidateChain(context.Background(), "", nil), "required param")
	_, _, err := inputs.prepareChain(context.Background(), "", map[string]string{"sdl": "missing.yaml"})
	require.ErrorContains(t, err, "does not exist")
}

func TestConsoleFullUpdateFailureAndMalformedChainSDL(t *testing.T) {
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := c.BroadcastTx(context.Background(), msgUpdateDeployment, map[string]string{"sdl": validConsoleSDL, "dseq": "42"})
	require.ErrorContains(t, err, "API key")
	_, _, err = buildUpdateDeploymentMsg(testOwner(), map[string]string{"sdl": "services: {}", "dseq": "42"})
	require.ErrorContains(t, err, "read deployment SDL")
}
