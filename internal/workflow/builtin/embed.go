// Package builtin provides embedded default workflow definitions.
//
// Workflow YAML files placed in this directory are embedded at build time
// and serve as defaults that users can override globally or per-context.
// To add a built-in workflow, create a <name>.yaml file in this directory.
package builtin

import "embed"

//go:embed *.yaml
var fs embed.FS

// Workflows returns a map of workflow name to YAML bytes for all
// embedded workflow definitions.
func Workflows() map[string][]byte {
	result := make(map[string][]byte)

	entries, err := fs.ReadDir(".")
	if err != nil {
		return result
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		// Strip .yaml extension.
		base := name[:len(name)-5]

		data, err := fs.ReadFile(name)
		if err != nil {
			continue
		}

		result[base] = data
	}

	return result
}
