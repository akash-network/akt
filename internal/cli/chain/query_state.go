package cli

import "fmt"

// requireStateMatch enforces the optional positional [state] argument on the
// single-record get path (SPEC §3.8.3). When the identity components uniquely
// select one record, the state keyword is a verification rather than a list
// filter: the record is fetched by identity and printed only when its
// on-chain state matches the requested keyword. On a mismatch the command
// fails instead of silently printing a record in a different state.
// requested is empty when no positional state was supplied, in which case
// there is nothing to verify.
func requireStateMatch(resource, id, requested, actual string) error {
	if requested == "" || requested == actual {
		return nil
	}

	return fmt.Errorf("%s %s is %s, not %s; drop the state argument to print it regardless of state", resource, id, actual, requested)
}
