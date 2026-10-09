package keys

import (
	"encoding/hex"
	"fmt"
	"io"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/mdp/qrterminal/v3"
	"github.com/spf13/cobra"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	"pkg.akt.dev/akt/internal/output"
)

func showCmd(
	getKeyring func() (sdkkeyring.Keyring, error),
	showAddress func(hd.BIP44Params, cryptotypes.PubKey, string) error,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name|address> [name|address...]",
		Short: "Show key details",
		Long: "Show key details by name or account address. Multiple keys form an ephemeral " +
			"multisig named multi without saving it to the keyring.",
		Args: cobra.MinimumNArgs(1),
		Example: `  # Show full key details
  akt context keys show alice

  # Print only the validator operator address
  akt context keys show alice -a --bech val

  # Print only the Protobuf JSON public key
  akt context keys show alice -p

  # Verify and display an account address on its Ledger
  akt context keys show alice -a -d

  # Preview a 2-of-2 multisig address
  akt context keys show alice bob -a --multisig-threshold 2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			addressOnly, _ := cmd.Flags().GetBool(flagdefs.FlagAddress)
			pubkeyOnly, _ := cmd.Flags().GetBool(flagdefs.FlagPubKey)
			device, _ := cmd.Flags().GetBool(flagdefs.FlagDevice)
			qrCode, _ := cmd.Flags().GetBool(flagdefs.FlagQRCode)
			prefix, _ := cmd.Flags().GetString(flagdefs.FlagBechPrefix)
			threshold, _ := cmd.Flags().GetInt(flagdefs.FlagMultisigThreshold)
			format := output.FormatFromCmd(cmd)

			if addressOnly && pubkeyOnly {
				return fmt.Errorf("cannot use both --address and --pubkey")
			}
			if device && (pubkeyOnly || prefix != sdk.PrefixAccount) {
				return fmt.Errorf("--device requires --bech acc and cannot be used with --pubkey")
			}
			if addressOnly && qrCode && format != output.FormatTable {
				return fmt.Errorf("--qrcode requires --output pretty when used with --address")
			}
			if len(args) > 1 && (threshold < 1 || threshold > len(args)) {
				return fmt.Errorf("--multisig-threshold must be between 1 and %d", len(args))
			}

			kr, err := getKeyring()
			if err != nil {
				return err
			}
			rec, err := showKeyRecord(kr, args, threshold, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			pk, err := rec.GetPubKey()
			if err != nil {
				return fmt.Errorf("get pubkey: %w", err)
			}

			address := sdk.AccAddress(pk.Address()).String()
			switch prefix {
			case sdk.PrefixValidator:
				address = sdk.ValAddress(pk.Address()).String()
			case sdk.PrefixConsensus:
				address = sdk.ConsAddress(pk.Address()).String()
			}

			if device {
				item := rec.GetLedger()
				if item == nil {
					return fmt.Errorf("--device requires a Ledger key; %q is not stored on a device", rec.Name)
				}
				if item.Path == nil {
					return fmt.Errorf("ledger key %q has no BIP44 path", rec.Name)
				}
				if err := showAddress(*item.Path, pk, sdk.GetConfig().GetBech32AccountAddrPrefix()); err != nil {
					return fmt.Errorf("show address on Ledger: %w", err)
				}
			}

			if addressOnly || pubkeyOnly {
				value := address
				if pubkeyOnly {
					encoded, err := codec.ProtoMarshalJSON(rec.PubKey, nil)
					if err != nil {
						return fmt.Errorf("encode pubkey: %w", err)
					}
					value = string(encoded)
				}
				if format != output.FormatTable {
					return output.Fprint(cmd.OutOrStdout(), format, quotedMachineScalar(value))
				}
				checked := output.NewCheckedWriter(cmd.OutOrStdout())
				if addressOnly && qrCode {
					qrterminal.GenerateHalfBlock(value, qrterminal.H, checked)
				}
				_, writeErr := fmt.Fprintln(checked, value)
				return checked.Complete(writeErr)
			}

			details := keyDetails{
				Name:    rec.Name,
				Type:    rec.GetType().String(),
				Address: address,
				PubKey:  hex.EncodeToString(pk.Bytes()),
			}
			if format != output.FormatTable {
				return output.Fprint(cmd.OutOrStdout(), format, details)
			}
			checked := output.NewCheckedWriter(cmd.OutOrStdout())
			_, writeErr := fmt.Fprintf(checked, "Name:      %s\n", details.Name)
			_, _ = fmt.Fprintf(checked, "Type:      %s\n", details.Type)
			_, _ = fmt.Fprintf(checked, "Address:   %s\n", details.Address)
			_, _ = fmt.Fprintf(checked, "PubKey:    %s\n", details.PubKey)
			return checked.Complete(writeErr)
		},
	}

	cmd.Flags().BoolP(flagdefs.FlagAddress, "a", false, "Print only the bech32 address")
	cmd.Flags().Var(output.NewEnumFlag(sdk.PrefixAccount, sdk.PrefixAccount, sdk.PrefixValidator, sdk.PrefixConsensus),
		flagdefs.FlagBechPrefix, "Address encoding: acc, val, cons")
	cmd.Flags().BoolP(flagdefs.FlagPubKey, "p", false, "Print only the Protobuf JSON public key")
	cmd.Flags().BoolP(flagdefs.FlagDevice, "d", false, "Verify and display the account address on its Ledger device")
	cmd.Flags().Int(flagdefs.FlagMultisigThreshold, 1, "Required signatures for an ephemeral multisig")
	cmd.Flags().Bool(flagdefs.FlagQRCode, false, "Display an address QR code with --address and --output pretty")
	return cmd
}

func showKeyRecord(kr sdkkeyring.Keyring, refs []string, threshold int, stderr io.Writer) (*sdkkeyring.Record, error) {
	if len(refs) == 1 {
		return fetchKey(kr, refs[0])
	}

	pubkeys := make([]cryptotypes.PubKey, len(refs))
	seen := make(map[string]bool, len(refs))
	for i, ref := range refs {
		if seen[ref] {
			if _, err := fmt.Fprintf(stderr, "WARNING: duplicate keys found: %s.\n\n", ref); err != nil {
				return nil, err
			}
		}
		seen[ref] = true
		rec, err := fetchKey(kr, ref)
		if err != nil {
			return nil, err
		}
		pubkeys[i], err = rec.GetPubKey()
		if err != nil {
			return nil, fmt.Errorf("get pubkey for %q: %w", ref, err)
		}
	}
	return sdkkeyring.NewMultiRecord("multi", multisig.NewLegacyAminoPubKey(threshold, pubkeys))
}
