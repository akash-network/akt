package cli

import (
	"fmt"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/pflag"
)

// parseByPerspective reads --by, which selects whether the leading address is
// an owner or a provider. It used to be compared against "provider" and
// nothing else, so every other value -- including "Provider" -- fell through
// to owner mode and answered a different question than the one asked.
func parseByPerspective(v string) (bool, error) {
	switch v {
	case "owner":
		return false, nil
	case "provider":
		return true, nil
	default:
		return false, fmt.Errorf("--by must be %q or %q, got %q", "owner", "provider", v)
	}
}

// rejectUnsupportedPaging refuses the pagination flags the escrow endpoints
// silently ignore.
//
// AddPaginationFlagsToCmd advertises the full set, but escrow honours only
// --limit and --page-key: --offset, --page and --reverse were accepted and
// dropped, so paging through escrow accounts returned page one every time and
// a caller summing the pages counted the same balances repeatedly. Refuse them
// rather than answer with the wrong page.
func rejectUnsupportedPaging(flags *pflag.FlagSet, resource string) error {
	for _, name := range []string{flagdefs.FlagOffset, flagdefs.FlagPage, flagdefs.FlagReverse} {
		if flags.Changed(name) {
			return fmt.Errorf("%s: --%s is not supported by this endpoint; page with --limit and --page-key", resource, name)
		}
	}

	return nil
}

// rejectOffsetPaging refuses --page and --offset on a listing that cannot
// carry a state filter. The chain accepts an offset only together with one,
// so the flags would fail there with an error about a filter the caller has
// no way to set.
func rejectOffsetPaging(flags *pflag.FlagSet, resource string) error {
	for _, name := range []string{flagdefs.FlagOffset, flagdefs.FlagPage} {
		if flags.Changed(name) {
			return fmt.Errorf("%s: --%s needs a state filter, which this listing cannot take; page with --limit and --page-key", resource, name)
		}
	}

	return nil
}
