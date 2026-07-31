// Package keys implements the `akt context keys` CLI commands for managing
// cryptographic keys within the context's keyring.
package keys

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

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

type keyDetails struct {
	Name    string `json:"name"    yaml:"name"`
	Type    string `json:"type"    yaml:"type"`
	Address string `json:"address" yaml:"address"`
	PubKey  string `json:"pubkey"  yaml:"pubkey"`
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

// Commands returns the "keys" command tree.
func Commands(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage keys in the current context's keyring",
		Long:  "Add, delete, list, show, export, and import cryptographic keys used for signing transactions.",
	}

	cmd.AddCommand(
		addCmd(getKeyring),
		deleteCmd(getKeyring),
		listCmd(getKeyring),
		showCmd(getKeyring),
		exportCmd(getKeyring),
		importCmd(getKeyring),
		renameCmd(getKeyring),
		mnemonicCmd(),
		parseCmd(),
	)

	return cmd
}

func addCmd(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
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
			multisigKeys, _ := cmd.Flags().GetString("multisig")
			if multisigKeys != "" {
				threshold, _ := cmd.Flags().GetInt("multisig-threshold")
				return addMultisig(kr, name, multisigKeys, threshold)
			}

			coinType, _ := cmd.Flags().GetUint32("coin-type")
			account, _ := cmd.Flags().GetUint32("account")
			index, _ := cmd.Flags().GetUint32("index")

			// Resolve HD path.
			path, _ := cmd.Flags().GetString("hd-path")
			if path == "" {
				path = hd.CreateHDPath(coinType, account, index).String()
			}

			// Ledger hardware wallet key.
			useLedger, _ := cmd.Flags().GetBool("ledger")
			if useLedger {
				record, err := kr.SaveLedgerKey(name, algo, "cosmos", coinType, account, index)
				if err != nil {
					return fmt.Errorf("add ledger key: %w", err)
				}

				addr, err := record.GetAddress()
				if err != nil {
					return fmt.Errorf("get address: %w", err)
				}

				fmt.Printf("- name: %s\n", record.Name)
				fmt.Printf("  address: %s\n", addr.String())
				fmt.Printf("  type: ledger\n")

				return nil
			}

			recoverKey, _ := cmd.Flags().GetBool("recover")
			source, _ := cmd.Flags().GetString("source")

			var mnemonic string

			if source != "" {
				data, err := os.ReadFile(source)
				if err != nil {
					return fmt.Errorf("read mnemonic source: %w", err)
				}

				mnemonic = strings.TrimSpace(string(data))
			} else if recoverKey {
				fmt.Print("Enter your mnemonic: ")

				reader := bufio.NewReader(os.Stdin)
				mnemonic, _ = reader.ReadString('\n')
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

			interactive, _ := cmd.Flags().GetBool("interactive")
			if interactive {
				fmt.Print("Enter BIP39 passphrase (leave empty for none): ")

				reader := bufio.NewReader(os.Stdin)
				bip39Passphrase, _ = reader.ReadString('\n')
				bip39Passphrase = strings.TrimSpace(bip39Passphrase)
			}

			record, err := kr.NewAccount(name, mnemonic, bip39Passphrase, path, algo)
			if err != nil {
				return fmt.Errorf("create key: %w", err)
			}

			addr, err := record.GetAddress()
			if err != nil {
				return fmt.Errorf("get address: %w", err)
			}

			keyType, _ := cmd.Flags().GetString("key-type")
			noBackup, _ := cmd.Flags().GetBool("no-backup")

			fmt.Printf("- name: %s\n", record.Name)
			fmt.Printf("  address: %s\n", addr.String())
			fmt.Printf("  type: %s\n", keyType)

			if !noBackup && !recoverKey && source == "" {
				fmt.Println("")
				fmt.Println("**Important** write this mnemonic phrase in a safe place.")
				fmt.Println("It is the only way to recover your account if you ever forget your password.")
				fmt.Println("")
				fmt.Println(mnemonic)
			}

			return nil
		},
	}

	cmd.Flags().Bool("recover", false, "Recover key from existing mnemonic")
	cmd.Flags().Bool("no-backup", false, "Don't print mnemonic after creation")
	cmd.Flags().BoolP("interactive", "i", false, "Interactive BIP39 passphrase prompt")
	cmd.Flags().Bool("ledger", false, "Use Ledger hardware wallet")
	cmd.Flags().Uint32("coin-type", 118, "BIP44 coin type")
	cmd.Flags().Uint32("account", 0, "BIP44 account number")
	cmd.Flags().Uint32("index", 0, "BIP44 address index")
	cmd.Flags().String("hd-path", "", "Manual HD path override")
	cmd.Flags().String("key-type", "secp256k1", "Signing algorithm")
	cmd.Flags().String("multisig", "", "Comma-separated list of key names for multisig")
	cmd.Flags().Int("multisig-threshold", 1, "K-of-N threshold for multisig")
	cmd.Flags().String("source", "", "File path to read mnemonic from")

	return cmd
}

