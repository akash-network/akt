package cli

import "fmt"

// requireOwnerScope refuses an owner-mode list query that has no owner to
// filter on. SPEC §3.8 defines a bare listing as "all matching resources for
// the context's default account", so with no account configured there is
// nothing to scope to. The query layer treats an empty owner as "no filter"
// rather than "no results", so sending it anyway returns every matching
// resource on the network under a heading the caller reads as their own.
//
// A context legitimately has no default account -- console-api contexts never
// set one, and the monitoring context in SPEC §5 omits it deliberately -- so
// this is a normal state to hit, not a misconfiguration.
func requireOwnerScope(resource string) error {
	return fmt.Errorf("%s: no default account set; provide an owner address or configure default-account", resource)
}

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

// requireProviderScope refuses a --by provider list query with no provider.
// SPEC §3.8.4 marks the leading address required in provider mode; nothing
// enforced it, so the empty filter listed the whole network.
func requireProviderScope(resource string) error {
	return fmt.Errorf("%s: --by provider requires a provider address; provide one as the first argument", resource)
}
