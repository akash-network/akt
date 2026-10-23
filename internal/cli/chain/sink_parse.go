package cli

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/mr-tron/base58"

	types "pkg.akt.dev/go/node/sink/v1"
)

// sinkSignatureSize is the length of a Solana transaction signature.
const sinkSignatureSize = 64

// parseSinkTransferID accepts the canonical decimal form the module itself
// uses for record keys: a positive integer with no sign, leading zeros or
// whitespace.
func parseSinkTransferID(value string) (uint64, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != value {
		return 0, fmt.Errorf("invalid sink transfer ID %q: use a positive whole number without leading zeros", value)
	}

	return id, nil
}

// parseSinkAmount converts a uakt or AKT amount exactly. A fractional uakt
// value is rejected rather than truncated, so 1.5uakt never becomes 1uakt.
func parseSinkAmount(value string) (sdk.Coin, error) {
	coin, err := sdk.ParseDecCoin(strings.TrimSpace(value))
	if err != nil {
		return sdk.Coin{}, fmt.Errorf("invalid amount %q: use whole uakt (1500000uakt) or an exact AKT decimal (1.5akt): %w", value, err)
	}

	amount := coin.Amount
	switch strings.ToLower(coin.Denom) {
	case types.Denom:
	case "akt":
		amount = amount.MulInt64(1_000_000)
	default:
		return sdk.Coin{}, fmt.Errorf("invalid amount %q: sink deposits are AKT, given in uakt or akt", value)
	}

	if !amount.IsInteger() {
		return sdk.Coin{}, fmt.Errorf("invalid amount %q: not a whole number of uakt", value)
	}

	whole := amount.TruncateInt()
	if !whole.IsPositive() || !whole.IsUint64() {
		return sdk.Coin{}, fmt.Errorf("invalid amount %q: must be a positive uint64 number of uakt", value)
	}

	return sdk.NewCoin(types.Denom, math.NewIntFromUint64(whole.Uint64())), nil
}

// parseSinkOutcome maps the reported Solana outcome to its enum.
func parseSinkOutcome(value string) (types.Outcome, error) {
	switch strings.ToLower(value) {
	case "minted":
		return types.Outcome_OUTCOME_MINTED, nil
	case "voided":
		return types.Outcome_OUTCOME_VOIDED, nil
	default:
		return types.Outcome_OUTCOME_UNSPECIFIED, fmt.Errorf("invalid outcome %q: use minted or voided", value)
	}
}

// parseSinkSlot accepts a positive Solana slot.
func parseSinkSlot(value string) (uint64, error) {
	slot, err := strconv.ParseUint(value, 10, 64)
	if err != nil || slot == 0 {
		return 0, fmt.Errorf("invalid Solana slot %q: use a positive whole number", value)
	}

	return slot, nil
}

// parseSinkSignature decodes a base58 Solana transaction signature. The
// module requires exactly 64 bytes that are not all zero.
func parseSinkSignature(value string) ([]byte, error) {
	signature, err := base58.Decode(value)
	if err != nil || len(signature) != sinkSignatureSize || bytes.Equal(signature, make([]byte, sinkSignatureSize)) {
		return nil, fmt.Errorf("invalid Solana transaction signature %q: use the base58 signature of a finalized transaction", value)
	}

	return signature, nil
}

// parseSinkObservation builds and checks the Solana evidence a report or a
// governance confirmation carries.
func parseSinkObservation(outcome types.Outcome, slotArg, signatureArg string) (types.Observation, error) {
	slot, err := parseSinkSlot(slotArg)
	if err != nil {
		return types.Observation{}, err
	}

	signature, err := parseSinkSignature(signatureArg)
	if err != nil {
		return types.Observation{}, err
	}

	observation := types.Observation{Outcome: outcome, Slot: slot, TransactionSignature: signature}
	if err := observation.Validate(); err != nil {
		return types.Observation{}, err
	}

	return observation, nil
}

// parseSinkDecision maps a governance resolution to its enum.
func parseSinkDecision(value string) (types.Decision, error) {
	switch strings.ToLower(value) {
	case "restart":
		return types.Decision_DECISION_RESTART, nil
	case "confirm-mint":
		return types.Decision_DECISION_CONFIRM_MINT, nil
	case "confirm-void":
		return types.Decision_DECISION_CONFIRM_VOID, nil
	default:
		return types.Decision_DECISION_UNSPECIFIED, fmt.Errorf("invalid decision %q: use restart, confirm-mint or confirm-void", value)
	}
}

// parseSinkPauseTarget names what a guardian pause disables.
func parseSinkPauseTarget(value string) (deposits, refunds bool, err error) {
	switch strings.ToLower(value) {
	case "deposits":
		return true, false, nil
	case "refunds":
		return false, true, nil
	case "all":
		return true, true, nil
	default:
		return false, false, fmt.Errorf("invalid pause target %q: use deposits, refunds or all", value)
	}
}

// parseSinkReason checks a veto or resolution reason against the module's
// limit before it costs gas.
func parseSinkReason(value string) (string, error) {
	if err := types.ValidateReason(value); err != nil {
		return "", fmt.Errorf("invalid reason: %w", err)
	}

	return value, nil
}
