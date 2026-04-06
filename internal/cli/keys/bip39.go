package keys

import (
	"crypto/rand"

	"github.com/cosmos/go-bip39"
)

const mnemonicEntropySize = 256

// bip39Entropy generates random entropy for BIP39 mnemonic generation.
func bip39Entropy() ([]byte, error) {
	entropy := make([]byte, mnemonicEntropySize/8)
	if _, err := rand.Read(entropy); err != nil {
		return nil, err
	}

	return entropy, nil
}

// bip39Mnemonic creates a mnemonic from the given entropy.
func bip39Mnemonic(entropy []byte) (string, error) {
	return bip39.NewMnemonic(entropy)
}
