package keyring

import (
	"errors"
	"reflect"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
)

func TestDeferredKeyringDoesNotLoadForBackendInspection(t *testing.T) {
	loads := 0
	deferred := NewDeferred(sdkkeyring.BackendOS, func() (sdkkeyring.Keyring, error) {
		loads++
		return NewInMemory(aktcodec.MakeEncodingConfig().Codec), nil
	})

	require.Equal(t, sdkkeyring.BackendOS, deferred.Backend())
	supported, ledger := deferred.SupportedAlgorithms()
	require.NotEmpty(t, supported)
	require.NotEmpty(t, ledger)
	require.Zero(t, loads)

	_, err := deferred.List()
	require.NoError(t, err)
	require.Equal(t, 1, loads)

	_, err = deferred.List()
	require.NoError(t, err)
	require.Equal(t, 1, loads, "the loaded keyring must be cached")
}

func TestDeferredKeyringDelegatesEveryKeyOperation(t *testing.T) {
	spy := &deferredKeyringSpy{}
	loads := 0
	deferred := NewDeferred(sdkkeyring.BackendTest, func() (sdkkeyring.Keyring, error) {
		loads++
		return spy, nil
	})
	address := sdk.AccAddress{1, 2, 3}
	algo := DefaultAlgo()

	records, err := deferred.List()
	require.NoError(t, err)
	require.Len(t, records, 1)
	record, err := deferred.Key("alice")
	require.NoError(t, err)
	require.Equal(t, "alice", record.Name)
	record, err = deferred.KeyByAddress(address)
	require.NoError(t, err)
	require.Equal(t, "address", record.Name)
	require.NoError(t, deferred.Delete("alice"))
	require.NoError(t, deferred.DeleteByAddress(address))
	require.NoError(t, deferred.Rename("alice", "bob"))
	record, mnemonic, err := deferred.NewMnemonic("new", sdkkeyring.English, "m/44'/118'/0'/0/0", "pass", algo)
	require.NoError(t, err)
	require.Equal(t, "new", record.Name)
	require.Equal(t, "seed words", mnemonic)
	record, err = deferred.NewAccount("recovered", "seed words", "pass", "path", algo)
	require.NoError(t, err)
	require.Equal(t, "recovered", record.Name)
	record, err = deferred.SaveLedgerKey("ledger", algo, "akash", 118, 1, 2)
	require.NoError(t, err)
	require.Equal(t, "ledger", record.Name)
	record, err = deferred.SaveOfflineKey("offline", nil)
	require.NoError(t, err)
	require.Equal(t, "offline", record.Name)
	record, err = deferred.SaveMultisig("multisig", nil)
	require.NoError(t, err)
	require.Equal(t, "multisig", record.Name)
	signature, _, err := deferred.Sign("alice", []byte("message"), signing.SignMode_SIGN_MODE_DIRECT)
	require.NoError(t, err)
	require.Equal(t, []byte("signature"), signature)
	signature, _, err = deferred.SignByAddress(address, []byte("message"), signing.SignMode_SIGN_MODE_DIRECT)
	require.NoError(t, err)
	require.Equal(t, []byte("address-signature"), signature)
	require.NoError(t, deferred.ImportPrivKey("private", "armor", "pass"))
	require.NoError(t, deferred.ImportPrivKeyHex("hex", "0123", "secp256k1"))
	require.NoError(t, deferred.ImportPubKey("public", "armor"))
	armor, err := deferred.ExportPubKeyArmor("public")
	require.NoError(t, err)
	require.Equal(t, "public-armor", armor)
	armor, err = deferred.ExportPubKeyArmorByAddress(address)
	require.NoError(t, err)
	require.Equal(t, "address-public-armor", armor)
	armor, err = deferred.ExportPrivKeyArmor("private", "pass")
	require.NoError(t, err)
	require.Equal(t, "private-armor", armor)
	armor, err = deferred.ExportPrivKeyArmorByAddress(address, "pass")
	require.NoError(t, err)
	require.Equal(t, "address-private-armor", armor)
	records, err = deferred.MigrateAll()
	require.NoError(t, err)
	require.Len(t, records, 1)

	wantCalls := []string{
		"List", "Key:alice", "KeyByAddress", "Delete:alice", "DeleteByAddress", "Rename:alice:bob",
		"NewMnemonic:new", "NewAccount:recovered", "SaveLedgerKey:ledger", "SaveOfflineKey:offline",
		"SaveMultisig:multisig", "Sign:alice", "SignByAddress", "ImportPrivKey:private",
		"ImportPrivKeyHex:hex", "ImportPubKey:public", "ExportPubKeyArmor:public",
		"ExportPubKeyArmorByAddress", "ExportPrivKeyArmor:private",
		"ExportPrivKeyArmorByAddress", "MigrateAll",
	}
	if !reflect.DeepEqual(spy.calls, wantCalls) {
		t.Errorf("delegated calls = %v, want %v", spy.calls, wantCalls)
	}
	if loads != 1 {
		t.Errorf("loader calls = %d, want one across every operation", loads)
	}
}

