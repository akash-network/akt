package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestUpstreamCommandFlagCoverage(t *testing.T) {
	var audit struct {
		GlobalFlagExceptions map[string]string   `json:"global_flag_exceptions"`
		ExclusionReasons     map[string]string   `json:"exclusion_reasons"`
		FlagSets             map[string][]string `json:"flag_sets"`
		Commands             []struct {
			Source         string            `json:"source"`
			Target         string            `json:"target"`
			FlagSet        string            `json:"flag_set"`
			Excluded       string            `json:"excluded"`
			FlagExceptions map[string]string `json:"flag_exceptions"`
		} `json:"commands"`
	}
	data, err := os.ReadFile("testdata/command-flag-audit.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &audit))
	require.Len(t, audit.Commands, 346, "330 Akash paths, 11 provider commands, and 5 Console commands")

	root := NewRootCmd(BuildInfo{Version: "flag-audit"})
	commands := make(map[string]*cobra.Command)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		cmd.InitDefaultHelpFlag()
		commands[strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), "akt"))] = cmd
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	t.Logf("Compared %d upstream/trial entries against %d akt command paths", len(audit.Commands), len(commands))
	seen := make(map[string]bool)
	for _, entry := range audit.Commands {
		t.Run(entry.Source, func(t *testing.T) {
			require.False(t, seen[entry.Source], "duplicate source path")
			seen[entry.Source] = true
			flags, exists := audit.FlagSets[entry.FlagSet]
			require.True(t, exists, "unknown flag set %s", entry.FlagSet)
			if entry.Excluded != "" {
				require.NotEmpty(t, audit.ExclusionReasons[entry.Excluded])
				return
			}
			cmd := commands[entry.Target]
			require.NotNil(t, cmd, "missing akt counterpart: %s", entry.Target)
			for _, name := range flags {
				if reason, excluded := audit.GlobalFlagExceptions[name]; excluded {
					require.NotEmpty(t, audit.ExclusionReasons[reason])
					continue
				}
				if reason, excluded := entry.FlagExceptions[name]; excluded {
					require.NotEmpty(t, audit.ExclusionReasons[reason], "unexplained --%s exception", name)
					continue
				}
				flag := cmd.Flags().Lookup(name)
				if flag == nil {
					flag = cmd.InheritedFlags().Lookup(name)
				}
				require.NotNil(t, flag, "akt %s is missing --%s", entry.Target, name)
			}
		})
	}
}
