package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"

	errorsmod "cosmossdk.io/errors"
	txsigning "cosmossdk.io/x/tx/signing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	kmultisig "github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/version"
	authclient "github.com/cosmos/cosmos-sdk/x/auth/client"
	"github.com/cosmos/cosmos-sdk/x/auth/signing"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

// GetAuthMultiSignCmd returns the multi-sign command
func GetAuthMultiSignCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "multi-sign [file] [name] [[signature]...]",
		Aliases: []string{"multisign"},
		Short:   "Generate multisig signatures for transactions generated offline",
		Long: strings.TrimSpace(
			fmt.Sprintf(`Sign transactions created with the --generate-only flag that require multisig signatures.

Read one or more signatures from one or more [signature] file, generate a multisig signature compliant to the
multisig key [name], and attach the key name to the transaction read from [file].

Example:
$ %s tx multi-sign transaction.json k1k2k3 k1sig.json k2sig.json k3sig.json

If --signature-only flag is on, output a JSON representation
of only the generated signature.

If the --offline flag is on, the client will not reach out to an external node.
Account number or sequence number lookups are not performed so you must
set these parameters manually.

If the --skip-signature-verification flag is on, the command will not verify the
signatures in the provided signature files. This is useful when the multisig
account is a signer in a nested multisig scenario.

The current multisig implementation defaults to amino-json sign mode.
The SIGN_MODE_DIRECT sign mode is not supported.'
`,
				version.AppName,
			),
		),
		RunE: makeMultiSignCmd(),
		Args: cobra.MinimumNArgs(3),
	}

	cmd.Flags().Bool(flagdefs.FlagSkipSignatureVerification, false, "Skip signature verification")
	cmd.Flags().Bool(flagdefs.FlagSigOnly, false, "Print only the generated signature, then exit")
	cmd.Flags().String(flagdefs.FlagOutputDocument, "", "The document is written to the given file instead of STDOUT")
	cflags.AddTxFlagsToCmd(cmd)
	_ = cmd.Flags().MarkHidden(flagdefs.FlagOutput)

	return cmd
}

func makeMultiSignCmd() func(cmd *cobra.Command, args []string) (err error) {
	return func(cmd *cobra.Command, args []string) (err error) {
		_ = cmd.Flags().Set(flagdefs.FlagFrom, args[1])

		cctx, err := client.GetClientTxContext(cmd)
		if err != nil {
			return err
		}
		parsedTx, err := authclient.ReadTxFromFile(cctx, args[0])
		if err != nil {
			return err
		}

		txFactory, err := tx.NewFactoryCLI(cctx, cmd.Flags())
		if err != nil {
			return err
		}
		if txFactory.SignMode() == signingtypes.SignMode_SIGN_MODE_UNSPECIFIED {
			txFactory = txFactory.WithSignMode(signingtypes.SignMode_SIGN_MODE_LEGACY_AMINO_JSON)
		}

		txCfg := cctx.TxConfig
		txBuilder, err := txCfg.WrapTxBuilder(parsedTx)
		if err != nil {
			return err
		}

		k, multisigPub, err := getMultisigRecord(cctx, args[1])
		if err != nil {
			return err
		}

		addr, err := k.GetAddress()
		if err != nil {
			return err
		}

		// avoid signature verification if the sender of the tx is different than
		// the multisig key (useful for nested multisigs).
		skipSigVerify, err := cmd.Flags().GetBool(flagdefs.FlagSkipSignatureVerification)
		if err != nil {
			return err
		}
		multisigSig := multisig.NewMultisig(len(multisigPub.PubKeys))
		if !cctx.Offline {
			accnum, seq, err := cctx.AccountRetriever.GetAccountNumberSequence(cctx, addr)
			if err != nil {
				return err
			}

			txFactory = txFactory.WithAccountNumber(accnum).WithSequence(seq)
		}

		// read each signature and add it to the multisig if valid
		for i := 2; i < len(args); i++ {
			sigs, err := unmarshalSignatureJSON(cctx, args[i])
			if err != nil {
				return err
			}

			if txFactory.ChainID() == "" {
				return fmt.Errorf("set the chain id with either the --chain-id flag or config file")
			}

			for _, sig := range sigs {
				anyPk, err := codectypes.NewAnyWithValue(sig.PubKey)
				if err != nil {
					return err
				}
				txSignerData := txsigning.SignerData{
					ChainID:       txFactory.ChainID(),
					AccountNumber: txFactory.AccountNumber(),
					Sequence:      txFactory.Sequence(),
					Address:       sdk.AccAddress(sig.PubKey.Address()).String(),
					PubKey: &anypb.Any{
						TypeUrl: anyPk.TypeUrl,
						Value:   anyPk.Value,
					},
				}
				builtTx := txBuilder.GetTx()
				adaptableTx, ok := builtTx.(signing.V2AdaptableTx)
				if !ok {
					return fmt.Errorf("expected Tx to be signing.V2AdaptableTx, got %T", builtTx)
				}
				txData := adaptableTx.GetSigningTxData()

				if !skipSigVerify {
					err = signing.VerifySignature(cmd.Context(), sig.PubKey, txSignerData, sig.Data,
						txCfg.SignModeHandler(), txData)
					if err != nil {
						addr, _ := sdk.AccAddressFromHexUnsafe(sig.PubKey.Address().String())
						return fmt.Errorf("couldn't verify signature for address %s %w", addr, err)
					}
				}

				if err := multisig.AddSignatureV2(multisigSig, sig, multisigPub.GetPubKeys()); err != nil {
					return err
				}
			}
		}

		sigV2 := signingtypes.SignatureV2{
			PubKey:   multisigPub,
			Data:     multisigSig,
			Sequence: txFactory.Sequence(),
		}

		err = txBuilder.SetSignatures(sigV2)
		if err != nil {
			return err
		}

		sigOnly, _ := cmd.Flags().GetBool(flagdefs.FlagSigOnly)

		var json []byte
		json, err = marshalSignatureJSON(txCfg, txBuilder, sigOnly)
		if err != nil {
			return err
		}

		closeFunc, err := setOutputFile(cmd)
		if err != nil {
			return err
		}

		defer closeFunc()

		cmd.Printf("%s\n", json)
		return nil
	}
}

func unmarshalSignatureJSON(cctx client.Context, filename string) (sigs []signingtypes.SignatureV2, err error) {
	var bytes []byte
	if bytes, err = os.ReadFile(filename); err != nil { //nolint: gosec
		return
	}
	return cctx.TxConfig.UnmarshalSignatureJSON(bytes)
}

func getMultisigRecord(cctx client.Context, name string) (*keyring.Record, *kmultisig.LegacyAminoPubKey, error) {
	kb := cctx.Keyring
	multisigRecord, err := kb.Key(name)
	if err != nil {
		return nil, nil, errorsmod.Wrap(err, "error getting keybase multisig account")
	}

	pubKey, err := multisigRecord.GetPubKey()
	if err != nil {
		return nil, nil, errorsmod.Wrapf(err, "read multisig key %q", name)
	}
	multisigPub, ok := pubKey.(*kmultisig.LegacyAminoPubKey)
	if !ok {
		return nil, nil, fmt.Errorf("key %q is %T; expected a legacy amino multisig key", name, pubKey)
	}

	return multisigRecord, multisigPub, nil
}
