package keys

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/go-bip39"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/spf13/cobra"

	flagdefs "pkg.akt.dev/akt/internal/flags"
)

func keyCodec() *codec.ProtoCodec {
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	return codec.NewProtoCodec(registry)
}

func addCmd(getKeyring func() (sdkkeyring.Keyring, error), recorder Recorder) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new key or recover an existing one",
		Args:  cobra.ExactArgs(1),
		Example: `  akt context keys add alice
  akt context keys add alice --recover
  printf '%s\n' "$MNEMONIC" | akt context keys add alice --recover
  akt context keys add signer --pubkey '{"@type":"/cosmos.crypto.secp256k1.PubKey","key":"..."}'
  akt context keys add team --multisig alice,signer --multisig-threshold 2
  akt context keys add ci --yes --no-backup`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("key name must not be blank")
			}

			keyType, _ := cmd.Flags().GetString(flagdefs.FlagKeyType)
			if cmd.Flags().Changed(flagdefs.FlagKeyAlgorithm) {
				alias, _ := cmd.Flags().GetString(flagdefs.FlagKeyAlgorithm)
				if cmd.Flags().Changed(flagdefs.FlagKeyType) && alias != keyType {
					return fmt.Errorf("--algo and --key-type must agree")
				}
				keyType = alias
			}

			pubJSON, _ := cmd.Flags().GetString(flagdefs.FlagPubKey)
			pubBase64, _ := cmd.Flags().GetString(flagdefs.FlagPubKeyBase64)
			var pubKey cryptotypes.PubKey
			var err error
			if cmd.Flags().Changed(flagdefs.FlagPubKeyBase64) {
				key, decodeErr := base64.StdEncoding.DecodeString(pubBase64)
				if decodeErr != nil {
					return fmt.Errorf("decode base64 public key: %w", decodeErr)
				}
				pubJSON = fmt.Sprintf(`{"@type":%q,"key":%q}`,
					"/cosmos.crypto."+keyType+".PubKey", base64.StdEncoding.EncodeToString(key))
			}
			if cmd.Flags().Changed(flagdefs.FlagPubKey) || cmd.Flags().Changed(flagdefs.FlagPubKeyBase64) {
				pubKey, err = decodePublicKey([]byte(pubJSON))
				if err != nil {
					return fmt.Errorf("parse public key: %w", err)
				}
			}

			multisigKeys, _ := cmd.Flags().GetStringSlice(flagdefs.FlagMultisig)
			dryRun, _ := cmd.Flags().GetBool(flagdefs.FlagDryRun)
			var target sdkkeyring.Keyring
			activeRecorder := recorder
			if dryRun {
				target = sdkkeyring.NewInMemory(keyCodec())
				activeRecorder = nil
			} else {
				target, err = getKeyring()
				if err != nil {
					return err
				}
				if _, err := target.Key(name); err == nil {
					return fmt.Errorf("key %q already exists; delete it first or use a different name", name)
				}
			}

			if cmd.Flags().Changed(flagdefs.FlagMultisig) {
				source := target
				if dryRun {
					source, err = getKeyring()
					if err != nil {
						return err
					}
				}
				return addMultisig(cmd, source, target, activeRecorder, name, multisigKeys)
			}
			if pubKey != nil {
				rec, err := target.SaveOfflineKey(name, pubKey)
				activeRecorder.record(cmd, actionKeysAdd, err, keyMutationDetails(name, keyTypeOffline, rec))
				return printSavedKey(cmd, rec, keyTypeOffline, err)
			}

			algos, ledgerAlgos := target.SupportedAlgorithms()
			useLedger, _ := cmd.Flags().GetBool(flagdefs.FlagUseLedger)
			if useLedger {
				algos = ledgerAlgos
			}
			algo, err := sdkkeyring.NewSigningAlgoFromString(keyType, algos)
			if err != nil {
				return err
			}
			coinType, _ := cmd.Flags().GetUint32(flagdefs.FlagCoinType)
			account, _ := cmd.Flags().GetUint32(flagdefs.FlagAccount)
			index, _ := cmd.Flags().GetUint32(flagdefs.FlagIndex)
			path, _ := cmd.Flags().GetString(flagdefs.FlagHDPath)
			if path == "" {
				path = hd.CreateHDPath(coinType, account, index).String()
			} else if useLedger {
				return fmt.Errorf("cannot set a custom HD path with Ledger")
			}
			if useLedger {
				prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
				rec, err := target.SaveLedgerKey(name, algo, prefix, coinType, account, index)
				activeRecorder.record(cmd, actionKeysAdd, err, keyMutationDetails(name, keyTypeLedger, rec))
				return printSavedKey(cmd, rec, keyTypeLedger, err)
			}

			return addLocalKey(cmd, target, activeRecorder, name, path, algo, generateMnemonic)
		},
	}

	cmd.Flags().Bool(flagdefs.FlagRecover, false, "Recover key from existing mnemonic")
	cmd.Flags().Bool(flagdefs.FlagNoBackup, false, "Don't print mnemonic after creation")
	cmd.Flags().BoolP(flagdefs.FlagSkipConfirmation, "y", false, "Skip confirmation prompts (key add never overwrites)")
	cmd.Flags().BoolP(flagdefs.FlagInteractive, "i", false, "Prompt for mnemonic and BIP39 passphrase")
	cmd.Flags().Bool(flagdefs.FlagUseLedger, false, "Use Ledger hardware wallet")
	cmd.Flags().Uint32(flagdefs.FlagCoinType, sdk.GetConfig().GetCoinType(), "BIP44 coin type")
	cmd.Flags().Uint32(flagdefs.FlagAccount, 0, "BIP44 account number")
	cmd.Flags().Uint32(flagdefs.FlagIndex, 0, "BIP44 address index")
	cmd.Flags().String(flagdefs.FlagHDPath, "", "Manual HD path override")
	cmd.Flags().String(flagdefs.FlagKeyType, "secp256k1", "Signing algorithm")
	cmd.Flags().String(flagdefs.FlagKeyAlgorithm, "secp256k1", "Alias for --key-type")
	_ = cmd.Flags().MarkHidden(flagdefs.FlagKeyAlgorithm)
	cmd.Flags().StringSlice(flagdefs.FlagMultisig, nil, "Key names for multisig (comma-separated or repeated)")
	cmd.Flags().Int(flagdefs.FlagMultisigThreshold, 1, "K-of-N threshold for multisig")
	cmd.Flags().Bool(flagdefs.FlagNoSort, false, "Keep multisig keys in supplied order instead of sorting by address")
	cmd.Flags().String(flagdefs.FlagMnemonicSource, "", "File path to read mnemonic from")
	cmd.Flags().String(flagdefs.FlagPubKey, "", "Import a public key in Protobuf JSON format")
	cmd.Flags().String(flagdefs.FlagPubKeyBase64, "", "Import a base64 public key using --key-type")
	cmd.Flags().Bool(flagdefs.FlagDryRun, false, "Create the key in memory without saving it")
	cmd.MarkFlagsMutuallyExclusive(flagdefs.FlagPubKey, flagdefs.FlagPubKeyBase64, flagdefs.FlagMultisig, flagdefs.FlagUseLedger)
	for _, mode := range []string{flagdefs.FlagPubKey, flagdefs.FlagPubKeyBase64, flagdefs.FlagMultisig, flagdefs.FlagUseLedger} {
		for _, mnemonic := range []string{flagdefs.FlagRecover, flagdefs.FlagMnemonicSource, flagdefs.FlagInteractive} {
			cmd.MarkFlagsMutuallyExclusive(mode, mnemonic)
		}
	}
	return cmd
}

