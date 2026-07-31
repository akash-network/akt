package flags

import (
	"fmt"
	"strconv"
)

// parseSeq parses a dseq, gseq or oseq.
//
// Zero is refused. The sequence numbers are 1-based on chain, and every
// consumer treats a zero value as "this component was not supplied" -- so
// `owner/0` parsed fine, then had its dseq dropped from the filter and
// returned the owner's entire list instead of matching nothing, at exit 0.
func parseSeq(s string, bits int) (uint64, error) {
	v, err := strconv.ParseUint(s, 10, bits)
	if err != nil {
		return 0, err
	}

	if v == 0 {
		return 0, fmt.Errorf("must be 1 or greater")
	}

	return v, nil
}
