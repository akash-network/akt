package cli

import (
	"testing"

	"github.com/spf13/viper"

	flagdefs "pkg.akt.dev/akt/internal/flags"
)

func TestEventsCommandHasIndependentNodeFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := EventsCmd()
	flag := cmd.Flags().Lookup(flagdefs.FlagNode)
	if flag == nil || flag.DefValue != "tcp://localhost:26657" {
		t.Fatalf("events --%s flag = %#v", flagdefs.FlagNode, flag)
	}
	if got := viper.GetString(flagdefs.FlagNode); got != "" {
		t.Fatalf("events changed global node binding = %q", got)
	}
}