func addLocalKey(cmd *cobra.Command, kr sdkkeyring.Keyring, recorder Recorder, name, path string, algo sdkkeyring.SignatureAlgo, generate func() (string, error)) error {
	recoverKey, _ := cmd.Flags().GetBool(flagdefs.FlagRecover)
	interactive, _ := cmd.Flags().GetBool(flagdefs.FlagInteractive)
	source, _ := cmd.Flags().GetString(flagdefs.FlagMnemonicSource)
	// The SDK input buffer is shared with keyring password reads. Standalone
	// keys command trees use their configured Cobra input.
	in := sdkclient.GetClientContextFromCmd(cmd).Input
	if in == nil {
		in = cmd.InOrStdin()
	}
	reader := bufio.NewReader(in)
	var mnemonic string
	var err error
	if source != "" {
		data, readErr := os.ReadFile(source)
		if readErr != nil {
			return fmt.Errorf("read mnemonic source: %w", readErr)
		}
		mnemonic = strings.TrimSpace(string(data))
	} else if recoverKey || interactive {
		prompt := "Enter your bip39 mnemonic: "
		if !recoverKey {
			prompt = "Enter your bip39 mnemonic, or hit enter to generate one: "
		}
		mnemonic, err = readKeyInput(cmd, reader, prompt, "mnemonic")
		if err != nil {
			return err
		}
	}
	generated := mnemonic == "" && !recoverKey && source == ""
	if !generated && !bip39.IsMnemonicValid(mnemonic) {
		return fmt.Errorf("invalid mnemonic")
	}
	if generated {
		mnemonic, err = generate()
		if err != nil {
			return err
		}
	}
	var passphrase string
	if interactive {
		passphrase, err = readKeyInput(cmd, reader, "Enter BIP39 passphrase (leave empty for none): ", "BIP39 passphrase")
		if err != nil {
			return err
		}
		if passphrase != "" {
			repeated, err := readKeyInput(cmd, reader, "Repeat BIP39 passphrase: ", "BIP39 passphrase confirmation")
			if err != nil {
				return err
			}
			if repeated != passphrase {
				return fmt.Errorf("BIP39 passphrases do not match")
			}
		}
	}

	rec, err := kr.NewAccount(name, mnemonic, passphrase, path, algo)
	action := actionKeysAdd
	if !generated {
		action = actionKeysRecover
	}
	recorder.record(cmd, action, err, keyMutationDetails(name, keyTypeLocal, rec))
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}
	addr, err := rec.GetAddress()
	if err != nil {
		return fmt.Errorf("get address: %w", err)
	}
	result := keyAddResult{
		Name:    rec.Name,
		Address: addr.String(),
		Type:    string(algo.Name()),
	}
	noBackup, _ := cmd.Flags().GetBool(flagdefs.FlagNoBackup)
	if generated && !noBackup {
		result.Mnemonic = mnemonic
	}
	return printAddedKey(cmd, result)
}

