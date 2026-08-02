package workflow

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/types/bech32"

	"pkg.akt.dev/go/sdkutil"
)

// ValidateBidSelection validates the modes accepted by prompt steps and the
// bid-selection workflow parameter type.
func ValidateBidSelection(value string) error {
	switch value {
	case "interactive", "cheapest":
		return nil
	}

	provider, ok := strings.CutPrefix(value, "provider=")
	if !ok {
		return fmt.Errorf(
			"invalid bid selection %q: use interactive, cheapest, or provider=<full-address>",
			value,
		)
	}
	if provider == "" {
		return fmt.Errorf("invalid bid selection %q: provider address cannot be empty", value)
	}
	hrp, address, err := bech32.DecodeAndConvert(provider)
	if err != nil {
		return fmt.Errorf("invalid bid selection %q: invalid provider address: %w", value, err)
	}
	if hrp != sdkutil.Bech32PrefixAccAddr || len(address) != 20 {
		return fmt.Errorf(
			"invalid bid selection %q: invalid provider address: expected a full %s account address",
			value,
			sdkutil.Bech32PrefixAccAddr,
		)
	}

	return nil
}
