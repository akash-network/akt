package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type flagStringer string

func (value flagStringer) String() string {
	return string(value)
}

func TestFlagsSetBuildsRecurringChainArguments(t *testing.T) {
	t.Parallel()

	base := TestFlags().With("deployment.yaml")
	args := base.
		WithFrom("akash1owner").
		WithGenerateOnly().
		WithOffline().
		WithGas(200000).
		WithChainID("chain-test").
		WithOutputJSON()

	require.Equal(t, FlagsSet{"deployment.yaml"}, base)
	require.Equal(t, FlagsSet{
		"deployment.yaml",
		"--from=akash1owner",
		"--generate-only=true",
		"--offline=true",
		"--gas=200000",
		"--chain-id=chain-test",
		"--output=json",
	}, args)
}

func TestFlagsSetBranchesDoNotMutateTheirBase(t *testing.T) {
	t.Parallel()

	base := TestFlags().With("query")
	left := base.WithFlag("height", int64(7))
	right := base.WithFlag("prove", true)
	combined := left.Append(right)

	require.Equal(t, FlagsSet{"query"}, base)
	require.Equal(t, FlagsSet{"query", "--height=7"}, left)
	require.Equal(t, FlagsSet{"query", "--prove=true"}, right)
	require.Equal(t, FlagsSet{
		"query",
		"--height=7",
		"query",
		"--prove=true",
	}, combined)

	combined[0] = "changed"
	require.Equal(t, FlagsSet{"query", "--height=7"}, left)
}

func TestFlagsSetWithFlagFormatsSupportedValues(t *testing.T) {
	t.Parallel()

	args := TestFlags().
		WithFlag("string", "value").
		WithFlag("int", int(1)).
		WithFlag("uint", uint(2)).
		WithFlag("int64", int64(3)).
		WithFlag("uint64", uint64(4)).
		WithFlag("bool", false).
		WithFlag("stringer", flagStringer("rendered"))

	require.Equal(t, FlagsSet{
		"--string=value",
		"--int=1",
		"--uint=2",
		"--int64=3",
		"--uint64=4",
		"--bool=false",
		"--stringer=rendered",
	}, args)
}

func TestFlagsSetWithFlagRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()

	require.PanicsWithValue(
		t,
		"val float64 is not a type of real|bool|string|Stringer",
		func() {
			_ = TestFlags().WithFlag("unsupported", float64(1))
		},
	)
}