func printSavedKey(cmd *cobra.Command, rec *sdkkeyring.Record, keyType string, saveErr error) error {
	if saveErr != nil {
		return fmt.Errorf("save %s key: %w", keyType, saveErr)
	}
	addr, err := rec.GetAddress()
	if err != nil {
		return fmt.Errorf("get address: %w", err)
	}
	return printAddedKey(cmd, keyAddResult{
		Name:    rec.Name,
		Address: addr.String(),
		Type:    keyType,
	})
}

func addMultisig(cmd *cobra.Command, source, target sdkkeyring.Keyring, recorder Recorder, name string, names []string) error {
	threshold, _ := cmd.Flags().GetInt(flagdefs.FlagMultisigThreshold)
	if threshold < 1 || threshold > len(names) {
		return fmt.Errorf("multisig threshold must be between 1 and %d", len(names))
	}
	pks := make([]cryptotypes.PubKey, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return fmt.Errorf("multisig key names must be nonempty and unique")
		}
		seen[n] = true
		rec, err := source.Key(n)
		if err != nil {
			return fmt.Errorf("key %q not found: %w", n, err)
		}
		pk, err := rec.GetPubKey()
		if err != nil {
			return fmt.Errorf("get pubkey for %q: %w", n, err)
		}
		pks = append(pks, pk)
	}
	noSort, _ := cmd.Flags().GetBool(flagdefs.FlagNoSort)
	if !noSort {
		sort.Slice(pks, func(i, j int) bool { return bytes.Compare(pks[i].Address(), pks[j].Address()) < 0 })
	}
	pk := multisig.NewLegacyAminoPubKey(threshold, pks)
	if _, err := multisig.AminoCdc.Marshal(pk); err != nil {
		return fmt.Errorf("encode multisig public key: %w", err)
	}
	rec, err := target.SaveMultisig(name, pk)
	details := keyMutationDetails(name, keyTypeMulti, rec)
	details["threshold"] = strconv.Itoa(threshold)
	details["pubkeys"] = strconv.Itoa(len(pks))
	recorder.record(cmd, actionKeysAdd, err, details)
	if err != nil {
		return fmt.Errorf("save multisig: %w", err)
	}
	addr, err := rec.GetAddress()
	if err != nil {
		return fmt.Errorf("get address: %w", err)
	}
	return printAddedKey(cmd, keyAddResult{
		Name:      rec.Name,
		Address:   addr.String(),
		Type:      keyTypeMulti,
		Threshold: threshold,
		PubKeys:   len(pks),
	})
}

