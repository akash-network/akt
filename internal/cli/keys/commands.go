// Package keys implements the `akt context keys` CLI commands for managing
// cryptographic keys within the context's keyring.
package keys

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	"pkg.akt.dev/akt/internal/output"
)

// Action identifiers recorded in the action log. They are namespaced under
// "keys." so `akt context log --type context` keeps key management readable
// and distinct from the bare create/delete/rename context actions
// (SPEC §2.2.2).
const (
	actionKeysAdd     = "keys.add"
	actionKeysRecover = "keys.recover"
	actionKeysDelete  = "keys.delete"
	actionKeysRename  = "keys.rename"
	actionKeysImport  = "keys.import"
	actionKeysExport  = "keys.export"
)

// Key types as recorded in the action log when the keyring cannot report one
// itself, i.e. when the mutation failed.
const (
	keyTypeLocal  = "local"
	keyTypeLedger = "ledger"
	keyTypeMulti  = "multi"
)

// Recorder records one key-management action in the action log of the context
// the command is running against (SPEC §2.2.2, §5.6). A nil actionErr records
// a successful mutation, a non-nil one records the failed attempt.
//
// The keys commands cannot open that log themselves: it belongs to a context,
// and internal/cli/context — which owns the single write path for context
// entries — imports this package, so the dependency can only run the other
// way. The recorder is therefore injected. A nil Recorder disables recording.
//
// Secret material (mnemonics, BIP39 passphrases, armor passphrases, armored
// keys) is never passed to a Recorder.
type Recorder func(cmd *cobra.Command, action string, actionErr error, details map[string]string)

func (r Recorder) record(cmd *cobra.Command, action string, actionErr error, details map[string]string) {
	if r == nil {
		return
	}

	r(cmd, action, actionErr, details)
}

// keyMutationDetails builds the parameters recorded for a key mutation. When
// the keyring returned a record, it supplies the authoritative key type and
// the full address (never truncated); a failed mutation records only the name
// and the requested type.
func keyMutationDetails(name, keyType string, rec *sdkkeyring.Record) map[string]string {
	details := map[string]string{"name": name, "type": keyType}

	if rec != nil {
		details["type"] = rec.GetType().String()

		if addr, err := rec.GetAddress(); err == nil {
			details["address"] = addr.String()
		}
	}

	return details
}

type keyDetails struct {
	Name    string `json:"name"    yaml:"name"`
	Type    string `json:"type"    yaml:"type"`
	Address string `json:"address" yaml:"address"`
	PubKey  string `json:"pubkey"  yaml:"pubkey"`
}

