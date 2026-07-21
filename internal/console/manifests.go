package console

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manifest cache: `deployment create` stores the manifest returned by the
// Console API so `lease create` can default to it later. Pure file helpers,
// independent of the Client.

// ManifestPath returns the cache path for a deployment's manifest:
// <root>/contexts/<ctxName>/manifests/<dseq>.json.
func ManifestPath(root, ctxName, dseq string) string {
	return filepath.Join(root, "contexts", ctxName, "manifests", dseq+".json")
}

// SaveManifest caches a deployment manifest under the context's manifests
// directory with mode 0600 (manifests can reference private registries or
// env values).
func SaveManifest(root, ctxName, dseq, manifest string) error {
	path := ManifestPath(root, ctxName, dseq)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("console: create manifests dir: %w", err)
	}

	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		return fmt.Errorf("console: write manifest: %w", err)
	}

	return nil
}

// LoadManifest reads a previously cached manifest. The returned error wraps
// fs.ErrNotExist when no manifest has been cached for the dseq.
func LoadManifest(root, ctxName, dseq string) (string, error) {
	data, err := os.ReadFile(ManifestPath(root, ctxName, dseq))
	if err != nil {
		return "", fmt.Errorf("console: load manifest for dseq %s: %w", dseq, err)
	}

	return string(data), nil
}
