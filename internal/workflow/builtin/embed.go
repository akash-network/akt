// Package builtin provides embedded default workflow definitions.
//
// Workflow YAML files placed in this directory are embedded at build time
// and serve as defaults that users can override globally or per-context.
// To add a built-in workflow, create a <name>.yaml file in this directory.
package builtin

// Workflows returns a map of workflow name to YAML bytes for all
// embedded workflow definitions. Returns an empty map when no built-in
// workflows are defined.
func Workflows() map[string][]byte {
	return make(map[string][]byte)
}
