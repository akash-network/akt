package cli

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestEveryLeafHasExample(t *testing.T) {
	root := NewRootCmd(BuildInfo{Version: "test"})

	var missing []string
	walkCommands(root, func(cmd *cobra.Command) {
		if cmd.HasAvailableSubCommands() || strings.TrimSpace(cmd.Example) != "" {
			return
		}

		missing = append(missing, strings.TrimPrefix(cmd.CommandPath(), root.Name()+" "))
	})

	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("leaf commands without an Example field:\n  %s", strings.Join(missing, "\n  "))
	}
}

func TestLeafExamplesUseTheirCommandAndKnownFlags(t *testing.T) {
	root := NewRootCmd(BuildInfo{Version: "test"})

	walkCommands(root, func(cmd *cobra.Command) {
		if cmd.HasAvailableSubCommands() {
			return
		}

		path := strings.TrimPrefix(cmd.CommandPath(), root.Name()+" ")
		example := strings.ReplaceAll(cmd.Example, "\\\n", " ")
		var invocation string
		for _, line := range strings.Split(example, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "$ "))
			if strings.HasPrefix(line, "akt "+path) {
				invocation = line
				break
			}
			if strings.HasPrefix(path, "query ") && strings.HasPrefix(line, "akt q "+strings.TrimPrefix(path, "query ")) {
				invocation = line
				break
			}
		}

		if invocation == "" {
			t.Errorf("%s: no example invokes its own command:\n%s", path, cmd.Example)
			return
		}

		for _, field := range strings.Fields(invocation) {
			if field == "--" || !strings.HasPrefix(field, "--") {
				continue
			}

			name := strings.TrimPrefix(field, "--")
			if before, _, ok := strings.Cut(name, "="); ok {
				name = before
			}
			name = strings.TrimRight(name, "'\"),;\\")
			if name == "help" || cmd.Flag(name) != nil {
				continue
			}

			t.Errorf("%s: example uses unknown flag --%s", path, name)
		}
	})
}

func TestHelpContainsNoInternalLanguage(t *testing.T) {
	root := NewRootCmd(BuildInfo{Version: "test"})
	forbidden := []string{
		"spec §",
		"feedback(",
		"<appd>",
		"ai assistant",
		"ai agent",
	}

	var violations []string
	walkCommands(root, func(cmd *cobra.Command) {
		help := strings.ToLower(strings.Join([]string{cmd.Short, cmd.Long, cmd.Example}, "\n"))
		for _, phrase := range forbidden {
			if strings.Contains(help, phrase) {
				violations = append(violations, cmd.CommandPath()+": "+phrase)
			}
		}
	})

	sort.Strings(violations)
	if len(violations) != 0 {
		t.Fatalf("user-facing help contains internal language:\n  %s", strings.Join(violations, "\n  "))
	}
}

func TestCorrectedHelpExamplesUseRegisteredSurface(t *testing.T) {
	root := NewRootCmd(BuildInfo{Version: "test"})

	tests := []struct {
		path       string
		contains   []string
		notContain []string
	}{
		{
			path:       "update",
			contains:   []string{"akt update deploy.yaml 12345"},
			notContain: []string{"akt update 12345 deploy.yaml"},
		},
		{
			path:       "query escrow payments",
			contains:   []string{"akt query escrow payments"},
			notContain: []string{"akt query escrow accounts"},
		},
		{
			path:       "query ibc channelv2 unreceived-packets",
			contains:   []string{"query ibc channelv2 unreceived-packets"},
			notContain: []string{"query ibc channelv2 unreceived-packet "},
		},
		{
			path:       "query ibc client config",
			contains:   []string{"query ibc client config 08-wasm-0"},
			notContain: []string{"query ibc client params 08-wasm-0"},
		},
		{
			path:       "query staking redelegations",
			contains:   []string{"query staking redelegations"},
			notContain: []string{"query staking redelegation "},
		},
		{
			path:       "tx ibc client update-client-config",
			contains:   []string{"tx ibc client update-client-config"},
			notContain: []string{"tx ibc client update-client-params"},
		},
		{
			path:       "tx wasm instantiate",
			contains:   []string{"akt context keys show mykey -a"},
			notContain: []string{"akt keys show mykey -a"},
		},
		{
			path:       "query ibc client consensus-state",
			contains:   []string{"--latest-height"},
			notContain: []string{"'--latest' flag"},
		},
		{
			path:       "tx gov submit-legacy-proposal param-change",
			contains:   []string{"tx gov submit-legacy-proposal param-change"},
			notContain: []string{"tx gov submit-proposal param-change"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			cmd, _, err := root.Find(strings.Fields(tt.path))
			if err != nil {
				t.Fatalf("find command: %v", err)
			}
			if got := strings.TrimPrefix(cmd.CommandPath(), root.Name()+" "); got != tt.path {
				t.Fatalf("resolved %q to %q", tt.path, got)
			}

			help := strings.Join([]string{cmd.Long, cmd.Example}, "\n")
			for _, want := range tt.contains {
				if !strings.Contains(help, want) {
					t.Errorf("help does not contain %q:\n%s", want, help)
				}
			}
			for _, unwanted := range tt.notContain {
				if strings.Contains(help, unwanted) {
					t.Errorf("help contains stale text %q:\n%s", unwanted, help)
				}
			}
		})
	}
}

func walkCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		walkCommands(child, visit)
	}
}
