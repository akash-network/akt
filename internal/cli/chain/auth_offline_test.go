package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	kmultisig "github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

type chainTestErrorWriter struct {
	err error
}

func (w chainTestErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type fixedAccountRetriever struct {
	number uint64
	seq    uint64
	err    error
}

func (r fixedAccountRetriever) GetAccount(client.Context, sdk.AccAddress) (client.Account, error) {
	return nil, r.err
}

func (r fixedAccountRetriever) GetAccountWithHeight(client.Context, sdk.AccAddress) (client.Account, int64, error) {
	return nil, 0, r.err
}

func (r fixedAccountRetriever) EnsureExists(client.Context, sdk.AccAddress) error {
	return r.err
}

func (r fixedAccountRetriever) GetAccountNumberSequence(client.Context, sdk.AccAddress) (uint64, uint64, error) {
	return r.number, r.seq, r.err
}

func encodedAuthTestTx(t *testing.T, cfg client.TxConfig, memo string) []byte {
	t.Helper()
	builder := cfg.NewTxBuilder()
	builder.SetMemo(memo)
	builder.SetGasLimit(50_000)
	builder.SetFeeAmount(sdk.NewCoins(sdk.NewInt64Coin("uakt", 125)))
	encoded, err := cfg.TxJSONEncoder()(builder.GetTx())
	if err != nil {
		t.Fatalf("encode transaction: %v", err)
	}
	return encoded
}

func writeAuthTestFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func authTestSendBuilder(t *testing.T, cfg client.TxConfig, signer sdk.AccAddress) client.TxBuilder {
	t.Helper()
	builder := cfg.NewTxBuilder()
	err := builder.SetMsgs(banktypes.NewMsgSend(
		signer,
		sdk.AccAddress(bytes.Repeat([]byte{9}, 20)),
		sdk.NewCoins(sdk.NewInt64Coin("uakt", 25)),
	))
	if err != nil {
		t.Fatalf("set message: %v", err)
	}
	return builder
}

func authTestTxMemo(t *testing.T, transaction sdk.Tx) string {
	t.Helper()
	memoTx, ok := transaction.(interface{ GetMemo() string })
	if !ok {
		t.Fatalf("transaction type %T does not expose a memo", transaction)
	}
	return memoTx.GetMemo()
}

func TestAuthTransactionInputDecoding(t *testing.T) {
	cfg := aktcodec.MakeEncodingConfig().TxConfig
	first := encodedAuthTestTx(t, cfg, "first")
	second := encodedAuthTestTx(t, cfg, "second")

	path := writeAuthTestFile(t, "transaction.json", first)
	decoded, err := ReadTxFromFile(client.Context{}.WithTxConfig(cfg), path)
	if err != nil || authTestTxMemo(t, decoded) != "first" {
		t.Fatalf("ReadTxFromFile = %q, %v", authTestTxMemo(t, decoded), err)
	}
	if _, err := ReadTxFromFile(client.Context{}.WithTxConfig(cfg), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing transaction file succeeded")
	}
	invalid := writeAuthTestFile(t, "invalid.json", []byte("not-json"))
	if _, err := ReadTxFromFile(client.Context{}.WithTxConfig(cfg), invalid); err == nil {
		t.Fatal("invalid transaction succeeded")
	}

	firstPath := writeAuthTestFile(t, "first.json", append(first, '\n'))
	secondPath := writeAuthTestFile(t, "second.json", append(second, '\n'))
	scanner, err := ReadTxsFromInput(cfg, firstPath, secondPath)
	if err != nil {
		t.Fatalf("ReadTxsFromInput: %v", err)
	}
	var memos []string
	for scanner.Scan() {
		memos = append(memos, authTestTxMemo(t, scanner.Tx()))
	}
	if scanner.Err() != nil || scanner.UnmarshalErr() != nil || strings.Join(memos, ",") != "first,second" {
		t.Fatalf("scan = %v/%v/%q", scanner.Err(), scanner.UnmarshalErr(), memos)
	}

	malformed := NewBatchScanner(cfg, strings.NewReader(fmt.Sprintf("%s\nbad\n%s\n", first, second)))
	if !malformed.Scan() || authTestTxMemo(t, malformed.Tx()) != "first" {
		t.Fatal("first batch transaction was not decoded")
	}
	if malformed.Scan() || malformed.UnmarshalErr() == nil {
		t.Fatalf("malformed scan = %t, %v", malformed.Scan(), malformed.UnmarshalErr())
	}
	if _, err := ReadTxsFromInput(cfg); err == nil || !strings.Contains(err.Error(), "no file") {
		t.Fatalf("empty filenames error = %v", err)
	}
}

func TestAuthSimulationAndAccountBoundaries(t *testing.T) {
	user := []byte{1, 2, 3}
	if !isTxSigner(user, [][]byte{{9}, {1, 2, 3}}) {
		t.Fatal("exact signer not found")
	}
	if isTxSigner(user, [][]byte{{1, 2}, {1, 2, 3, 4}}) {
		t.Fatal("prefix signer matched")
	}

	addr := sdk.AccAddress(bytes.Repeat([]byte{1}, 20))
	factory := clienttx.Factory{}
	retriever := fixedAccountRetriever{number: 31, seq: 47}
	populated, err := populateAccountFromState(factory, client.Context{}.WithAccountRetriever(retriever), addr)
	if err != nil || populated.AccountNumber() != 31 || populated.Sequence() != 47 {
		t.Fatalf("populate = %d/%d, %v", populated.AccountNumber(), populated.Sequence(), err)
	}
	wantErr := errors.New("query failed")
	retriever = fixedAccountRetriever{err: wantErr}
	unchanged, err := populateAccountFromState(factory, client.Context{}.WithAccountRetriever(retriever), addr)
	if !errors.Is(err, wantErr) || unchanged.AccountNumber() != 0 || unchanged.Sequence() != 0 {
		t.Fatalf("failed populate = %d/%d, %v", unchanged.AccountNumber(), unchanged.Sequence(), err)
	}
}

func TestSignTxOfflineValidatesSignerAndMode(t *testing.T) {
	encoding := aktcodec.MakeEncodingConfig()
	keys := aktkeyring.NewInMemory(encoding.Codec)
	record, _, err := keys.NewMnemonic(
		"alice",
		keyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := record.GetAddress()
	if err != nil {
		t.Fatal(err)
	}
	cctx := client.Context{}.
		WithTxConfig(encoding.TxConfig).
		WithKeyring(keys).
		WithChainID("offline-chain").
		WithFromName("alice").
		WithCmdContext(context.Background())
	factory := clienttx.Factory{}.
		WithTxConfig(encoding.TxConfig).
		WithKeybase(keys).
		WithChainID(cctx.ChainID).
		WithAccountNumber(7).
		WithSequence(11).
		WithSignMode(signingtypes.SignMode_SIGN_MODE_DIRECT)

	builder := authTestSendBuilder(t, encoding.TxConfig, signer)
	if err := SignTx(factory, cctx, "alice", builder, true, true); err != nil {
		t.Fatalf("SignTx: %v", err)
	}
	signatures, err := builder.GetTx().GetSignaturesV2()
	if err != nil || len(signatures) != 1 || !bytes.Equal(signatures[0].PubKey.Address(), signer) {
		t.Fatalf("signatures = %#v, %v", signatures, err)
	}

	wrong := authTestSendBuilder(t, encoding.TxConfig, sdk.AccAddress(bytes.Repeat([]byte{3}, 20)))
	err = SignTx(factory, cctx, "alice", wrong, true, true)
	if err == nil || !errors.Is(err, sdkerrors.ErrorInvalidSigner) || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("wrong signer error = %v", err)
	}

}

func TestMultisigOfflineBoundaries(t *testing.T) {
	first := secp256k1.GenPrivKey().PubKey()
	second := secp256k1.GenPrivKey().PubKey()
	absent := secp256k1.GenPrivKey().PubKey()
	nested := kmultisig.NewLegacyAminoPubKey(1, []cryptotypes.PubKey{second})
	outer := kmultisig.NewLegacyAminoPubKey(2, []cryptotypes.PubKey{first, nested})
	for _, tc := range []struct {
		name string
		key  cryptotypes.PubKey
		want bool
	}{
		{name: "direct", key: first, want: true},
		{name: "nested", key: second, want: true},
		{name: "absent", key: absent, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := isMultisigSigner(client.Context{}, outer, tc.key)
			if err != nil || got != tc.want {
				t.Fatalf("isMultisigSigner = %t, %v", got, err)
			}
		})
	}

	encoding := aktcodec.MakeEncodingConfig()
	keys := aktkeyring.NewInMemory(encoding.Codec)
	if _, err := keys.SaveMultisig("group", outer); err != nil {
		t.Fatal(err)
	}
	cctx := client.Context{}.WithKeyring(keys).WithTxConfig(encoding.TxConfig)
	record, key, err := getMultisigRecord(cctx, "group")
	if err != nil || record.Name != "group" || !key.Equals(outer) {
		t.Fatalf("getMultisigRecord = %v/%T, %v", record, key, err)
	}
	if _, _, err := getMultisigRecord(cctx, "missing"); err == nil {
		t.Fatal("missing multisig record succeeded")
	}

	sig := signingtypes.SignatureV2{
		PubKey: first,
		Data: &signingtypes.SingleSignatureData{
			SignMode:  signingtypes.SignMode_SIGN_MODE_LEGACY_AMINO_JSON,
			Signature: []byte{1, 2, 3},
		},
		Sequence: 8,
	}
	sigJSON, err := encoding.TxConfig.MarshalSignatureJSON([]signingtypes.SignatureV2{sig})
	if err != nil {
		t.Fatal(err)
	}
	path := writeAuthTestFile(t, "signature.json", sigJSON)
	decoded, err := unmarshalSignatureJSON(cctx, path)
	if err != nil || len(decoded) != 1 || decoded[0].Sequence != 8 {
		t.Fatalf("unmarshalSignatureJSON = %#v, %v", decoded, err)
	}
}