func TestDeferredKeyringPropagatesAndCachesLoaderFailure(t *testing.T) {
	loaderErr := errors.New("credential store unavailable")
	loads := 0
	deferred := NewDeferred(sdkkeyring.BackendOS, func() (sdkkeyring.Keyring, error) {
		loads++
		return nil, loaderErr
	})
	address := sdk.AccAddress{1}
	algo := DefaultAlgo()
	operations := []struct {
		name string
		call func() error
	}{
		{name: "list", call: func() error { _, err := deferred.List(); return err }},
		{name: "key", call: func() error { _, err := deferred.Key("alice"); return err }},
		{name: "key by address", call: func() error { _, err := deferred.KeyByAddress(address); return err }},
		{name: "delete", call: func() error { return deferred.Delete("alice") }},
		{name: "delete by address", call: func() error { return deferred.DeleteByAddress(address) }},
		{name: "rename", call: func() error { return deferred.Rename("alice", "bob") }},
		{name: "new mnemonic", call: func() error {
			_, _, err := deferred.NewMnemonic("new", sdkkeyring.English, "path", "", algo)
			return err
		}},
		{name: "new account", call: func() error { _, err := deferred.NewAccount("new", "seed", "", "path", algo); return err }},
		{name: "ledger", call: func() error { _, err := deferred.SaveLedgerKey("ledger", algo, "akash", 118, 0, 0); return err }},
		{name: "offline", call: func() error { _, err := deferred.SaveOfflineKey("offline", nil); return err }},
		{name: "multisig", call: func() error { _, err := deferred.SaveMultisig("multisig", nil); return err }},
		{name: "sign", call: func() error { _, _, err := deferred.Sign("alice", nil, signing.SignMode_SIGN_MODE_DIRECT); return err }},
		{name: "sign by address", call: func() error {
			_, _, err := deferred.SignByAddress(address, nil, signing.SignMode_SIGN_MODE_DIRECT)
			return err
		}},
		{name: "import private", call: func() error { return deferred.ImportPrivKey("key", "armor", "pass") }},
		{name: "import private hex", call: func() error { return deferred.ImportPrivKeyHex("key", "00", "algo") }},
		{name: "import public", call: func() error { return deferred.ImportPubKey("key", "armor") }},
		{name: "export public", call: func() error { _, err := deferred.ExportPubKeyArmor("key"); return err }},
		{name: "export public by address", call: func() error { _, err := deferred.ExportPubKeyArmorByAddress(address); return err }},
		{name: "export private", call: func() error { _, err := deferred.ExportPrivKeyArmor("key", "pass"); return err }},
		{name: "export private by address", call: func() error { _, err := deferred.ExportPrivKeyArmorByAddress(address, "pass"); return err }},
		{name: "migrate", call: func() error { _, err := deferred.MigrateAll(); return err }},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.call(); !errors.Is(err, loaderErr) {
				t.Errorf("error = %v, want loader error", err)
			}
		})
	}
	if loads != 1 {
		t.Errorf("failed loader calls = %d, want one", loads)
	}
}

func TestDeferredKeyringRejectsNilLoaderResult(t *testing.T) {
	deferred := NewDeferred(sdkkeyring.BackendOS, func() (sdkkeyring.Keyring, error) { return nil, nil })
	_, err := deferred.List()
	require.EqualError(t, err, "deferred keyring loader returned no keyring")
}

type deferredKeyringSpy struct {
	sdkkeyring.Keyring
	calls []string
}

func spyRecord(name string) *sdkkeyring.Record {
	return &sdkkeyring.Record{Name: name}
}

func (s *deferredKeyringSpy) List() ([]*sdkkeyring.Record, error) {
	s.calls = append(s.calls, "List")
	return []*sdkkeyring.Record{spyRecord("listed")}, nil
}

