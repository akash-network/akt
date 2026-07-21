package console_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

func TestManifestCacheRoundTrip(t *testing.T) {
	root := t.TempDir()
	manifest := `[{"name":"web","services":[{"image":"nginx"}]}]`

	require.NoError(t, console.SaveManifest(root, "prod", "12345", manifest))

	// Stored at <root>/contexts/<ctx>/manifests/<dseq>.json with mode 0600.
	path := filepath.Join(root, "contexts", "prod", "manifests", "12345.json")
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
	}

	got, err := console.LoadManifest(root, "prod", "12345")
	require.NoError(t, err)
	assert.Equal(t, manifest, got)
}

func TestManifestCacheOverwrite(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, console.SaveManifest(root, "prod", "1", "old"))
	require.NoError(t, console.SaveManifest(root, "prod", "1", "new"))

	got, err := console.LoadManifest(root, "prod", "1")
	require.NoError(t, err)
	assert.Equal(t, "new", got)
}

func TestManifestCacheIsolatedPerContext(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, console.SaveManifest(root, "prod", "1", "prod-manifest"))
	require.NoError(t, console.SaveManifest(root, "dev", "1", "dev-manifest"))

	got, err := console.LoadManifest(root, "dev", "1")
	require.NoError(t, err)
	assert.Equal(t, "dev-manifest", got)
}

func TestLoadManifestMissing(t *testing.T) {
	_, err := console.LoadManifest(t.TempDir(), "prod", "404404")
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "missing manifest must wrap fs.ErrNotExist")
}
