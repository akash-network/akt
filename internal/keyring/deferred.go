package keyring

import (
	"errors"
	"sync"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
)

// DeferredLoader opens the configured keyring when an operation first needs
// key material. The returned keyring or error is cached for the process.
type DeferredLoader func() (sdkkeyring.Keyring, error)

// deferredKeyring implements the Cosmos SDK Keyring interface without opening
// its backend during client-context construction. Backend is metadata and is
// therefore the only method that does not resolve the wrapped keyring.
type deferredKeyring struct {
	backend string
	loader  DeferredLoader
	once    sync.Once
	keyring sdkkeyring.Keyring
	err     error
}

// NewDeferred returns a keyring proxy that invokes loader on its first key
// operation. It is used by public reads that may need a named default account
// later but must not prompt merely because the context has one configured.
func NewDeferred(backend string, loader DeferredLoader) sdkkeyring.Keyring {
	return &deferredKeyring{backend: backend, loader: loader}
}

func (d *deferredKeyring) resolve() (sdkkeyring.Keyring, error) {
	d.once.Do(func() {
		d.keyring, d.err = d.loader()
		if d.keyring == nil && d.err == nil {
			d.err = errors.New("deferred keyring loader returned no keyring")
		}
	})

	return d.keyring, d.err
}

func (d *deferredKeyring) Backend() string {
	return d.backend
}

func (d *deferredKeyring) List() ([]*sdkkeyring.Record, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, err
	}
	return kr.List()
}

func (d *deferredKeyring) SupportedAlgorithms() (sdkkeyring.SigningAlgoList, sdkkeyring.SigningAlgoList) {
	// Manager opens SDK keyrings without custom algorithm options, so these
	// lists are backend-independent metadata and need not resolve the store.
	algorithms := sdkkeyring.SigningAlgoList{hd.Secp256k1}
	return algorithms, algorithms
}

func (d *deferredKeyring) Key(uid string) (*sdkkeyring.Record, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, err
	}
	return kr.Key(uid)
}

func (d *deferredKeyring) KeyByAddress(address sdk.Address) (*sdkkeyring.Record, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, err
	}
	return kr.KeyByAddress(address)
}

func (d *deferredKeyring) Delete(uid string) error {
	kr, err := d.resolve()
	if err != nil {
		return err
	}
	return kr.Delete(uid)
}

func (d *deferredKeyring) DeleteByAddress(address sdk.Address) error {
	kr, err := d.resolve()
	if err != nil {
		return err
	}
	return kr.DeleteByAddress(address)
}

func (d *deferredKeyring) Rename(from, to string) error {
	kr, err := d.resolve()
	if err != nil {
		return err
	}
	return kr.Rename(from, to)
}

func (d *deferredKeyring) NewMnemonic(
	uid string,
	language sdkkeyring.Language,
	hdPath string,
	bip39Passphrase string,
	algo sdkkeyring.SignatureAlgo,
) (*sdkkeyring.Record, string, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, "", err
	}
	return kr.NewMnemonic(uid, language, hdPath, bip39Passphrase, algo)
}

func (d *deferredKeyring) NewAccount(
	uid string,
	mnemonic string,
	bip39Passphrase string,
	hdPath string,
	algo sdkkeyring.SignatureAlgo,
) (*sdkkeyring.Record, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, err
	}
	return kr.NewAccount(uid, mnemonic, bip39Passphrase, hdPath, algo)
}

func (d *deferredKeyring) SaveLedgerKey(
	uid string,
	algo sdkkeyring.SignatureAlgo,
	hrp string,
	coinType uint32,
	account uint32,
	index uint32,
) (*sdkkeyring.Record, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, err
	}
	return kr.SaveLedgerKey(uid, algo, hrp, coinType, account, index)
}

func (d *deferredKeyring) SaveOfflineKey(uid string, pubkey cryptotypes.PubKey) (*sdkkeyring.Record, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, err
	}
	return kr.SaveOfflineKey(uid, pubkey)
}

func (d *deferredKeyring) SaveMultisig(uid string, pubkey cryptotypes.PubKey) (*sdkkeyring.Record, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, err
	}
	return kr.SaveMultisig(uid, pubkey)
}

func (d *deferredKeyring) Sign(
	uid string,
	msg []byte,
	signMode signing.SignMode,
) ([]byte, cryptotypes.PubKey, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, nil, err
	}
	return kr.Sign(uid, msg, signMode)
}

func (d *deferredKeyring) SignByAddress(
	address sdk.Address,
	msg []byte,
	signMode signing.SignMode,
) ([]byte, cryptotypes.PubKey, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, nil, err
	}
	return kr.SignByAddress(address, msg, signMode)
}

func (d *deferredKeyring) ImportPrivKey(uid, armor, passphrase string) error {
	kr, err := d.resolve()
	if err != nil {
		return err
	}
	return kr.ImportPrivKey(uid, armor, passphrase)
}

func (d *deferredKeyring) ImportPrivKeyHex(uid, privKey, algo string) error {
	kr, err := d.resolve()
	if err != nil {
		return err
	}
	return kr.ImportPrivKeyHex(uid, privKey, algo)
}

func (d *deferredKeyring) ImportPubKey(uid, armor string) error {
	kr, err := d.resolve()
	if err != nil {
		return err
	}
	return kr.ImportPubKey(uid, armor)
}

func (d *deferredKeyring) ExportPubKeyArmor(uid string) (string, error) {
	kr, err := d.resolve()
	if err != nil {
		return "", err
	}
	return kr.ExportPubKeyArmor(uid)
}

func (d *deferredKeyring) ExportPubKeyArmorByAddress(address sdk.Address) (string, error) {
	kr, err := d.resolve()
	if err != nil {
		return "", err
	}
	return kr.ExportPubKeyArmorByAddress(address)
}

func (d *deferredKeyring) ExportPrivKeyArmor(uid, passphrase string) (string, error) {
	kr, err := d.resolve()
	if err != nil {
		return "", err
	}
	return kr.ExportPrivKeyArmor(uid, passphrase)
}

func (d *deferredKeyring) ExportPrivKeyArmorByAddress(address sdk.Address, passphrase string) (string, error) {
	kr, err := d.resolve()
	if err != nil {
		return "", err
	}
	return kr.ExportPrivKeyArmorByAddress(address, passphrase)
}

func (d *deferredKeyring) MigrateAll() ([]*sdkkeyring.Record, error) {
	kr, err := d.resolve()
	if err != nil {
		return nil, err
	}
	return kr.MigrateAll()
}
