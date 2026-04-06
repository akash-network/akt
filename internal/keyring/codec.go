package keyring

import (
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
)

// DefaultAlgo returns the default signing algorithm (secp256k1).
func DefaultAlgo() sdkkeyring.SignatureAlgo {
	return hd.Secp256k1
}