type keyAddResult struct {
	Name      string `json:"name"                yaml:"name"`
	Address   string `json:"address"             yaml:"address"`
	Type      string `json:"type"                yaml:"type"`
	Mnemonic  string `json:"mnemonic,omitempty"  yaml:"mnemonic,omitempty"`
	Threshold int    `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	PubKeys   int    `json:"pubkeys,omitempty"   yaml:"pubkeys,omitempty"`
}

type addressParseResult struct {
	Format    string            `json:"format"          yaml:"format"`
	HRP       string            `json:"hrp,omitempty"   yaml:"hrp,omitempty"`
	Hex       string            `json:"hex"             yaml:"hex"`
	Addresses map[string]string `json:"addresses"       yaml:"addresses"`
}

type quotedMachineScalar string

func (value quotedMachineScalar) MarshalYAML() (any, error) {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: string(value),
		Style: yaml.DoubleQuotedStyle,
	}, nil
}

// Commands returns the "keys" command tree. recorder receives every keyring
// mutation for the action log (SPEC §2.2.2); it may be nil.
func Commands(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage keys in the current context's keyring",
		Long:  "Add, delete, list, show, export, and import cryptographic keys used for signing transactions.",
	}

	cmd.AddCommand(
		addCmd(getKeyring, recorder),
		deleteCmd(getKeyring, recorder),
		listCmd(getKeyring),
		showCmd(getKeyring),
		exportCmd(getKeyring, recorder),
		importCmd(getKeyring, recorder),
		renameCmd(getKeyring, recorder),
		mnemonicCmd(),
		parseCmd(),
	)

	return cmd
}

func addCmd(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new key or recover an existing one",
		Args:  cobra.ExactArgs(1),
		Example: `  # Generate a new key
  akt context keys add alice

  # Recover from mnemonic
  akt context keys add alice --recover

  # Add a Ledger key
  akt context keys add alice --ledger`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}

			name := args[0]

			// Check if key already exists.
			if _, err := kr.Key(name); err == nil {
				return fmt.Errorf("key %q already exists; delete it first or use a different name", name)
			}

			algo := aktkeyring.DefaultAlgo()

			// Multisig key.
			multisigKeys, _ := cmd.Flags().GetString(flagdefs.FlagMultisig)
			if multisigKeys != "" {
				threshold, _ := cmd.Flags().GetInt(flagdefs.FlagMultisigThreshold)
				return addMultisig(cmd, kr, recorder, name, multisigKeys, threshold)
			}

			coinType, _ := cmd.Flags().GetUint32(flagdefs.FlagCoinType)
			account, _ := cmd.Flags().GetUint32(flagdefs.FlagAccount)
			index, _ := cmd.Flags().GetUint32(flagdefs.FlagIndex)

			// Resolve HD path.
			path, _ := cmd.Flags().GetString(flagdefs.FlagHDPath)
			if path == "" {
				path = hd.CreateHDPath(coinType, account, index).String()
			}

			// Ledger hardware wallet key.
			useLedger, _ := cmd.Flags().GetBool(flagdefs.FlagUseLedger)
			if useLedger {
				bech32PrefixAccAddr := sdk.GetConfig().GetBech32AccountAddrPrefix()
				record, err := kr.SaveLedgerKey(name, algo, bech32PrefixAccAddr, coinType, account, index)
				recorder.record(cmd, actionKeysAdd, err, keyMutationDetails(name, keyTypeLedger, record))

				if err != nil {
					return fmt.Errorf("add ledger key: %w", err)
				}

				addr, err := record.GetAddress()
				if err != nil {
					return fmt.Errorf("get address: %w", err)
				}

				return printAddedKey(cmd, keyAddResult{
					Name:    record.Name,
					Address: addr.String(),
					Type:    "ledger",
				})
			}

			recoverKey, _ := cmd.Flags().GetBool(flagdefs.FlagRecover)
			source, _ := cmd.Flags().GetString(flagdefs.FlagMnemonicSource)

			var mnemonic string

			if source != "" {
				data, err := os.ReadFile(source)
				if err != nil {
					return fmt.Errorf("read mnemonic source: %w", err)
				}

				mnemonic = strings.TrimSpace(string(data))
			} else if recoverKey {
				if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Enter your mnemonic: "); err != nil {
					return err
				}

				reader := bufio.NewReader(cmd.InOrStdin())
				mnemonic, err = reader.ReadString('\n')
				if err != nil && strings.TrimSpace(mnemonic) == "" {
					return fmt.Errorf("read mnemonic: %w", err)
				}
				mnemonic = strings.TrimSpace(mnemonic)
			} else {
				// Generate new mnemonic.
				var err error
				mnemonic, err = generateMnemonic()
				if err != nil {
					return err
				}
			}

			var bip39Passphrase string

			interactive, _ := cmd.Flags().GetBool(flagdefs.FlagInteractive)
			if interactive {
				if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Enter BIP39 passphrase (leave empty for none): "); err != nil {
					return err
				}

				reader := bufio.NewReader(cmd.InOrStdin())
				bip39Passphrase, err = reader.ReadString('\n')
				if err != nil && bip39Passphrase == "" {
					return fmt.Errorf("read BIP39 passphrase: %w", err)
				}
				bip39Passphrase = strings.TrimSpace(bip39Passphrase)
			}

			record, err := kr.NewAccount(name, mnemonic, bip39Passphrase, path, algo)

			// Recovering an existing key and generating a new one are
			// different events for an audit reader; neither records the
			// mnemonic or the BIP39 passphrase.
			addAction := actionKeysAdd
			if recoverKey || source != "" {
				addAction = actionKeysRecover
			}

			recorder.record(cmd, addAction, err, keyMutationDetails(name, keyTypeLocal, record))

			if err != nil {
				return fmt.Errorf("create key: %w", err)
			}

			addr, err := record.GetAddress()
			if err != nil {
				return fmt.Errorf("get address: %w", err)
			}

			keyType, _ := cmd.Flags().GetString(flagdefs.FlagKeyType)
			noBackup, _ := cmd.Flags().GetBool(flagdefs.FlagNoBackup)

			result := keyAddResult{
				Name:    record.Name,
				Address: addr.String(),
				Type:    keyType,
			}
			if !noBackup && !recoverKey && source == "" {
				result.Mnemonic = mnemonic
			}

			return printAddedKey(cmd, result)
		},
	}

	cmd.Flags().Bool(flagdefs.FlagRecover, false, "Recover key from existing mnemonic")
	cmd.Flags().Bool(flagdefs.FlagNoBackup, false, "Don't print mnemonic after creation")
	cmd.Flags().BoolP(flagdefs.FlagInteractive, "i", false, "Interactive BIP39 passphrase prompt")
	cmd.Flags().Bool(flagdefs.FlagUseLedger, false, "Use Ledger hardware wallet")
	cmd.Flags().Uint32(flagdefs.FlagCoinType, 118, "BIP44 coin type")
	cmd.Flags().Uint32(flagdefs.FlagAccount, 0, "BIP44 account number")
	cmd.Flags().Uint32(flagdefs.FlagIndex, 0, "BIP44 address index")
	cmd.Flags().String(flagdefs.FlagHDPath, "", "Manual HD path override")
	cmd.Flags().String(flagdefs.FlagKeyType, "secp256k1", "Signing algorithm")
	cmd.Flags().String(flagdefs.FlagMultisig, "", "Comma-separated list of key names for multisig")
	cmd.Flags().Int(flagdefs.FlagMultisigThreshold, 1, "K-of-N threshold for multisig")
	cmd.Flags().String(flagdefs.FlagMnemonicSource, "", "File path to read mnemonic from")

	return cmd
}

func addMultisig(cmd *cobra.Command, kr sdkkeyring.Keyring, recorder Recorder, name, keyNames string, threshold int) error {
	names := strings.Split(keyNames, ",")
	pks := make([]cryptotypes.PubKey, 0, len(names))

	for _, n := range names {
		n = strings.TrimSpace(n)
		rec, err := kr.Key(n)
		if err != nil {
			return fmt.Errorf("key %q not found: %w", n, err)
		}

		pk, err := rec.GetPubKey()
		if err != nil {
			return fmt.Errorf("get pubkey for %q: %w", n, err)
		}

		pks = append(pks, pk)
	}

	pk := multisig.NewLegacyAminoPubKey(threshold, pks)

	record, err := kr.SaveMultisig(name, pk)

	multisigDetails := keyMutationDetails(name, keyTypeMulti, record)
	multisigDetails["threshold"] = strconv.Itoa(threshold)
	multisigDetails["pubkeys"] = strconv.Itoa(len(pks))
	recorder.record(cmd, actionKeysAdd, err, multisigDetails)

	if err != nil {
		return fmt.Errorf("save multisig: %w", err)
	}

	addr, err := record.GetAddress()
	if err != nil {
		return fmt.Errorf("get address: %w", err)
	}

	return printAddedKey(cmd, keyAddResult{
		Name:      record.Name,
		Address:   addr.String(),
		Type:      "multi",
		Threshold: threshold,
		PubKeys:   len(pks),
	})
}

func printAddedKey(cmd *cobra.Command, result keyAddResult) error {
	format := output.FormatFromCmd(cmd)
	if format != output.FormatTable {
		return output.Fprint(cmd.OutOrStdout(), format, result)
	}

	w := output.NewCheckedWriter(cmd.OutOrStdout())
	_, err := fmt.Fprintf(w, "- name: %s\n", result.Name)
	_, _ = fmt.Fprintf(w, "  address: %s\n", result.Address)
	_, _ = fmt.Fprintf(w, "  type: %s\n", result.Type)
	if result.Threshold > 0 {
		_, _ = fmt.Fprintf(w, "  threshold: %d\n", result.Threshold)
		_, _ = fmt.Fprintf(w, "  pubkeys: %d\n", result.PubKeys)
	}
	if result.Mnemonic != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "**Important** write this mnemonic phrase in a safe place.")
		_, _ = fmt.Fprintln(w, "It is the only way to recover your account if you ever forget your password.")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, result.Mnemonic)
	}

	return w.Complete(err)
}

func deleteCmd(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <name>",
		Short:   "Delete a key from the keyring",
		Args:    cobra.ExactArgs(1),
		Example: `  akt context keys delete alice`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}

			name := args[0]

			// Read the record before deleting it: afterwards its type and
			// address are gone, and they are what identifies the deleted key
			// in the log.
			record, err := kr.Key(name)
			if err != nil {
				return fmt.Errorf("key %q not found", name)
			}

			yes, _ := cmd.Flags().GetBool(flagdefs.FlagSkipConfirmation)
			if !yes {
				checked := output.NewCheckedWriter(cmd.ErrOrStderr())
				if _, err := fmt.Fprintf(checked, "Delete key %q? [y/N]: ", name); err != nil {
					return checked.Complete(err)
				}

				line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if err != nil && strings.TrimSpace(line) == "" {
					return fmt.Errorf("read delete confirmation: %w", err)
				}
				answer := strings.TrimSpace(line)

				// A declined confirmation changes nothing, so it is not an
				// action to record.
				if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
					_, err := fmt.Fprintln(checked, "Cancelled.")
					return checked.Complete(err)
				}
			}

			deleteErr := kr.Delete(name)
			recorder.record(cmd, actionKeysDelete, deleteErr, keyMutationDetails(name, keyTypeLocal, record))

			return deleteErr
		},
	}

	cmd.Flags().BoolP(flagdefs.FlagSkipConfirmation, "y", false, "Skip confirmation")

	return cmd
}

func listCmd(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List all keys in the current keyring",
		Args:    cobra.NoArgs,
		Example: `  akt context keys list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}

			records, err := kr.List()
			if err != nil {
				return err
			}

			// See the action-log listing: the hint is for humans, and a
			// structured caller needs an empty collection instead.
			if len(records) == 0 {
				if output.FormatFromCmd(cmd) != output.FormatTable {
					return output.Fprint(cmd.OutOrStdout(), output.FormatFromCmd(cmd), []struct{}{})
				}

				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No keys found. Add one with: akt context keys add <name>")
				return err
			}

			type keyRow struct {
				Name    string `json:"name"    yaml:"name"`
				Type    string `json:"type"    yaml:"type"`
				Address string `json:"address" yaml:"address"`
				PubKey  string `json:"pubkey"  yaml:"pubkey"`
			}

			data := make([]keyRow, 0, len(records))
			columns := []output.Column{
				{Header: "NAME"},
				{Header: "TYPE"},
				{Header: "ADDRESS"},
				{Header: "PUBKEY"},
			}

			rows := make([][]string, 0, len(records))
			for _, rec := range records {
				addr, err := rec.GetAddress()
				if err != nil {
					continue
				}

				pk, err := rec.GetPubKey()
				if err != nil {
					continue
				}

				pubkeyHex := hex.EncodeToString(pk.Bytes())
				rows = append(rows, []string{
					rec.Name,
					rec.GetType().String(),
					addr.String(),
					pubkeyHex[:16] + "...",
				})
				data = append(data, keyRow{
					Name:    rec.Name,
					Type:    rec.GetType().String(),
					Address: addr.String(),
					PubKey:  pubkeyHex,
				})
			}

			return output.PrintData(cmd, columns, rows, data)
		},
	}
}

