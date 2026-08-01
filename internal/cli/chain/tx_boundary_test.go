package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func txFlagCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func TestTransactionModeFlagsRejectUnknownValues(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flag    string
		allowed []string
		def     string
	}{
		{
			name: "sign mode",
			flag: cflags.FlagSignMode,
			allowed: []string{
				cflags.SignModeDirect,
				cflags.SignModeLegacyAminoJSON,
				cflags.SignModeDirectAux,
				cflags.SignModeEIP191,
			},
			def: cflags.SignModeDirect,
		},
		{
			name: "broadcast mode",
			flag: cflags.FlagBroadcastMode,
			allowed: []string{
				cflags.BroadcastSync,
				cflags.BroadcastAsync,
				cflags.BroadcastBlock,
			},
			def: cflags.BroadcastSync,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, value := range tc.allowed {
				cmd := txFlagCommand()
				if err := cmd.Flags().Set(tc.flag, value); err != nil {
					t.Errorf("--%s %s: %v", tc.flag, value, err)
				}
			}

			cmd := txFlagCommand()
			flag := cmd.Flags().Lookup(tc.flag)
			if flag.DefValue != tc.def {
				t.Errorf("--%s default = %q, want %q", tc.flag, flag.DefValue, tc.def)
			}
			for _, value := range tc.allowed {
				if !strings.Contains(flag.Usage, value) {
					t.Errorf("--%s help %q omits %q", tc.flag, flag.Usage, value)
				}
			}
			if err := cmd.Flags().Set(tc.flag, "not-a-mode"); err == nil {
				t.Errorf("--%s accepted an unknown value", tc.flag)
			}
		})
	}
}

func TestValidateTxInvocationChainIdentity(t *testing.T) {
	context := sdkclient.Context{}.WithChainID("akashnet-2")

	t.Run("matching online chain", func(t *testing.T) {
		cmd := txFlagCommand()
		if err := cmd.Flags().Set(cflags.FlagChainID, "akashnet-2"); err != nil {
			t.Fatal(err)
		}
		if err := validateTxInvocation(context, cmd.Flags()); err != nil {
			t.Fatalf("matching chain rejected: %v", err)
		}
	})

	t.Run("mismatched online chain", func(t *testing.T) {
		cmd := txFlagCommand()
		if err := cmd.Flags().Set(cflags.FlagChainID, "wrong-chain"); err != nil {
			t.Fatal(err)
		}
		err := validateTxInvocation(context, cmd.Flags())
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("mismatched chain error = %v", err)
		}
	})

	t.Run("mismatched explicit offline chain", func(t *testing.T) {
		cmd := txFlagCommand()
		if err := cmd.Flags().Set(cflags.FlagChainID, "other-chain"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set(cflags.FlagOffline, "true"); err != nil {
			t.Fatal(err)
		}
		if err := validateTxInvocation(context, cmd.Flags()); err != nil {
			t.Fatalf("explicit offline chain rejected: %v", err)
		}
	})

	t.Run("empty explicit chain", func(t *testing.T) {
		cmd := txFlagCommand()
		if err := cmd.Flags().Set(cflags.FlagChainID, ""); err != nil {
			t.Fatal(err)
		}
		err := validateTxInvocation(context, cmd.Flags())
		if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
			t.Fatalf("empty chain error = %v", err)
		}
	})
}

func TestReadTxFlagsRejectsUnresolvedPreselectedSigner(t *testing.T) {
	cmd := txFlagCommand()
	cctx := sdkclient.Context{}.
		WithCmdContext(context.Background()).
		WithHomeDir(t.TempDir()).
		WithKeyring(aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)).
		WithFrom("missing-signer")

	_, err := ReadTxCommandFlags(cctx, cmd.Flags())
	if err == nil || !strings.Contains(err.Error(), "missing-signer") {
		t.Fatalf("unresolved signer error = %v", err)
	}
}

