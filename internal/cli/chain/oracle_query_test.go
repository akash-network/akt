package cli

import (
	"errors"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/cliutil"
	"pkg.akt.dev/go/sdkutil"
)

// The oracle keys prices by base denom ("akt") while the rest of the CLI uses
// the micro denom ("uakt"). Every spelling a user could reasonably type must
// reach the same key, otherwise this one command rejects the denom the tool
// asks for everywhere else.
func TestNormalizeOracleDenom(t *testing.T) {
	tests := map[string]string{
		"akt":   sdkutil.DenomAkt,
		"AKT":   sdkutil.DenomAkt,
		"uakt":  sdkutil.DenomAkt,
		"uAKT":  sdkutil.DenomAkt,
		"makt":  sdkutil.DenomAkt,
		" akt ": sdkutil.DenomAkt,
		"act":   sdkutil.DenomAct,
		"uact":  sdkutil.DenomAct,
		"ACT":   sdkutil.DenomAct,
		// Outside the AKT/ACT families there is no mapping to apply, and the
		// value may be case-sensitive.
		"usd":                "usd",
		"ibc/ABCDEF":         "ibc/ABCDEF",
		"some-other-denom":   "some-other-denom",
		"UnknownMixedCase":   "UnknownMixedCase",
		"uatom":              "uatom",
		"factory/akash1/foo": "factory/akash1/foo",
	}

	for in, want := range tests {
		if got := normalizeOracleDenom(in); got != want {
			t.Errorf("normalizeOracleDenom(%q) = %q, want %q", in, got, want)
		}
	}
}

// A raw gRPC/ABCI status reaching the user verbatim breaks the three-part error
// contract (SPEC §11.1). Oracle failures must name what was tried and how to
// find out what the oracle carries.
func TestOracleQueryErrorFollowsTheErrorContract(t *testing.T) {
	cause := errors.New("rpc error: code = Unknown desc = collections: not found")
	err := oracleQueryError(`no aggregated price for denom "akt"`, cause)

	var cliErr *cliutil.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("oracleQueryError returned %T, want *cliutil.CLIError", err)
	}
	if !errors.Is(err, cause) {
		t.Error("the underlying transport error must stay unwrappable")
	}

	msg := err.Error()
	for _, want := range []string{
		"Error:",
		"Context:",
		"Suggestion:",
		`no aggregated price for denom "akt"`,
		"akt q oracle prices",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q from the rendered error:\n%s", want, msg)
		}
	}
}
