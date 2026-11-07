package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	flagdefs "pkg.akt.dev/akt/internal/flags"
)

func TestCompatibilityReadersRejectMissingOrConflictingFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, err := DSeqFromFlagsAndArgs(fs, nil)
	require.ErrorContains(t, err, "dseq")
	_, err = ExpressionFromFlagsAndArgs(fs, nil, flagdefs.FlagQuery)
	require.ErrorContains(t, err, "query")
	fs.Uint64(flagdefs.FlagDSeq, 0, "")
	fs.String(flagdefs.FlagQuery, "", "")
	require.NoError(t, fs.Set(flagdefs.FlagDSeq, "42"))
	require.NoError(t, fs.Set(flagdefs.FlagQuery, "block.height > 1"))
	_, err = DSeqFromFlagsAndArgs(fs, []string{"43"})
	require.ErrorContains(t, err, "both positionally")
	_, err = ExpressionFromFlagsAndArgs(fs, []string{"other"}, flagdefs.FlagQuery)
	require.ErrorContains(t, err, "both positionally")
}

func TestFilterReadersRejectIncompleteSchemasAndInvalidAddresses(t *testing.T) {
	for _, tc := range []struct {
		name string
		add  func(*pflag.FlagSet)
		read func(*pflag.FlagSet) error
	}{
		{"deployment", AddDeploymentFilterFlags, func(f *pflag.FlagSet) error { _, err := DepFiltersFromFlags(f); return err }},
		{"order", AddOrderFilterFlags, func(f *pflag.FlagSet) error { _, err := OrderFiltersFromFlags(f); return err }},
		{"bid", AddBidFilterFlags, func(f *pflag.FlagSet) error { _, err := BidFiltersFromFlags(f); return err }},
	} {
		full := pflag.NewFlagSet(tc.name, pflag.ContinueOnError)
		tc.add(full)
		full.VisitAll(func(omitted *pflag.Flag) {
			t.Run(tc.name+"/missing-"+omitted.Name, func(t *testing.T) {
				partial := pflag.NewFlagSet(tc.name, pflag.ContinueOnError)
				full.VisitAll(func(f *pflag.Flag) {
					if f.Name != omitted.Name {
						partial.AddFlag(f)
					}
				})
				require.ErrorContains(t, tc.read(partial), omitted.Name)
			})
		})
		for _, name := range []string{flagdefs.FlagOwner, flagdefs.FlagProvider} {
			fs := pflag.NewFlagSet(tc.name, pflag.ContinueOnError)
			tc.add(fs)
			if fs.Lookup(name) == nil {
				continue
			}
			require.NoError(t, fs.Set(name, "invalid-address"))
			require.Error(t, tc.read(fs))
		}
	}
}

func TestCompatibilityArgumentValidatorsRejectExtraAndDuplicateState(t *testing.T) {
	cmd := &cobra.Command{}
	AddDeploymentFilterFlags(cmd.Flags())
	require.Error(t, ResourceFilterArgs(nil)(cmd, []string{"one", "two"}))
	require.Error(t, PositionalOrFlagArgs(flagdefs.FlagDSeq)(cmd, []string{"one", "two"}))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagState, "active"))
	require.ErrorContains(t, ResourceFilterArgs(DeploymentStateFromArg)(cmd, []string{"active"}), "both positionally")
}
