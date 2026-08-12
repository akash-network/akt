package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/capability"
	"pkg.akt.dev/akt/internal/cli"
	"pkg.akt.dev/akt/internal/workflow/builtin"
)

const commandInventorySchemaVersion = 1

// commandInventoryReport is structural inventory, not a claim that a command
// has a semantic E2E scenario. Scenario metadata can attach to these stable
// paths later without maintaining a second command list.
type commandInventoryReport struct {
	SchemaVersion int                     `json:"schema_version"`
	Commands      []commandInventoryEntry `json:"commands"`
	Excluded      []excludedCommandEntry  `json:"excluded"`
}

type commandInventoryEntry struct {
	Path           string   `json:"path"`
	Args           []string `json:"args"`
	Kind           string   `json:"kind"`
	Requirement    string   `json:"requirement,omitempty"`
	Aliases        []string `json:"aliases,omitempty"`
	Runnable       bool     `json:"runnable"`
	HasSubcommands bool     `json:"has_subcommands"`
}

type excludedCommandEntry struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// TestAllCommandsHelp discovers the real assembled tree, then exercises help
// through the compiled binary for every visible path. It deliberately proves
// structural reachability only; semantic command scenarios belong in the
// lane-specific E2E suites.
func TestAllCommandsHelp(t *testing.T) {
	sandbox := t.TempDir()
	aktHome := t.TempDir()
	processHome := t.TempDir()

	// NewRootCmd discovers workflow definitions while it assembles the tree.
	// An empty working directory and isolated homes ensure discovery can see
	// embedded workflows but cannot inspect or mutate the developer's config.
	t.Chdir(sandbox)
	t.Setenv("AKT_HOME", aktHome)
	t.Setenv("HOME", processHome)
	t.Setenv("XDG_CONFIG_HOME", processHome)
	t.Setenv("AKT_CONTEXT", "")
	t.Setenv("AKT_CONSOLE_API_KEY", "")
	t.Setenv("AKT_EXPERIMENTAL_TUI", "")

	report := discoverCommandInventory()
	assertInventoryIsMachineReadable(t, report)
	assertBuiltInWorkflowsDiscovered(t, report.Commands)
	assertUniqueCommandPaths(t, report.Commands)
	assertExactExcludedCommands(t, report.Excluded, map[string]string{
		// Cobra injects this transport-only dispatcher. It is not an akt action;
		// every target it can dispatch to is inventoried and exercised directly.
		"akt help": "cobra help plumbing",
	})
	assertOnlyApprovedInventoryExclusions(t, report.Excluded)
	assertDirectoryEmpty(t, aktHome, "root discovery")
	t.Logf("discovered %d visible command paths; deliberately excluded %d internal paths", len(report.Commands), len(report.Excluded))

	for _, entry := range report.Commands {
		t.Run(entry.Path, func(t *testing.T) {
			args := append(append([]string(nil), entry.Args...), "--help")
			stdout, stderr, exitCode := runAktNoHome(t, args...)
			if exitCode != 0 {
				t.Fatalf("%s --help exited %d\nstdout: %s\nstderr: %s", entry.Path, exitCode, stdout, stderr)
			}
			if strings.TrimSpace(stdout) == "" {
				t.Fatalf("%s --help produced no stdout\nstderr: %s", entry.Path, stderr)
			}
		})
	}

	assertDirectoryEmpty(t, aktHome, "help subprocesses")
	assertDirectoryEmpty(t, sandbox, "command discovery and help subprocesses")
	assertDirectoryEmpty(t, processHome, "command discovery and help subprocesses")
}

func discoverCommandInventory() commandInventoryReport {
	root := cli.NewRootCmd(cli.BuildInfo{Version: "test"})
	builtInWorkflows := builtin.Workflows()

	report := commandInventoryReport{SchemaVersion: commandInventorySchemaVersion}
	report.Commands = append(report.Commands, inventoryEntry(root, nil, builtInWorkflows))

	var walk func(*cobra.Command, []string)
	walk = func(parent *cobra.Command, parentArgs []string) {
		for _, cmd := range parent.Commands() {
			args := append(append([]string(nil), parentArgs...), cmd.Name())
			path := "akt " + strings.Join(args, " ")

			switch {
			case isDefaultCobraHelpCommand(cmd):
				report.Excluded = append(report.Excluded, excludedCommandEntry{Path: path, Reason: "cobra help plumbing"})
				continue
			case cmd.Hidden:
				// Hidden executable commands are still directly invocable and must
				// therefore remain in the structural inventory. A future truly
				// internal helper needs an exact reviewed exclusion below.
			case cmd.Deprecated != "":
				// Deprecated commands remain user-reachable until removal, so they
				// receive the same help and scenario accounting as current commands.
			case !cmd.IsAvailableCommand() && !cmd.IsAdditionalHelpTopicCommand():
				if reason, approved := approvedInventoryExclusion(path); approved {
					report.Excluded = append(report.Excluded, excludedCommandEntry{Path: path, Reason: reason})
					continue
				}
			}

			report.Commands = append(report.Commands, inventoryEntry(cmd, args, builtInWorkflows))
			walk(cmd, args)
		}
	}
	walk(root, nil)

	sort.Slice(report.Commands, func(i, j int) bool {
		return report.Commands[i].Path < report.Commands[j].Path
	})
	sort.Slice(report.Excluded, func(i, j int) bool {
		return report.Excluded[i].Path < report.Excluded[j].Path
	})

	return report
}