func showCmd(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name|address>",
		Short: "Show key details",
		Args:  cobra.ExactArgs(1),
		Example: `  # Show full key details
  akt context keys show alice

  # Print only the bech32 address
  akt context keys show alice -a`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}

			rec, err := fetchKey(kr, args[0])
			if err != nil {
				return err
			}

			addr, err := rec.GetAddress()
			if err != nil {
				return fmt.Errorf("get address: %w", err)
			}

			addressOnly, _ := cmd.Flags().GetBool(flagdefs.FlagAddress)
			if addressOnly {
				if output.FormatFromCmd(cmd) == output.FormatTable {
					_, err := fmt.Fprintln(cmd.OutOrStdout(), addr.String())
					return err
				}

				return output.Fprint(
					cmd.OutOrStdout(),
					output.FormatFromCmd(cmd),
					quotedMachineScalar(addr.String()),
				)
			}

			pk, err := rec.GetPubKey()
			if err != nil {
				return fmt.Errorf("get pubkey: %w", err)
			}

			details := keyDetails{
				Name:    rec.Name,
				Type:    rec.GetType().String(),
				Address: addr.String(),
				PubKey:  hex.EncodeToString(pk.Bytes()),
			}
			if output.FormatFromCmd(cmd) != output.FormatTable {
				return output.Fprint(cmd.OutOrStdout(), output.FormatFromCmd(cmd), details)
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

	return cmd
}