func (s *deferredKeyringSpy) Key(uid string) (*sdkkeyring.Record, error) {
	s.calls = append(s.calls, "Key:"+uid)
	return spyRecord(uid), nil
}

func (s *deferredKeyringSpy) KeyByAddress(sdk.Address) (*sdkkeyring.Record, error) {
	s.calls = append(s.calls, "KeyByAddress")
	return spyRecord("address"), nil
}

func (s *deferredKeyringSpy) Delete(uid string) error {
	s.calls = append(s.calls, "Delete:"+uid)
	return nil
}

func (s *deferredKeyringSpy) DeleteByAddress(sdk.Address) error {
	s.calls = append(s.calls, "DeleteByAddress")
	return nil
}

func (s *deferredKeyringSpy) Rename(from, to string) error {
	s.calls = append(s.calls, "Rename:"+from+":"+to)
	return nil
}

func (s *deferredKeyringSpy) NewMnemonic(uid string, _ sdkkeyring.Language, _, _ string, _ sdkkeyring.SignatureAlgo) (*sdkkeyring.Record, string, error) {
	s.calls = append(s.calls, "NewMnemonic:"+uid)
	return spyRecord(uid), "seed words", nil
}

func (s *deferredKeyringSpy) NewAccount(uid, _, _, _ string, _ sdkkeyring.SignatureAlgo) (*sdkkeyring.Record, error) {
	s.calls = append(s.calls, "NewAccount:"+uid)
	return spyRecord(uid), nil
}

func (s *deferredKeyringSpy) SaveLedgerKey(uid string, _ sdkkeyring.SignatureAlgo, _ string, _, _, _ uint32) (*sdkkeyring.Record, error) {
	s.calls = append(s.calls, "SaveLedgerKey:"+uid)
	return spyRecord(uid), nil
}

func (s *deferredKeyringSpy) SaveOfflineKey(uid string, _ cryptotypes.PubKey) (*sdkkeyring.Record, error) {
	s.calls = append(s.calls, "SaveOfflineKey:"+uid)
	return spyRecord(uid), nil
}

func (s *deferredKeyringSpy) SaveMultisig(uid string, _ cryptotypes.PubKey) (*sdkkeyring.Record, error) {
	s.calls = append(s.calls, "SaveMultisig:"+uid)
	return spyRecord(uid), nil
}

func (s *deferredKeyringSpy) Sign(uid string, _ []byte, _ signing.SignMode) ([]byte, cryptotypes.PubKey, error) {
	s.calls = append(s.calls, "Sign:"+uid)
	return []byte("signature"), nil, nil
}

func (s *deferredKeyringSpy) SignByAddress(sdk.Address, []byte, signing.SignMode) ([]byte, cryptotypes.PubKey, error) {
	s.calls = append(s.calls, "SignByAddress")
	return []byte("address-signature"), nil, nil
}

func (s *deferredKeyringSpy) ImportPrivKey(uid, _, _ string) error {
	s.calls = append(s.calls, "ImportPrivKey:"+uid)
	return nil
}

func (s *deferredKeyringSpy) ImportPrivKeyHex(uid, _, _ string) error {
	s.calls = append(s.calls, "ImportPrivKeyHex:"+uid)
	return nil
}

func (s *deferredKeyringSpy) ImportPubKey(uid, _ string) error {
	s.calls = append(s.calls, "ImportPubKey:"+uid)
	return nil
}

func (s *deferredKeyringSpy) ExportPubKeyArmor(uid string) (string, error) {
	s.calls = append(s.calls, "ExportPubKeyArmor:"+uid)
	return "public-armor", nil
}

func (s *deferredKeyringSpy) ExportPubKeyArmorByAddress(sdk.Address) (string, error) {
	s.calls = append(s.calls, "ExportPubKeyArmorByAddress")
	return "address-public-armor", nil
}

func (s *deferredKeyringSpy) ExportPrivKeyArmor(uid, _ string) (string, error) {
	s.calls = append(s.calls, "ExportPrivKeyArmor:"+uid)
	return "private-armor", nil
}

func (s *deferredKeyringSpy) ExportPrivKeyArmorByAddress(sdk.Address, string) (string, error) {
	s.calls = append(s.calls, "ExportPrivKeyArmorByAddress")
	return "address-private-armor", nil
}

func (s *deferredKeyringSpy) MigrateAll() ([]*sdkkeyring.Record, error) {
	s.calls = append(s.calls, "MigrateAll")
	return []*sdkkeyring.Record{spyRecord("migrated")}, nil
}
