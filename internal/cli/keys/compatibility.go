package keys

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	"pkg.akt.dev/akt/internal/output"
)

func readKeyInput(cmd *cobra.Command, reader *bufio.Reader, prompt, field string) (string, error) {
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), prompt); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", fmt.Errorf("read %s: %w", field, err)
	}
	return strings.TrimSpace(line), nil
}

func confirmKeyAction(cmd *cobra.Command, reader *bufio.Reader, prompt string) (bool, error) {
	yes, _ := cmd.Flags().GetBool(flagdefs.FlagSkipConfirmation)
	if yes {
		return true, nil
	}
	answer, err := readKeyInput(cmd, reader, prompt+" [y/N]: ", "confirmation")
	if err != nil {
		return false, err
	}
	if strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
		return true, nil
	}
	_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Cancelled.")
	return false, err
}

func readPrivateKey(cmd *cobra.Command) (string, error) {
	const prompt = "Enter hex private key: "
	if file, ok := cmd.InOrStdin().(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		if _, err := fmt.Fprint(cmd.ErrOrStderr(), prompt); err != nil {
			return "", err
		}
		key, err := term.ReadPassword(int(file.Fd()))
		if err != nil {
			return "", fmt.Errorf("read private key: %w", err)
		}
		if _, err := fmt.Fprintln(cmd.ErrOrStderr()); err != nil {
			return "", err
		}
		return strings.TrimSpace(string(key)), nil
	}
	return readKeyInput(cmd, bufio.NewReader(cmd.InOrStdin()), prompt, "private key")
}

func printKeyStrings(cmd *cobra.Command, values []string) error {
	format := output.FormatFromCmd(cmd)
	if format != output.FormatTable {
		return output.Fprint(cmd.OutOrStdout(), format, values)
	}
	for _, value := range values {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), value); err != nil {
			return err
		}
	}
	return nil
}

func listKeyTypesCmd(getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
	return &cobra.Command{
		Use:     "list-key-types",
		Short:   "List supported key signing algorithms",
		Args:    cobra.NoArgs,
		Example: `  akt context keys list-key-types`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}
			algos, _ := kr.SupportedAlgorithms()
			names := make([]string, 0, len(algos))
			for _, algo := range algos {
				names = append(names, string(algo.Name()))
			}
			return printKeyStrings(cmd, names)
		},
	}
}

func importHexCmd(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "import-hex <name> [hex]",
		Short:   "Import a hex-encoded private key",
		Args:    cobra.RangeArgs(1, 2),
		Example: `  akt context keys import-hex alice --key-type secp256k1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("key name must not be blank")
			}
			kr, err := getKeyring()
			if err != nil {
				return err
			}
			if _, err := kr.Key(name); err == nil {
				return fmt.Errorf("key %q already exists; delete it first or use a different name", name)
			}
			keyType, _ := cmd.Flags().GetString(flagdefs.FlagKeyType)
			algos, _ := kr.SupportedAlgorithms()
			if _, err := sdkkeyring.NewSigningAlgoFromString(keyType, algos); err != nil {
				return err
			}
			var key string
			if len(args) == 2 {
				key = args[1]
			} else {
				key, err = readPrivateKey(cmd)
				if err != nil {
					return err
				}
			}
			decoded, err := hex.DecodeString(strings.TrimPrefix(key, "0x"))
			if err != nil || len(decoded) != 32 {
				return fmt.Errorf("private key must encode exactly 32 bytes of hex")
			}
			if keyType == "secp256k1" {
				var scalar secp.ModNScalar
				if overflow := scalar.SetByteSlice(decoded); overflow || scalar.IsZero() {
					return fmt.Errorf("invalid secp256k1 private key")
				}
			}
			importErr := kr.ImportPrivKeyHex(name, hex.EncodeToString(decoded), keyType)
			var rec *sdkkeyring.Record
			if importErr == nil {
				rec, _ = kr.Key(name)
			}
			recorder.record(cmd, actionKeysImport, importErr, keyMutationDetails(name, keyTypeLocal, rec))
			if importErr != nil {
				return fmt.Errorf("import key: %w", importErr)
			}
			_, err = fmt.Fprintf(cmd.ErrOrStderr(), "Key %q imported successfully.\n", name)
			return err
		},
	}
	cmd.Flags().String(flagdefs.FlagKeyType, "secp256k1", "Private key signing algorithm")
	return cmd
}

func exportRawKey(cmd *cobra.Command, kr sdkkeyring.Keyring, recorder Recorder, name string) error {
	confirmed, err := confirmKeyAction(cmd, bufio.NewReader(cmd.InOrStdin()), "Export unencrypted private key?")
	if err != nil || !confirmed {
		return err
	}
	exporter, ok := kr.(interface {
		ExportPrivateKeyObject(string) (cryptotypes.PrivKey, error)
	})
	if !ok {
		return fmt.Errorf("selected keyring does not support raw private-key export")
	}
	key, err := exporter.ExportPrivateKeyObject(name)
	recorder.record(cmd, actionKeysExport, err, map[string]string{"name": name})
	if err != nil {
		return fmt.Errorf("export key %q: %w", name, err)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), hex.EncodeToString(key.Bytes()))
	return err
}

func migrateCmd(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	return &cobra.Command{
		Use:     "migrate",
		Short:   "Migrate the selected keyring's Amino records to Protobuf",
		Args:    cobra.NoArgs,
		Example: `  akt context keys migrate`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kr, err := getKeyring()
			if err != nil {
				return err
			}
			records, err := kr.MigrateAll()
			recorder.record(cmd, actionKeysMigrate, err, map[string]string{"count": strconv.Itoa(len(records))})
			if err != nil {
				return fmt.Errorf("migrate keys: %w", err)
			}
			_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Keys migration completed.")
			return err
		},
	}
}

func mnemonicCmd(generate func() (string, error), encode func([]byte) (string, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mnemonic",
		Short:   "Generate a new BIP39 mnemonic",
		Args:    cobra.NoArgs,
		Example: `  akt context keys mnemonic`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			unsafeEntropy, _ := cmd.Flags().GetBool(flagdefs.FlagUnsafeEntropy)
			var mnemonic string
			var err error
			if unsafeEntropy {
				reader := bufio.NewReader(cmd.InOrStdin())
				entropy, err := readKeyInput(cmd, reader, "Enter at least 256 bits of entropy: ", "entropy")
				if err != nil {
					return err
				}
				if len(entropy) < 43 {
					return fmt.Errorf("entropy must contain at least 43 characters")
				}
				confirmed, err := confirmKeyAction(cmd, reader, fmt.Sprintf("Use supplied entropy (%d characters)?", len(entropy)))
				if err != nil || !confirmed {
					return err
				}
				hash := sha256.Sum256([]byte(entropy))
				mnemonic, err = encode(hash[:])
				if err != nil {
					return err
				}
			} else {
				mnemonic, err = generate()
				if err != nil {
					return err
				}
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), mnemonic)
			return err
		},
	}
	cmd.Flags().Bool(flagdefs.FlagUnsafeEntropy, false, "Supply your own entropy instead of using system randomness")
	cmd.Flags().BoolP(flagdefs.FlagSkipConfirmation, "y", false, "Skip entropy confirmation")
	return cmd
}