func addMultisig(kr sdkkeyring.Keyring, name, keyNames string, threshold int) error {
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
	if err != nil {
		return fmt.Errorf("save multisig: %w", err)
	}

	addr, err := record.GetAddress()
	if err != nil {
		return fmt.Errorf("get address: %w", err)
	}

	fmt.Printf("- name: %s\n", record.Name)
	fmt.Printf("  address: %s\n", addr.String())
	fmt.Printf("  type: multi\n")
	fmt.Printf("  threshold: %d\n", threshold)
	fmt.Printf("  pubkeys: %d\n", len(pks))

	return nil
}

func deleteCmd(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
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

			if _, err := kr.Key(name); err != nil {
				return fmt.Errorf("key %q not found", name)
			}

			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("Delete key %q? [y/N]: ", name)

				var answer string
				_, _ = fmt.Scanln(&answer)

				if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			return kr.Delete(name)
		},
	}

	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

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
					return output.Print(output.FormatFromCmd(cmd), []struct{}{})
				}

				fmt.Println("No keys found. Add one with: akt context keys add <name>")

				return nil
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

			addressOnly, _ := cmd.Flags().GetBool("address")
			if addressOnly {
				if output.FormatFromCmd(cmd) == output.FormatTable {
					fmt.Fprintln(cmd.OutOrStdout(), addr.String())
					return nil
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

			fmt.Fprintf(cmd.OutOrStdout(), "Name:      %s\n", details.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Type:      %s\n", details.Type)
			fmt.Fprintf(cmd.OutOrStdout(), "Address:   %s\n", details.Address)
			fmt.Fprintf(cmd.OutOrStdout(), "PubKey:    %s\n", details.PubKey)

			return nil
		},
	}

	cmd.Flags().BoolP("address", "a", false, "Print only the bech32 address")

	return cmd
}

func exportCmd(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
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

			fmt.Print("Enter passphrase to encrypt the key: ")

			reader := bufio.NewReader(os.Stdin)
			passphrase, _ := reader.ReadString('\n')
			passphrase = strings.TrimSpace(passphrase)

			if passphrase == "" {
				return fmt.Errorf("passphrase is required")
			}

			armor, err := kr.ExportPrivKeyArmor(name, passphrase)
			if err != nil {
				return fmt.Errorf("export key %q: %w", name, err)
			}

			fmt.Println(armor)

			return nil
		},
	}
}

func importCmd(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
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

			fmt.Print("Enter passphrase to decrypt the key: ")

			reader := bufio.NewReader(os.Stdin)
			passphrase, _ := reader.ReadString('\n')
			passphrase = strings.TrimSpace(passphrase)

			if err := kr.ImportPrivKey(name, string(data), passphrase); err != nil {
				return fmt.Errorf("import key: %w", err)
			}

			fmt.Printf("Key %q imported successfully.\n", name)

			return nil
		},
	}
}

func renameCmd(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
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

			if err := kr.Rename(args[0], args[1]); err != nil {
				return fmt.Errorf("rename key: %w", err)
			}

			fmt.Printf("Key renamed from %q to %q.\n", args[0], args[1])

			return nil
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

			fmt.Println(mnemonic)

			return nil
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

			fmt.Fprintf(cmd.OutOrStdout(), "Format:  %s\n", result.Format)
			if result.HRP != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "HRP:     %s\n", result.HRP)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Hex:     %s\n", result.Hex)
			for _, prefix := range []string{"akash", "cosmos", "osmo"} {
				fmt.Fprintf(cmd.OutOrStdout(), "%-8s %s\n", prefix+":", result.Addresses[prefix])
			}

			return nil
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