func TestCertificateFlagsRemainLeafLocal(t *testing.T) {
	clientGenerate := GetTxCertGenerateClientCmd()
	serverGenerate := GetTxCertGenerateServerCmd()
	for name, value := range map[string]string{
		flagStart:            "2026-01-02T03:04:05Z",
		flagValidTime:        "2h",
		cflags.FlagOverwrite: "true",
	} {
		if err := clientGenerate.Flags().Set(name, value); err != nil {
			t.Fatalf("set client --%s: %v", name, err)
		}
	}
	if err := serverGenerate.Flags().Set(flagStart, "2030-01-02T03:04:05Z"); err != nil {
		t.Fatal(err)
	}

	clientOptions, err := certGenerateOptionsFromCmd(clientGenerate)
	if err != nil {
		t.Fatalf("client generate options: %v", err)
	}
	serverOptions, err := certGenerateOptionsFromCmd(serverGenerate)
	if err != nil {
		t.Fatalf("server generate options: %v", err)
	}
	if got := clientOptions.startTime.Format(time.RFC3339); got != "2026-01-02T03:04:05Z" {
		t.Errorf("client start = %q", got)
	}
	if clientOptions.validDuration != 2*time.Hour || !clientOptions.allowOverwrite {
		t.Errorf("client options = %+v", clientOptions)
	}
	if got := serverOptions.startTime.Format(time.RFC3339); got != "2030-01-02T03:04:05Z" {
		t.Errorf("server start = %q", got)
	}
	if serverOptions.validDuration != 365*24*time.Hour || serverOptions.allowOverwrite {
		t.Errorf("server options = %+v", serverOptions)
	}

	clientPublish := GetTxCertPublishClientCmd()
	serverPublish := GetTxCertPublishServerCmd()
	if err := clientPublish.Flags().Set(flagToGenesis, "true"); err != nil {
		t.Fatal(err)
	}
	if !certToGenesisFromCmd(clientPublish) || certToGenesisFromCmd(serverPublish) {
		t.Fatal("--to-genesis leaked between publish siblings")
	}

	clientRevoke := GetTxCertsRevokeClientCmd()
	serverRevoke := GetTxCertRevokeServerCmd()
	if err := clientRevoke.Flags().Set(flagSerial, "11"); err != nil {
		t.Fatal(err)
	}
	if err := serverRevoke.Flags().Set(flagSerial, "22"); err != nil {
		t.Fatal(err)
	}
	if got := certSerialFromCmd(clientRevoke); got != "11" {
		t.Errorf("client serial = %q", got)
	}
	if got := certSerialFromCmd(serverRevoke); got != "22" {
		t.Errorf("server serial = %q", got)
	}
}

func TestBatchMultisignReadsNoAutoIncrementFromItsCommand(t *testing.T) {
	cmd := GetMultiSignBatchCmd()
	if batchNoAutoIncrement(cmd) {
		t.Fatal("no-auto-increment defaulted true")
	}
	if err := cmd.Flags().Set(cflags.FlagNoAutoIncrement, "true"); err != nil {
		t.Fatal(err)
	}
	if !batchNoAutoIncrement(cmd) {
		t.Fatal("--no-auto-increment was ignored")
	}
}

func TestGetMultisigRecordRejectsOrdinaryKey(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	_, _, err := kr.NewMnemonic(
		"ordinary",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create ordinary key: %v", err)
	}

	cctx := sdkclient.Context{}.WithKeyring(kr)
	_, _, err = getMultisigRecord(cctx, "ordinary")
	if err == nil || !strings.Contains(err.Error(), "ordinary") || !strings.Contains(err.Error(), "multisig") {
		t.Fatalf("getMultisigRecord error = %v", err)
	}
}

func TestSignatureForTransactionRejectsShortBatch(t *testing.T) {
	_, err := signatureForTransaction([]signingtypes.SignatureV2{}, 0, "short.json")
	if err == nil || !strings.Contains(err.Error(), "short.json") || !strings.Contains(err.Error(), "transaction 1") {
		t.Fatalf("signatureForTransaction error = %v", err)
	}
}

func TestClientContextForTxClientPreservesAddressWithoutKeyLookup(t *testing.T) {
	address := sdk.AccAddress([]byte("01234567890123456789"))
	base := sdkclient.Context{}.
		WithFrom(address.String()).
		WithFromAddress(address)

	got := clientContextForTxClient(base.WithGenerateOnly(true))
	if got.From != "" || got.FromName != "" {
		t.Fatalf("signer lookup fields remain: from=%q name=%q", got.From, got.FromName)
	}
	if !got.GetFromAddress().Equals(address) {
		t.Fatalf("from address = %s, want %s", got.GetFromAddress(), address)
	}

	named := base.WithFrom("ordinary").WithFromName("ordinary").WithGenerateOnly(true)
	if got := clientContextForTxClient(named); got.From != "ordinary" || got.FromName != "ordinary" {
		t.Fatalf("named signer was cleared: from=%q name=%q", got.From, got.FromName)
	}
}