func approvedInventoryExclusion(path string) (string, bool) {
	// Keep approvals exact and local. Broad hidden/deprecated rules would let a
	// newly shipped action disappear from the generated coverage contract.
	switch path {
	default:
		return "", false
	}
}

func inventoryEntry(cmd *cobra.Command, args []string, builtInWorkflows map[string][]byte) commandInventoryEntry {
	kind := "group"
	isBuiltInWorkflow := false
	if len(args) == 1 {
		_, isBuiltInWorkflow = builtInWorkflows[args[0]]
	}
	switch {
	case len(args) == 0:
		kind = "root"
	case isBuiltInWorkflow:
		kind = "builtin-workflow"
	case cmd.IsAdditionalHelpTopicCommand():
		kind = "help-topic"
	case cmd.Runnable() && cmd.HasAvailableSubCommands():
		kind = "action-group"
	case cmd.Runnable():
		kind = "action"
	}

	aliases := append([]string(nil), cmd.Aliases...)
	sort.Strings(aliases)

	path := "akt"
	if len(args) != 0 {
		path += " " + strings.Join(args, " ")
	}

	return commandInventoryEntry{
		Path:           path,
		Args:           append([]string(nil), args...),
		Kind:           kind,
		Requirement:    inheritedCapability(cmd),
		Aliases:        aliases,
		Runnable:       cmd.Runnable(),
		HasSubcommands: cmd.HasAvailableSubCommands(),
	}
}

func inheritedCapability(cmd *cobra.Command) string {
	for current := cmd; current != nil; current = current.Parent() {
		if requirement := current.Annotations[capability.AnnotationKey]; requirement != "" {
			return requirement
		}
	}
	return ""
}

func isDefaultCobraHelpCommand(cmd *cobra.Command) bool {
	return cmd.Name() == "help" &&
		cmd.Use == "help [command]" &&
		cmd.Short == "Help about any command"
}

func assertInventoryIsMachineReadable(t *testing.T, report commandInventoryReport) {
	t.Helper()

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		t.Fatalf("encode command inventory: %v", err)
	}

	var decoded commandInventoryReport
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatalf("decode command inventory: %v", err)
	}
	if decoded.SchemaVersion != commandInventorySchemaVersion || len(decoded.Commands) != len(report.Commands) {
		t.Fatalf("command inventory JSON round trip changed report: got schema %d with %d commands", decoded.SchemaVersion, len(decoded.Commands))
	}
}

func assertBuiltInWorkflowsDiscovered(t *testing.T, entries []commandInventoryEntry) {
	t.Helper()

	discovered := make(map[string]commandInventoryEntry, len(entries))
	for _, entry := range entries {
		discovered[entry.Path] = entry
	}

	names := make([]string, 0, len(builtin.Workflows()))
	for name := range builtin.Workflows() {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		path := "akt " + name
		entry, ok := discovered[path]
		if !ok {
			t.Errorf("dynamically generated built-in workflow %q is missing from command inventory", path)
			continue
		}
		if entry.Kind != "builtin-workflow" {
			t.Errorf("%s inventory kind = %q, want builtin-workflow", path, entry.Kind)
		}
	}
}

func assertUniqueCommandPaths(t *testing.T, entries []commandInventoryEntry) {
	t.Helper()

	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, ok := seen[entry.Path]; ok {
			t.Errorf("duplicate visible command path %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
	}
}

func assertExactExcludedCommands(t *testing.T, entries []excludedCommandEntry, expected map[string]string) {
	t.Helper()

	for _, entry := range entries {
		reason, reviewed := expected[entry.Path]
		if !reviewed {
			t.Errorf("command inventory silently excluded unreviewed path %q (%s)", entry.Path, entry.Reason)
			continue
		}
		if entry.Reason != reason {
			t.Errorf("excluded command %q reason = %q, want %q", entry.Path, entry.Reason, reason)
		}
		delete(expected, entry.Path)
	}
	for path, reason := range expected {
		t.Errorf("reviewed exclusion %q (%s) is no longer present", path, reason)
	}
}

func assertOnlyApprovedInventoryExclusions(t *testing.T, entries []excludedCommandEntry) {
	t.Helper()
	for _, entry := range entries {
		if entry.Path == "akt help" && entry.Reason == "cobra help plumbing" {
			continue
		}
		reason, approved := approvedInventoryExclusion(entry.Path)
		if !approved || reason != entry.Reason {
			t.Errorf("command inventory has unapproved exclusion %q (%s)", entry.Path, entry.Reason)
		}
	}
}

func assertDirectoryEmpty(t *testing.T, path, operation string) {
	t.Helper()

	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read isolated directory %s after %s: %v", path, operation, err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("%s wrote to isolated directory %s: %s", operation, path, strings.Join(names, ", "))
	}
}
