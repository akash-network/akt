package testutil

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"fmt"
	"reflect"
	"strconv"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

// FlagsSet is an immutable CLI argument set derived from
// pkg.akt.dev/go/cli.TestFlags in akash-network/chain-sdk.
type FlagsSet []string

// Stringer is the value contract accepted by FlagsSet.WithFlag.
type Stringer interface {
	String() string
}

// TestFlags returns an empty CLI argument set.
func TestFlags() FlagsSet {
	return FlagsSet{}
}

// With returns a new argument set with args appended.
func (flags FlagsSet) With(args ...string) FlagsSet {
	result := make([]string, len(flags), len(flags)+len(args))
	copy(result, flags)

	return append(result, args...)
}

// Append returns a new argument set with rhs appended.
func (flags FlagsSet) Append(rhs FlagsSet) FlagsSet {
	return flags.With(rhs...)
}

// WithFlag returns a new argument set containing --key=value.
func (flags FlagsSet) WithFlag(key string, value any) FlagsSet {
	var rendered string

	switch value := value.(type) {
	case string:
		rendered = value
	case int, uint, int64, uint64:
		rendered = fmt.Sprintf("%d", value)
	case bool:
		rendered = strconv.FormatBool(value)
	case Stringer:
		rendered = value.String()
	default:
		panic(fmt.Sprintf(
			"val %s is not a type of real|bool|string|Stringer",
			reflect.TypeOf(value).String(),
		))
	}

	return flags.With(fmt.Sprintf("--%s=%s", key, rendered))
}

// WithFrom appends the transaction signer flag.
func (flags FlagsSet) WithFrom(account string) FlagsSet {
	return flags.WithFlag(flagdefs.FlagFrom, account)
}

// WithGenerateOnly appends the transaction generation-only flag.
func (flags FlagsSet) WithGenerateOnly() FlagsSet {
	return flags.WithFlag(flagdefs.FlagGenerateOnly, true)
}

// WithOffline appends the offline transaction flag.
func (flags FlagsSet) WithOffline() FlagsSet {
	return flags.WithFlag(flagdefs.FlagOffline, true)
}

// WithGas appends the gas limit flag.
func (flags FlagsSet) WithGas(gas int) FlagsSet {
	return flags.WithFlag(flagdefs.FlagGas, gas)
}

// WithChainID appends the chain ID flag.
func (flags FlagsSet) WithChainID(chainID string) FlagsSet {
	return flags.WithFlag(flagdefs.FlagChainID, chainID)
}

// WithOutput appends the output format flag.
func (flags FlagsSet) WithOutput(format string) FlagsSet {
	return flags.WithFlag(flagdefs.FlagOutput, format)
}

// WithOutputJSON appends the JSON output format flag.
func (flags FlagsSet) WithOutputJSON() FlagsSet {
	return flags.WithOutput(cflags.OutputJSON)
}
