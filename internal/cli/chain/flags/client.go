package flags

import (
	"fmt"
	"strconv"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/pflag"

	cltypes "pkg.akt.dev/go/node/client/types"
)

// ClientOptionsFromFlags reads client options from cli flag set.
func ClientOptionsFromFlags(flagSet *pflag.FlagSet) ([]cltypes.ClientOption, error) {
	opts := make([]cltypes.ClientOption, 0)

	if flagSet.Changed(flagdefs.FlagAccountNumber) {
		accNum, _ := flagSet.GetUint64(flagdefs.FlagAccountNumber)
		opts = append(opts, cltypes.WithAccountNumber(accNum))
	}

	if flagSet.Changed(flagdefs.FlagSequence) {
		accSeq, _ := flagSet.GetUint64(flagdefs.FlagSequence)
		opts = append(opts, cltypes.WithAccountSequence(accSeq))
	}

	gasAdj, _ := flagSet.GetFloat64(flagdefs.FlagGasAdjustment)
	opts = append(opts, cltypes.WithGasAdjustment(gasAdj))

	if flagSet.Changed(flagdefs.FlagNote) {
		memo, _ := flagSet.GetString(flagdefs.FlagNote)
		opts = append(opts, cltypes.WithNote(memo))
	}

	if flagSet.Changed(flagdefs.FlagTimeoutHeight) {
		timeoutHeight, _ := flagSet.GetUint64(flagdefs.FlagTimeoutHeight)
		opts = append(opts, cltypes.WithTimeoutHeight(timeoutHeight))
	}

	if flagSet.Changed(flagdefs.FlagSkipConfirmation) {
		skip, _ := flagSet.GetBool(flagdefs.FlagSkipConfirmation)
		opts = append(opts, cltypes.WithSkipConfirm(skip))
	}

	// The gas setting must be applied even when the flag is left at its
	// default: "auto" means simulate-and-adjust, and skipping the option
	// entirely used to broadcast transactions with gasWanted=0, which fail
	// CheckTx with an out-of-gas error.
	if flagSet.Lookup(flagdefs.FlagGas) != nil {
		gasStr, _ := flagSet.GetString(flagdefs.FlagGas)
		gasSetting, err := ParseGasSetting(gasStr)
		if err != nil {
			return nil, err
		}
		opts = append(opts, cltypes.WithGas(gasSetting))
	}

	feesStr, _ := flagSet.GetString(flagdefs.FlagFees)
	if feesStr != "" {
		if _, err := sdk.ParseCoinsNormalized(feesStr); err != nil {
			return nil, fmt.Errorf("--%s: %w", flagdefs.FlagFees, err)
		}
		opts = append(opts, cltypes.WithFees(feesStr))
	} else {
		gasPrices, _ := flagSet.GetString(flagdefs.FlagGasPrices)
		if gasPrices != "" {
			if _, err := sdk.ParseDecCoins(gasPrices); err != nil {
				return nil, fmt.Errorf("--%s: %w", flagdefs.FlagGasPrices, err)
			}
			opts = append(opts, cltypes.WithGasPrices(gasPrices))
		}
	}

	signMode := SignModeDirect
	if flagSet.Changed(flagdefs.FlagSignMode) {
		signMode, _ = flagSet.GetString(flagdefs.FlagSignMode)
	}

	opts = append(opts, cltypes.WithSignMode(signMode))

	return opts, nil
}

// ParseGasSetting parses a string gas value. The value may either be 'auto',
// which indicates a transaction should be executed in simulate mode to
// automatically find a sufficient gas value, or a string integer. It returns an
// error if a string integer is provided which cannot be parsed.
func ParseGasSetting(gasStr string) (cltypes.GasSetting, error) {
	switch gasStr {
	case "":
		return cltypes.GasSetting{Simulate: false, Gas: DefaultGasLimit}, nil

	case GasFlagAuto:
		return cltypes.GasSetting{Simulate: true, Gas: 0}, nil

	default:
		gas, err := strconv.ParseUint(gasStr, 10, 64)
		if err != nil {
			return cltypes.GasSetting{}, fmt.Errorf("gas must be either integer or %s", GasFlagAuto)
		}

		return cltypes.GasSetting{Simulate: false, Gas: gas}, nil
	}
}
