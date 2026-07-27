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

func TestManifestDSeqTraversalRejected(t *testing.T) {
	// The dseq lands in a filesystem path and can come from untrusted input
	// (CLI args on load, the Console API response on save), so anything but
	// a plain numeric sequence must be rejected before touching the disk.
	hostile := []string{
		"../../../../etc/passwd",
		"../../../../x",
		"..",
		"/etc/passwd",
		"12345/../../escape",
		"12345x",
		"-1",
		"",
	}

	root := t.TempDir()

	for _, dseq := range hostile {
		t.Run(dseq, func(t *testing.T) {
			// Write side: a hostile Console API response must not steer the
			// write outside the config root.
			err := console.SaveManifest(root, "prod", dseq, "owned")
			require.Error(t, err, "SaveManifest must reject dseq %q", dseq)
			assert.Contains(t, err.Error(), "invalid dseq")

			// Read side: a hostile CLI argument must not read arbitrary files.
			_, err = console.LoadManifest(root, "prod", dseq)
			require.Error(t, err, "LoadManifest must reject dseq %q", dseq)
			assert.Contains(t, err.Error(), "invalid dseq")

			_, err = console.ManifestPath(root, "prod", dseq)
			require.Error(t, err, "ManifestPath must reject dseq %q", dseq)
		})
	}

	// Nothing may have been written outside the manifests directory: the
	// root must contain no entries at all (SaveManifest failed before
	// MkdirAll every time).
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "hostile dseqs must not create any files or directories")
}

func TestManifestPathValidDSeq(t *testing.T) {
	root := t.TempDir()

	path, err := console.ManifestPath(root, "prod", "12345")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "contexts", "prod", "manifests", "12345.json"), path)
}
