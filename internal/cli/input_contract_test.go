package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestEveryCommandGroupRejectsUnknownSubcommands(t *testing.T) {
	t.Setenv("AKT_HOME", t.TempDir())
	root := NewRootCmd(BuildInfo{Version: "test"})

	var violations []string
	walkInputContractCommands(root, func(cmd *cobra.Command) {
		if cmd.HasSubCommands() && !cmd.Runnable() {
			violations = append(violations, cmd.CommandPath())
		}
	})

	if len(violations) != 0 {
		t.Fatalf("command groups without an input validator:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

func TestEveryOutputFlagRejectsUnknownFormats(t *testing.T) {
	t.Setenv("AKT_HOME", t.TempDir())
	root := NewRootCmd(BuildInfo{Version: "test"})

	seen := make(map[*pflag.Flag]string)
	var violations []string
	walkInputContractCommands(root, func(cmd *cobra.Command) {
		for _, flags := range []*pflag.FlagSet{cmd.LocalFlags(), cmd.PersistentFlags()} {
			flag := flags.Lookup("output")
			if flag == nil {
				continue
			}
			if _, ok := seen[flag]; ok {
				continue
			}
			seen[flag] = cmd.CommandPath()
			if flag.DefValue != "pretty" {
				violations = append(violations, cmd.CommandPath()+" (default "+flag.DefValue+")")
			}
			if err := flag.Value.Set("josn"); err == nil {
				violations = append(violations, cmd.CommandPath())
			}
		}
	})

	if len(violations) != 0 {
		t.Fatalf("--output accepted an undocumented value on:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

func TestOutputEnumsAreCommandSpecific(t *testing.T) {
	t.Setenv("AKT_HOME", t.TempDir())
	root := NewRootCmd(BuildInfo{Version: "test"})

	rootOutput := root.PersistentFlags().Lookup("output")
	for _, value := range []string{"pretty", "json", "yaml"} {
		if err := rootOutput.Value.Set(value); err != nil {
			t.Errorf("root --output %q: %v", value, err)
		}
	}
	for _, value := range []string{"table", "jsonl", "josn"} {
		if err := rootOutput.Value.Set(value); err == nil {
			t.Errorf("root --output unexpectedly accepted %q", value)
		}
	}

	deploy := childCommand(root, "deploy")
	if deploy == nil {
		t.Fatal("deploy command not found")
	}
	if err := deploy.Flags().Lookup("output").Value.Set("jsonl"); err != nil {
		t.Fatalf("deploy --output jsonl: %v", err)
	}
}

func TestAdoptedOutputHelpMatchesEnforcedEnum(t *testing.T) {
	t.Setenv("AKT_HOME", t.TempDir())
	root := NewRootCmd(BuildInfo{Version: "test"})

	states := childCommand(childCommand(childCommand(root, "query"), "ibc"), "client")
	states = childCommand(states, "states")
	if states == nil {
		t.Fatal("query ibc client states command not found")
	}

	flag := states.Flags().Lookup("output")
	if flag == nil {
		t.Fatal("query ibc client states output flag not found")
		return
	}
	if flag.Usage != "Output format (pretty|json|yaml)" {
		t.Fatalf("output help = %q", flag.Usage)
	}
}

func TestQuietHelpDescribesInformationalSuppression(t *testing.T) {
	t.Setenv("AKT_HOME", t.TempDir())
	root := NewRootCmd(BuildInfo{Version: "test"})

	usage := root.PersistentFlags().Lookup("quiet").Usage
	if !strings.Contains(usage, "informational") || strings.Contains(usage, "all output") {
		t.Fatalf("quiet help = %q", usage)
	}
}

func walkInputContractCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		walkInputContractCommands(child, visit)
	}
}

func childCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}
