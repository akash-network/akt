package flags

import (
	"github.com/spf13/pflag"
	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/bme/v1"
)

// AddBMELedgerFilterFlags add flags to filter for ledger record list
func AddBMELedgerFilterFlags(flags *pflag.FlagSet) {
	flags.String(flagdefs.FlagOwner, "", "source address to filter")
	flags.String(flagdefs.FlagDenom, "", "burn denomination to filter")
	flags.String(flagdefs.FlagToDenom, "", "mint denomination to filter")
	flags.String(flagdefs.FlagStatus, "", "record status to filter (ledger_record_status_pending, ledger_record_status_executed, ledger_record_status_canceled)")
}

// BMELedgerFiltersFromFlags returns LedgerRecordFilters with given flags and error if occurred
func BMELedgerFiltersFromFlags(flags *pflag.FlagSet) (types.LedgerRecordFilters, error) {
	var filters types.LedgerRecordFilters

	owner, err := flags.GetString(flagdefs.FlagOwner)
	if err != nil {
		return filters, err
	}

	if owner != "" {
		_, err = sdk.AccAddressFromBech32(owner)
		if err != nil {
			return filters, err
		}
	}

	filters.Source = owner

	if filters.Denom, err = flags.GetString(flagdefs.FlagDenom); err != nil {
		return filters, err
	}

	if filters.ToDenom, err = flags.GetString(flagdefs.FlagToDenom); err != nil {
		return filters, err
	}

	if filters.Status, err = flags.GetString(flagdefs.FlagStatus); err != nil {
		return filters, err
	}

	return filters, nil
}
