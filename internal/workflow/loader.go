package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Loader resolves and loads workflow definitions from the filesystem
// and embedded defaults.
//
// Resolution order:
//  1. Per-context: <home>/contexts/<ctx>/workflows/<name>.yaml
//  2. Global user: <home>/workflows/<name>.yaml
//  3. Embedded built-in defaults
type Loader struct {
	home        string
	contextName string
	builtins    map[string][]byte // name -> yaml bytes
}

// NewLoader creates a workflow loader for the given home directory and context.
func NewLoader(home, contextName string, builtins map[string][]byte) *Loader {
	return &Loader{
		home:        home,
		contextName: contextName,
		builtins:    builtins,
	}
}

// Load loads a workflow definition by name, following the resolution order.
func (l *Loader) Load(name string) (*WorkflowDef, error) {
	// 1. Per-context override.
	if l.contextName != "" {
		path := filepath.Join(l.home, "contexts", l.contextName, "workflows", name+".yaml")
		if wf, err := loadFromFile(path); err == nil {
			return wf, nil
		}
	}

	// 2. Global user-defined.
	path := filepath.Join(l.home, "workflows", name+".yaml")
	if wf, err := loadFromFile(path); err == nil {
		return wf, nil
	}

	// 3. Embedded built-in.
	if data, ok := l.builtins[name]; ok {
		return parseWorkflow(data)
	}

	return nil, fmt.Errorf("workflow %q not found", name)
}

// List returns the names of all available workflows (from all sources, deduplicated).
func (l *Loader) List() []string {
	seen := make(map[string]bool)
	var names []string

	// Per-context workflows.
	if l.contextName != "" {
		dir := filepath.Join(l.home, "contexts", l.contextName, "workflows")
		addFromDir(dir, seen, &names)
	}

	// Global workflows.
	dir := filepath.Join(l.home, "workflows")
	addFromDir(dir, seen, &names)

	// Built-in workflows.
	for name := range l.builtins {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	return names
}

func addFromDir(dir string, seen map[string]bool, names *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		base := name[:len(name)-len(ext)]
		if !seen[base] {
			seen[base] = true
			*names = append(*names, base)
		}
	}
}

func loadFromFile(path string) (*WorkflowDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return parseWorkflow(data)
}

func parseWorkflow(data []byte) (*WorkflowDef, error) {
	var wf WorkflowDef
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse workflow: %w", err)
	}

	if wf.Name == "" {
		return nil, fmt.Errorf("workflow missing required field: name")
	}

	if len(wf.Steps) == 0 {
		return nil, fmt.Errorf("workflow %q has no steps", wf.Name)
	}

	return &wf, nil
}