func exportCmd(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	return &cobra.Command{
		Use:     "export <name>",
		Short:   "Export a private key (encrypted armor)",
		Args:    cobra.ExactArgs(1),
		Example: `  akt context keys export alice`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}

			name := args[0]

			if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Enter passphrase to encrypt the key: "); err != nil {
				return err
			}

			reader := bufio.NewReader(cmd.InOrStdin())
			passphrase, readErr := reader.ReadString('\n')
			if readErr != nil && strings.TrimSpace(passphrase) == "" {
				return fmt.Errorf("read export passphrase: %w", readErr)
			}
			passphrase = strings.TrimSpace(passphrase)

			if passphrase == "" {
				return fmt.Errorf("passphrase is required")
			}

			armor, err := kr.ExportPrivKeyArmor(name, passphrase)

			// Export changes no state, but it is the one command that moves
			// private key material out of the keyring, so it is recorded as a
			// security event (SPEC §2.2.2). Only the key name is recorded --
			// never the passphrase or the armor.
			recorder.record(cmd, actionKeysExport, err, map[string]string{"name": name})

			if err != nil {
				return fmt.Errorf("export key %q: %w", name, err)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), armor)
			return err
		},
	}
}

func importCmd(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	return &cobra.Command{
		Use:     "import <name> <keyfile>",
		Short:   "Import a private key from encrypted armor file",
		Args:    cobra.ExactArgs(2),
		Example: `  akt context keys import alice ./alice-key.armor`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}

			name := args[0]
			file := args[1]

			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read key file: %w", err)
			}

			if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Enter passphrase to decrypt the key: "); err != nil {
				return err
			}

			reader := bufio.NewReader(cmd.InOrStdin())
			passphrase, readErr := reader.ReadString('\n')
			if readErr != nil && strings.TrimSpace(passphrase) == "" {
				return fmt.Errorf("read import passphrase: %w", readErr)
			}
			passphrase = strings.TrimSpace(passphrase)

			importErr := kr.ImportPrivKey(name, string(data), passphrase)

			// The imported record carries the address the log needs; neither
			// the armor nor the passphrase is ever recorded.
			var record *sdkkeyring.Record
			if importErr == nil {
				record, _ = kr.Key(name)
			}

			recorder.record(cmd, actionKeysImport, importErr, keyMutationDetails(name, keyTypeLocal, record))

			if importErr != nil {
				return fmt.Errorf("import key: %w", importErr)
			}

			_, err = fmt.Fprintf(cmd.ErrOrStderr(), "Key %q imported successfully.\n", name)
			return err
		},
	}
}

