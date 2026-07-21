package flags

import (
	"fmt"
	"strconv"
)

// DSeqFromArgs resolves a deployment sequence from an optional positional
// argument. When args is empty the flag-derived fallback is returned; when a
// positional dseq is present it always wins over the fallback (SPEC §3.8.2:
// positional values are applied after flags).
func DSeqFromArgs(args []string, fallback uint64) (uint64, error) {
	if len(args) == 0 {
		return fallback, nil
	}

	dseq, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid dseq %q: %w", args[0], err)
	}

	return dseq, nil
}

// GroupSeqsFromArgs resolves deployment and group sequences from optional
// positional arguments in the form [dseq [gseq]]. Positional values always
// win over the flag-derived fallbacks (SPEC §3.8.2).
func GroupSeqsFromArgs(args []string, dseq uint64, gseq uint32) (uint64, uint32, error) {
	dseq, err := DSeqFromArgs(args, dseq)
	if err != nil {
		return 0, 0, err
	}

	if len(args) > 1 {
		val, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid gseq %q: %w", args[1], err)
		}

		gseq = uint32(val)
	}

	return dseq, gseq, nil
}
