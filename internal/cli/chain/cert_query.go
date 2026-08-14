package cli

import (
	"fmt"
	"math/big"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	types "pkg.akt.dev/go/node/cert/v1"
	utiltls "pkg.akt.dev/go/util/tls"
)

func GetQueryCertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Certificate query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetQueryCertCertificatesCmd(),
	)

	return cmd
}

// GetQueryCertCertificatesCmd returns the command to list certificates.
// Accepts optional positional [owner] [state] filters (SPEC §3.8): the first
// argument is an owner address (or a bare state keyword), the second a state
// keyword (valid|revoked).
func GetQueryCertCertificatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list [owner] [state]",
		Short:             "Query for all certificates",
		Args:              cobra.MaximumNArgs(2),
		SilenceUsage:      true,
		PersistentPreRunE: QueryPersistentPreRunE,
		Example: `akt query cert list
akt query cert list akash1owner...
akt query cert list akash1owner... valid
akt query cert list revoked`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &types.QueryCertificatesRequest{
				Pagination: pageReq,
			}

			// Positional [owner] [state] filters with smart type detection
			// on the first component (SPEC §3.8.2): a bech32 address is the
			// owner; a bare state keyword is a state filter.
			if len(args) > 0 {
				if owner, aerr := sdk.AccAddressFromBech32(args[0]); aerr == nil {
					params.Filter.Owner = owner.String()
				} else if val, exists := types.State_value[args[0]]; exists && types.State(val) != types.CertificateStateInvalid {
					if len(args) > 1 {
						return fmt.Errorf("certificate filter: state keyword %q cannot be combined with a second argument %q", args[0], args[1])
					}
					params.Filter.State = args[0]
				} else {
					return fmt.Errorf("certificate filter: %q is not a valid owner address or state (valid|revoked)", args[0])
				}
			}

			if len(args) > 1 {
				if val, exists := types.State_value[args[1]]; !exists || types.State(val) == types.CertificateStateInvalid {
					return fmt.Errorf("certificate filter: %q is not a valid state (valid|revoked)", args[1])
				}

				params.Filter.State = args[1]
			}

			if params.Filter.Owner == "" {
				defaultOwner, err := resolveDefaultAccountAddress(cl.ClientContext())
				if err != nil {
					return err
				}
				if defaultOwner == "" {
					return requireOwnerScope("certificate filter")
				}
				params.Filter.Owner = defaultOwner
			}

			// FEEDBACK(2026-07): the --owner flag read is disabled for the
			// positional-only UX trial (the positional [owner] argument is
			// the only source). Restore by uncommenting if users ask for the
			// flag form back.
			// if value := cmd.Flag("owner").Value.String(); value != "" {
			// 	var owner sdk.Address
			// 	if owner, err = sdk.AccAddressFromBech32(value); err != nil {
			// 		return err
			// 	}
			//
			// 	params.Filter.Owner = owner.String()
			// }

			if value := cmd.Flag("serial").Value.String(); value != "" {
				if params.Filter.Owner == "" {
					return fmt.Errorf("--serial flag requires the positional [owner] argument to be set")
				}
				val, valid := new(big.Int).SetString(value, 10)
				if !valid {
					return utiltls.ErrInvalidSerialFlag
				}

				params.Filter.Serial = val.String()
			}

			// FEEDBACK(2026-07): the --state flag read is disabled for the
			// positional-only UX trial (the positional [state] argument is
			// the only source). Restore by uncommenting if users ask for the
			// flag form back.
			// if value := cmd.Flag("state").Value.String(); value != "" {
			// 	if val, exists := types.State_value[value]; !exists || types.State(val) == types.CertificateStateInvalid {
			// 		return fmt.Errorf("invalid value of --state flag. expected valid|revoked")
			// 	}
			//
			// 	params.Filter.State = value
			// }

			res, err := cl.Query().Certs().Certificates(cmd.Context(), params)
			if err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "certificates")

	cmd.Flags().String(flagdefs.FlagSerial, "", "filter certificates by serial number")
	// FEEDBACK(2026-07): --owner disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().String("owner", "", "filter certificates by owner")
	// FEEDBACK(2026-07): --state disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().String("state", "", "filter certificates by valid|revoked")

	return cmd
}
