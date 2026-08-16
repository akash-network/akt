package cli

import (
	"strings"
	"testing"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

func TestAssembledTransactionModeFlagsAreClosedEnums(t *testing.T) {
	root := NewRootCmd(BuildInfo{Version: "test"})
	seen := make(map[*pflag.Flag]struct{})
	counts := map[string]int{}

	allowed := map[string][]string{
		flagdefs.FlagSignMode: {
			cflags.SignModeDirect,
			cflags.SignModeLegacyAminoJSON,
			cflags.SignModeDirectAux,
			cflags.SignModeEIP191,
		},
		flagdefs.FlagBroadcastMode: {
			cflags.BroadcastSync,
			cflags.BroadcastAsync,
			cflags.BroadcastBlock,
		},
	}
	defaults := map[string]string{
		flagdefs.FlagSignMode:      cflags.SignModeDirect,
		flagdefs.FlagBroadcastMode: cflags.BroadcastSync,
	}

	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, set := range []*pflag.FlagSet{cmd.LocalFlags(), cmd.PersistentFlags()} {
			for name, values := range allowed {
				flag := set.Lookup(name)
				if flag == nil {
					continue
				}
				if _, ok := seen[flag]; ok {
					continue
				}
				seen[flag] = struct{}{}
				counts[name]++
				if flag.DefValue != defaults[name] {
					t.Errorf("%s --%s default = %q, want %q", cmd.CommandPath(), name, flag.DefValue, defaults[name])
				}
				for _, value := range values {
					if !strings.Contains(flag.Usage, value) {
						t.Errorf("%s --%s help %q omits %q", cmd.CommandPath(), name, flag.Usage, value)
					}
					if err := flag.Value.Set(value); err != nil {
						t.Errorf("%s --%s %s: %v", cmd.CommandPath(), name, value, err)
					}
				}
				if err := flag.Value.Set("not-a-mode"); err == nil {
					t.Errorf("%s --%s accepted an unknown value", cmd.CommandPath(), name)
				}
			}
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)

	for name := range allowed {
		if counts[name] == 0 {
			t.Errorf("assembled tree has no --%s flags", name)
		}
	}
}
