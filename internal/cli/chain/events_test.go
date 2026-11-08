package cli

import (
	"context"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

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

func TestEventsRejectsInvalidNodeURI(t *testing.T) {
	cmd := EventsCmd()
	cctx := sdkclient.Context{}.WithNodeURI("http://%")
	cmd.SetContext(context.WithValue(context.Background(), ClientContextKey, &cctx))
	require.ErrorContains(t, getEvents(cmd.Context(), cmd, nil), "invalid URL escape")
}
