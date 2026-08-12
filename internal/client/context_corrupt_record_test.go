package client

import (
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
)

type corruptRecordLookup struct {
	sdkkeyring.Keyring
	record *sdkkeyring.Record
}

func (lookup corruptRecordLookup) Key(string) (*sdkkeyring.Record, error) {
	return lookup.record, nil
}

func TestResolveAccountAddressRejectsUnpackedPublicKey(t *testing.T) {
	// A protobuf Any can be present while its cached cryptographic value is
	// absent, as happens when persisted keyring bytes cannot be unpacked. This is
	// distinct from a nil PubKey and must still be an error rather than a panic.
	record := &sdkkeyring.Record{
		Name:   "alice",
		PubKey: &codectypes.Any{TypeUrl: "/corrupt.PublicKey", Value: []byte{0xff}},
	}
	cctx := sdkclient.Context{}.
		WithFrom("alice").
		WithKeyring(corruptRecordLookup{record: record})

	_, err := ResolveAccountAddress(cctx)
	if err == nil || !strings.Contains(err.Error(), `resolve account "alice" address`) {
		t.Fatalf("ResolveAccountAddress error = %v, want corrupt-address context", err)
	}
}
