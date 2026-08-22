package console

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Manifest cache: `deployment create` stores the manifest returned by the
// Console API so `lease create` can default to it later. Pure file helpers,
// independent of the Client.

// validateDSeq rejects any dseq that is not a plain unsigned decimal number.
// The dseq becomes a filesystem path component, and it can arrive from
// untrusted places (CLI arguments, a Console API response), so anything else
// — "../..", absolute paths, empty strings — must never reach
// filepath.Join.
func validateDSeq(dseq string) error {
	parsed, err := strconv.ParseUint(dseq, 10, 64)
	if err != nil || parsed == 0 {
		return fmt.Errorf("console: invalid dseq %q: must be a positive numeric sequence", dseq)
	}

	return nil
}

// ManifestPath returns the cache path for a deployment's manifest:
// <root>/contexts/<ctxName>/manifests/<dseq>.json. The dseq must be a plain
// numeric sequence; anything else (e.g. a path-traversal attempt) is an
// error.
func ManifestPath(root, ctxName, dseq string) (string, error) {
	if err := validateDSeq(dseq); err != nil {
		return "", err
	}

	return filepath.Join(root, "contexts", ctxName, "manifests", dseq+".json"), nil
}

// SaveManifest caches a deployment manifest under the context's manifests
// directory with mode 0600 (manifests can reference private registries or
// env values). The dseq is validated as a plain numeric sequence first: it
// comes from the Console API response, and a hostile value must not steer
// the write outside the config root.
func SaveManifest(root, ctxName, dseq, manifest string) error {
	path, err := ManifestPath(root, ctxName, dseq)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("console: create manifests dir: %w", err)
	}

	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		return fmt.Errorf("console: write manifest: %w", err)
	}

	return nil
}

// LoadManifest reads a previously cached manifest. The dseq is validated as
// a plain numeric sequence so user-supplied values cannot read arbitrary
// files. The returned error wraps fs.ErrNotExist when no manifest has been
// cached for the dseq.
func LoadManifest(root, ctxName, dseq string) (string, error) {
	path, err := ManifestPath(root, ctxName, dseq)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("console: load manifest for dseq %s: %w", dseq, err)
	}

	return string(data), nil
}
