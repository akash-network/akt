package flags

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
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

// LeaseSeqsFromArgs resolves a deployment sequence and provider address from
// optional positional arguments in the form [dseq [provider]]. Positional
// values always win over the flag-derived fallbacks (SPEC §3.8.2).
func LeaseSeqsFromArgs(args []string, dseq uint64, provider string) (uint64, string, error) {
	dseq, err := DSeqFromArgs(args, dseq)
	if err != nil {
		return 0, "", err
	}

	if len(args) > 1 {
		if _, err := sdk.AccAddressFromBech32(args[1]); err != nil {
			return 0, "", fmt.Errorf("invalid provider %q: %w", args[1], err)
		}

		provider = args[1]
	}

	return dseq, provider, nil
}

// ExprFromArgs resolves a search expression from an optional positional
// argument. When args is empty the flag-derived fallback is returned; a
// positional expression always wins over the fallback (SPEC §3.8.2).
func ExprFromArgs(args []string, fallback string) string {
	if len(args) == 0 {
		return fallback
	}

	return args[0]
}

// DeploymentIDFromArgs resolves a deployment ID from an optional positional
// [owner/]dseq filter argument (SPEC §3.8), starting from the flag-derived
// fallback. Positional components win over the fallback and the owner falls
// back to defaultOwner when neither source sets it (SPEC §3.8.4).
func DeploymentIDFromArgs(args []string, fallback dv1.DeploymentID, defaultOwner string) (dv1.DeploymentID, error) {
	id := fallback

	if len(args) > 0 {
		f, err := DepFiltersFromArg(args[0], defaultOwner)
		if err != nil {
			return dv1.DeploymentID{}, err
		}

		if f.State != "" {
			return dv1.DeploymentID{}, fmt.Errorf("deployment ID: expected [owner/]dseq, got state keyword %q", args[0])
		}

		if f.Owner != "" {
			id.Owner = f.Owner
		}

		if f.DSeq != 0 {
			id.DSeq = f.DSeq
		}
	}

	if id.Owner == "" {
		id.Owner = defaultOwner
	}

	return id, nil
}
