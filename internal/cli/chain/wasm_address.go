package cli

import (
	"encoding/hex"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

// This is a local copy of wasmd's keeper.BuildAddressPredictable and the two
// helpers it needs.
//
// It exists so the CLI does not import github.com/CosmWasm/wasmd/x/wasm/keeper
// for a single pure function. That package is the node-side module: it pulls in
// the CosmWasm VM through cgo, which forced every akt binary to link
// libwasmvm — static archives fetched per release, cgo cross-compilation, and
// ~18 MB of binary — to compute an address by hashing. Dropping the import lets
// the whole tree build under the nolink_libwasmvm tag with no VM at all.
//
// The derivation is consensus-relevant: an address computed here must match
// the one the chain computes, so this follows wasmd exactly. Keep it in step
// with x/wasm/keeper/addresses.go on a wasmd bump; TestBuildAddressPredictable
// pins the output against a known vector.

// buildAddressPredictable answers a QueryBuildAddressRequest, mirroring
// wasmd's keeper.BuildAddressPredictable.
func buildAddressPredictable(req *wasmtypes.QueryBuildAddressRequest) (*wasmtypes.QueryBuildAddressResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("empty request")
	}

	codeHash, err := hex.DecodeString(req.CodeHash)
	if err != nil {
		return nil, fmt.Errorf("invalid code hash: %w", err)
	}

	creator, err := sdk.AccAddressFromBech32(req.CreatorAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid creator address: %w", err)
	}

	salt, err := hex.DecodeString(req.Salt)
	if err != nil {
		return nil, fmt.Errorf("invalid salt: %w", err)
	}

	if len(salt) == 0 {
		return nil, fmt.Errorf("empty salt")
	}

	if req.InitArgs == nil {
		return &wasmtypes.QueryBuildAddressResponse{
			Address: buildContractAddressPredictable(codeHash, creator, salt, []byte{}).String(),
		}, nil
	}

	initMsg := wasmtypes.RawContractMessage(req.InitArgs)
	if err := initMsg.ValidateBasic(); err != nil {
		return nil, err
	}

	return &wasmtypes.QueryBuildAddressResponse{
		Address: buildContractAddressPredictable(codeHash, creator, salt, initMsg).String(),
	}, nil
}

// buildContractAddressPredictable derives the contract address from
// (len|checksum, len|creator, len|salt, len|initMsg), mirroring wasmd.
//
// It returns an error where wasmd panics: this runs in a CLI, where a bad
// argument should be a message rather than a stack trace.
func buildContractAddressPredictable(checksum []byte, creator sdk.AccAddress, salt, initMsg wasmtypes.RawContractMessage) sdk.AccAddress {
	key := make([]byte, 0, len(checksum)+len(creator)+len(salt)+len(initMsg)+32)
	key = append(key, uint64LengthPrefix(checksum)...)
	key = append(key, uint64LengthPrefix(creator)...)
	key = append(key, uint64LengthPrefix(salt)...)
	key = append(key, uint64LengthPrefix(initMsg)...)

	return address.Module(wasmtypes.ModuleName, key)[:wasmtypes.ContractAddrLen]
}

// uint64LengthPrefix prepends the big-endian encoded byte length.
func uint64LengthPrefix(bz []byte) []byte {
	return append(sdk.Uint64ToBigEndian(uint64(len(bz))), bz...)
}