func TestAuthOutputAndSignatureReportBoundaries(t *testing.T) {
	path := writeAuthTestFile(t, "signed.json", []byte("old"))
	cmd := &cobra.Command{}
	cmd.Flags().String(cflags.FlagOutputDocument, "", "")
	if err := cmd.Flags().Set(cflags.FlagOutputDocument, path); err != nil {
		t.Fatal(err)
	}
	closeOutput, err := setOutputFile(cmd)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Print("new")
	closeOutput()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("output document = %q, %v", got, err)
	}

	signCmd := GetSignCommand()
	if err := signCmd.Flags().Set(cflags.FlagOffline, "true"); err != nil {
		t.Fatal(err)
	}
	preSignCmd(signCmd, nil)
	for _, name := range []string{cflags.FlagAccountNumber, cflags.FlagSequence} {
		flag := signCmd.Flags().Lookup(name)
		if flag == nil || flag.Annotations[cobra.BashCompOneRequiredFlag] == nil {
			t.Fatalf("--%s was not required offline", name)
		}
	}

	encoding := aktcodec.MakeEncodingConfig()
	pub := secp256k1.GenPrivKey().PubKey()
	signer := sdk.AccAddress(pub.Address())
	builder := authTestSendBuilder(t, encoding.TxConfig, signer)
	if err := builder.SetSignatures(signingtypes.SignatureV2{
		PubKey: pub,
		Data: &signingtypes.SingleSignatureData{
			SignMode:  signingtypes.SignMode_SIGN_MODE_DIRECT,
			Signature: []byte("offline"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	reportCmd := &cobra.Command{}
	var report bytes.Buffer
	reportCmd.SetOut(&report)
	reportCmd.SetContext(context.Background())
	cctx := client.Context{}.WithTxConfig(encoding.TxConfig).WithCmdContext(reportCmd.Context())
	valid, err := printAndValidateSigs(reportCmd, cctx, "offline-chain", builder.GetTx(), true)
	if err != nil || !valid || !strings.Contains(report.String(), signer.String()) || !strings.Contains(report.String(), "[OK]") {
		t.Fatalf("report = %t/%q, %v", valid, report.String(), err)
	}

	badCmd := &cobra.Command{}
	wantErr := errors.New("write failed")
	badCmd.SetOut(chainTestErrorWriter{err: wantErr})
	badCmd.SetContext(context.Background())
	valid, err = printAndValidateSigs(badCmd, cctx, "offline-chain", builder.GetTx(), true)
	if valid || !errors.Is(err, wantErr) {
		t.Fatalf("writer result = %t, %v", valid, err)
	}
}