func decodePublicKey(data []byte) (cryptotypes.PubKey, error) {
	// Check byte fields before calling the SDK's custom JSON decoders, and
	// thresholds before its multisig constructor (which panics on bad input).
	var fields struct {
		Type       string            `json:"@type"`
		Key        []byte            `json:"key"`
		Threshold  json.Number       `json:"threshold"`
		PublicKeys []json.RawMessage `json:"public_keys"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields.Type == "/cosmos.crypto.multisig.LegacyAminoPubKey" {
		threshold, err := strconv.Atoi(string(fields.Threshold))
		if err != nil || threshold < 1 || threshold > len(fields.PublicKeys) {
			return nil, fmt.Errorf("invalid multisig threshold")
		}
		keys := make([]cryptotypes.PubKey, 0, len(fields.PublicKeys))
		for _, raw := range fields.PublicKeys {
			pk, err := decodePublicKey(raw)
			if err != nil {
				return nil, err
			}
			keys = append(keys, pk)
		}
		pk := multisig.NewLegacyAminoPubKey(threshold, keys)
		if _, err := multisig.AminoCdc.Marshal(pk); err != nil {
			return nil, fmt.Errorf("encode multisig public key: %w", err)
		}
		return pk, nil
	}
	if len(fields.Key) == 0 {
		return nil, fmt.Errorf("public key bytes are required")
	}
	switch fields.Type {
	case "/cosmos.crypto.secp256k1.PubKey":
		if len(fields.Key) != secp256k1.PubKeySize {
			return nil, fmt.Errorf("secp256k1 public key must be %d bytes", secp256k1.PubKeySize)
		}
		if _, err := secp.ParsePubKey(fields.Key); err != nil {
			return nil, fmt.Errorf("invalid secp256k1 public key: %w", err)
		}
	case "/cosmos.crypto.ed25519.PubKey":
		if len(fields.Key) != ed25519.PubKeySize {
			return nil, fmt.Errorf("ed25519 public key must be %d bytes", ed25519.PubKeySize)
		}
	case "/cosmos.crypto.secp256r1.PubKey":
		// The SDK decoder validates the point on the curve.
	default:
		return nil, fmt.Errorf("unsupported public key type %q", fields.Type)
	}
	var pk cryptotypes.PubKey
	if err := keyCodec().UnmarshalInterfaceJSON(data, &pk); err != nil {
		return nil, err
	}
	return pk, nil
}
