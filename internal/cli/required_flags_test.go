package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// requiredFlagAllowlist enumerates the only command flags that may be marked
// required. The CLI convention (SPEC §3.8, AKT-650) is positional-primary:
// a command's primary value is a positional argument and flags are optional
// overrides, so new required flags need an explicit entry here (and a good
// reason).
var requiredFlagAllowlist = map[string]bool{
	// A context's network is a named reference selected from shared
	// definitions; the spec defines it as a required flag (SPEC §2.2).
	"context create --network": true,
	// Explicit signer identity is the point of offline signing (cosmos convention).
	"tx sign --from":       true,
	"tx sign-batch --from": true,
	// The auditor identity must be explicit when creating/deleting audited attributes.
	"tx audit attr create --from": true,
	"tx audit attr delete --from": true,
	// Governance proposal metadata: multiple required fields, flag-shaped upstream in cosmos-sdk.
	"tx ibc client recover-client --title":       true,
	"tx ibc client schedule-ibc-upgrade --title": true,
	"tx upgrade software-upgrade --title":        true,
	"tx upgrade cancel-software-upgrade --title": true,
}

// TestNoUnapprovedRequiredFlags walks the full command tree and fails when a
// flag is marked required without an allowlist entry. This pins the
// positional-primary convention: primary values must be accepted as
// positional arguments, with flags as optional overrides.
func TestNoUnapprovedRequiredFlags(t *testing.T) {
	root := NewRootCmd(BuildInfo{Version: "test"})

	var violations []string

	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		check := func(f *pflag.Flag) {
			required := f.Annotations[cobra.BashCompOneRequiredFlag]
			if len(required) == 0 || required[0] != "true" {
				return
			}

			key := strings.TrimSpace(path) + " --" + f.Name
			if !requiredFlagAllowlist[key] {
				violations = append(violations, key)
			}
		}

		cmd.LocalFlags().VisitAll(check)

		for _, sub := range cmd.Commands() {
			name := sub.Name()
			if path == "" {
				walk(sub, name)
			} else {
				walk(sub, path+" "+name)
			}
		}
	}

	walk(root, "")

	if len(violations) > 0 {
		t.Errorf("commands with required flags not in the allowlist (primary values should be positional per SPEC §3.8):\n  %s",
			strings.Join(violations, "\n  "))
	}
}