func renameCmd(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	return &cobra.Command{
		Use:     "rename <old> <new>",
		Short:   "Rename a key",
		Args:    cobra.ExactArgs(2),
		Example: `  akt context keys rename alice alice-main`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}

			renameErr := kr.Rename(args[0], args[1])
			recorder.record(cmd, actionKeysRename, renameErr, map[string]string{"from": args[0], "to": args[1]})

			if renameErr != nil {
				return fmt.Errorf("rename key: %w", renameErr)
			}

			_, err = fmt.Fprintf(cmd.ErrOrStderr(), "Key renamed from %q to %q.\n", args[0], args[1])
			return err
		},
	}
}

func mnemonicCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "mnemonic",
		Short:   "Generate a new BIP39 mnemonic",
		Args:    cobra.NoArgs,
		Example: `  akt context keys mnemonic`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mnemonic, err := generateMnemonic()
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), mnemonic)
			return err
		},
	}
}

func parseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "parse <hex-or-bech32>",
		Short: "Parse address between hex and bech32 formats",
		Args:  cobra.ExactArgs(1),
		Example: `  # Parse a bech32 address
  akt context keys parse akash1abc...

  # Parse a hex address
  akt context keys parse 0ABC1DEF...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parseAddress(args[0])
			if err != nil {
				return err
			}
			if output.FormatFromCmd(cmd) != output.FormatTable {
				return output.Fprint(cmd.OutOrStdout(), output.FormatFromCmd(cmd), result)
			}

			checked := output.NewCheckedWriter(cmd.OutOrStdout())
			_, writeErr := fmt.Fprintf(checked, "Format:  %s\n", result.Format)
			if result.HRP != "" {
				_, _ = fmt.Fprintf(checked, "HRP:     %s\n", result.HRP)
			}
			_, _ = fmt.Fprintf(checked, "Hex:     %s\n", result.Hex)
			for _, prefix := range []string{"akash", "cosmos", "osmo"} {
				_, _ = fmt.Fprintf(checked, "%-8s %s\n", prefix+":", result.Addresses[prefix])
			}

			return checked.Complete(writeErr)
		},
	}
}

func parseAddress(input string) (addressParseResult, error) {
	if hrp, data, err := bech32.DecodeAndConvert(input); err == nil {
		return addressParseResultFromBytes("bech32", hrp, data)
	}

	hexInput := strings.TrimPrefix(strings.TrimPrefix(input, "0x"), "0X")
	if hexInput == "" {
		return addressParseResult{}, fmt.Errorf("cannot parse %q as bech32 or hex", input)
	}
	data, err := hex.DecodeString(hexInput)
	if err != nil || len(data) == 0 {
		return addressParseResult{}, fmt.Errorf("cannot parse %q as bech32 or hex", input)
	}

	return addressParseResultFromBytes("hex", "", data)
}

func addressParseResultFromBytes(format, hrp string, data []byte) (addressParseResult, error) {
	result := addressParseResult{
		Format:    format,
		HRP:       hrp,
		Hex:       strings.ToUpper(hex.EncodeToString(data)),
		Addresses: make(map[string]string, 3),
	}
	for _, prefix := range []string{"akash", "cosmos", "osmo"} {
		encoded, err := bech32.ConvertAndEncode(prefix, data)
		if err != nil {
			return addressParseResult{}, fmt.Errorf("encode %s address: %w", prefix, err)
		}
		result.Addresses[prefix] = encoded
	}

	return result, nil
}

// fetchKey looks up a key by name or bech32 address.
func fetchKey(kr sdkkeyring.Keyring, ref string) (*sdkkeyring.Record, error) {
	// Try by name first.
	rec, err := kr.Key(ref)
	if err == nil {
		return rec, nil
	}

	// Try as bech32 address.
	addr, err := sdk.AccAddressFromBech32(ref)
	if err != nil {
		return nil, fmt.Errorf("key %q not found (also not a valid bech32 address)", ref)
	}

	rec, err = kr.KeyByAddress(addr)
	if err != nil {
		return nil, fmt.Errorf("no key found for address %s", ref)
	}

	return rec, nil
}

// generateMnemonic creates a new BIP39 mnemonic phrase.
func generateMnemonic() (string, error) {
	entropySeed, err := bip39Entropy()
	if err != nil {
		return "", err
	}

	return bip39Mnemonic(entropySeed)
}
