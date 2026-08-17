package output

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

func TestConstrainOutputFlagUpdatesItsAdvertisedEnum(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String(flagdefs.FlagOutput, "text", "Output format (text|json)")
	flag := flags.Lookup(flagdefs.FlagOutput)

	ConstrainFlag(flag, "pretty", "pretty", "json", "yaml")

	require.Equal(t, "Output format (pretty|json|yaml)", flag.Usage)
	require.NoError(t, flag.Value.Set("yaml"))
	require.Error(t, flag.Value.Set("text"))
}
