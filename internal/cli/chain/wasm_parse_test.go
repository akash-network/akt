package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CosmWasm/wasmd/x/wasm/ioutils"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	wasmvm "github.com/CosmWasm/wasmvm/v3"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestParseWasmStoreCodeArgsValidatesAndNormalizesArtifact(t *testing.T) {
	sender := sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String()
	wasm := []byte("\x00asm\x01\x00\x00\x00")

	t.Run("raw Wasm is gzipped", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "counter.wasm")
		require.NoError(t, os.WriteFile(path, wasm, 0o600))

		cmd := GetTxWasmStoreCodeCmd()
		msg, err := ParseWasmStoreCodeArgs(path, sender, cmd.Flags())
		require.NoError(t, err)
		require.Equal(t, sender, msg.Sender)
		require.True(t, ioutils.IsGzip(msg.WASMByteCode))
		uncompressed, err := ioutils.Uncompress(msg.WASMByteCode, int64(wasmtypes.MaxWasmSize))
		require.NoError(t, err)
		require.Equal(t, wasm, uncompressed)
	})

	t.Run("valid gzip is retained", func(t *testing.T) {
		compressed, err := ioutils.GzipIt(wasm)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "counter.wasm.gz")
		require.NoError(t, os.WriteFile(path, compressed, 0o600))

		cmd := GetTxWasmStoreCodeCmd()
		msg, err := ParseWasmStoreCodeArgs(path, sender, cmd.Flags())
		require.NoError(t, err)
		require.Equal(t, compressed, msg.WASMByteCode)
	})

	t.Run("any-of permission preserves complete addresses", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "counter.wasm")
		require.NoError(t, os.WriteFile(path, wasm, 0o600))
		first := sdk.AccAddress(bytes.Repeat([]byte{2}, 20))
		second := sdk.AccAddress(bytes.Repeat([]byte{3}, 20))

		cmd := GetTxWasmStoreCodeCmd()
		require.NoError(t, cmd.Flags().Set(
			cflags.FlagInstantiateByAnyOfAddress,
			first.String()+","+second.String(),
		))
		msg, err := ParseWasmStoreCodeArgs(path, sender, cmd.Flags())
		require.NoError(t, err)
		require.Equal(t, wasmtypes.AccessTypeAnyOfAddresses, msg.InstantiatePermission.Permission)
		require.Equal(t, []string{first.String(), second.String()}, msg.InstantiatePermission.Addresses)
	})
}

func TestParseWasmStoreCodeArgsRejectsInvalidArtifacts(t *testing.T) {
	sender := sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String()
	tests := []struct {
		name      string
		contents  []byte
		wantError string
	}{
		{name: "empty", contents: nil, wantError: "invalid input file"},
		{name: "truncated Wasm", contents: []byte{0x00}, wantError: "invalid input file"},
		{name: "unrelated file", contents: []byte("not wasm"), wantError: "invalid input file"},
		{name: "corrupt gzip", contents: []byte("\x1f\x8b\x08broken"), wantError: "invalid gzip"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "artifact")
			require.NoError(t, os.WriteFile(path, test.contents, 0o600))

			cmd := GetTxWasmStoreCodeCmd()
			_, err := ParseWasmStoreCodeArgs(path, sender, cmd.Flags())
			require.ErrorContains(t, err, test.wantError)
		})
	}

	t.Run("gzip containing non-Wasm bytes", func(t *testing.T) {
		compressed, err := ioutils.GzipIt([]byte("not wasm"))
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "not-wasm.gz")
		require.NoError(t, os.WriteFile(path, compressed, 0o600))

		cmd := GetTxWasmStoreCodeCmd()
		_, err = ParseWasmStoreCodeArgs(path, sender, cmd.Flags())
		require.ErrorContains(t, err, "decompressed payload is not wasm")
	})

	cmd := GetTxWasmStoreCodeCmd()
	_, err := ParseWasmStoreCodeArgs(
		filepath.Join(t.TempDir(), "missing.wasm"),
		sender,
		cmd.Flags(),
	)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestParseWasmVerificationFlagsRequiresMatchingCompleteProvenance(t *testing.T) {
	wasm := []byte("\x00asm\x01\x00\x00\x00")
	compressed, err := ioutils.GzipIt(wasm)
	require.NoError(t, err)
	checksum, err := wasmvm.CreateChecksum(wasm)
	require.NoError(t, err)

	newCommand := func(t *testing.T, source, builder, hash string) *cobra.Command {
		t.Helper()
		cmd := GetTxGovWasmProposalStoreAndInstantiateContractCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagSource, source))
		require.NoError(t, cmd.Flags().Set(cflags.FlagBuilder, builder))
		if hash != "" {
			require.NoError(t, cmd.Flags().Set(cflags.FlagCodeHash, hash))
		}
		return cmd
	}

	t.Run("verification omitted", func(t *testing.T) {
		cmd := newCommand(t, "", "", "")
		source, builder, hash, err := ParseWasmVerificationFlags(compressed, cmd.Flags())
		require.NoError(t, err)
		require.Empty(t, source)
		require.Empty(t, builder)
		require.Empty(t, hash)
	})

	t.Run("exact provenance", func(t *testing.T) {
		cmd := newCommand(
			t,
			"https://example.test/contracts/counter",
			"cosmwasm/workspace-optimizer:0.16.0",
			checksum.String(),
		)
		source, builder, hash, err := ParseWasmVerificationFlags(compressed, cmd.Flags())
		require.NoError(t, err)
		require.Equal(t, "https://example.test/contracts/counter", source)
		require.Equal(t, "cosmwasm/workspace-optimizer:0.16.0", builder)
		require.Equal(t, []byte(checksum), hash)
	})

	tests := []struct {
		name       string
		source     string
		builder    string
		hash       string
		compressed []byte
		wantError  string
	}{
		{
			name:       "source is required",
			builder:    "cosmwasm/workspace-optimizer:0.16.0",
			hash:       checksum.String(),
			compressed: compressed,
			wantError:  "source is required",
		},
		{
			name:       "source is a URI",
			source:     "://bad",
			builder:    "cosmwasm/workspace-optimizer:0.16.0",
			hash:       checksum.String(),
			compressed: compressed,
			wantError:  "source",
		},
		{
			name:       "builder is required",
			source:     "https://example.test/counter",
			hash:       checksum.String(),
			compressed: compressed,
			wantError:  "builder is required",
		},
		{
			name:       "builder is an image",
			source:     "https://example.test/counter",
			builder:    "INVALID IMAGE",
			hash:       checksum.String(),
			compressed: compressed,
			wantError:  "builder",
		},
		{
			name:       "code hash is required",
			source:     "https://example.test/counter",
			builder:    "cosmwasm/workspace-optimizer:0.16.0",
			compressed: compressed,
			wantError:  "code hash is required",
		},
		{
			name:       "gzip is valid",
			source:     "https://example.test/counter",
			builder:    "cosmwasm/workspace-optimizer:0.16.0",
			hash:       checksum.String(),
			compressed: []byte("\x1f\x8b\x08broken"),
			wantError:  "invalid zip",
		},
		{
			name:       "checksum matches",
			source:     "https://example.test/counter",
			builder:    "cosmwasm/workspace-optimizer:0.16.0",
			hash:       strings.Repeat("11", 32),
			compressed: compressed,
			wantError:  "code-hash mismatch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := newCommand(t, test.source, test.builder, test.hash)
			_, _, _, err := ParseWasmVerificationFlags(test.compressed, cmd.Flags())
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestParseWasmAccessConfigFlagsRequireOneUnambiguousMode(t *testing.T) {
	first := sdk.AccAddress(bytes.Repeat([]byte{6}, 20))
	second := sdk.AccAddress(bytes.Repeat([]byte{7}, 20))

	t.Run("omitted", func(t *testing.T) {
		cmd := GetTxWasmStoreCodeCmd()
		access, err := ParseWasmAccessConfigFlags(cmd.Flags())
		require.NoError(t, err)
		require.Nil(t, access)
	})

	t.Run("false does not select a mode", func(t *testing.T) {
		cmd := GetTxWasmStoreCodeCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagInstantiateByEverybody, "false"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagInstantiateNobody, "false"))
		access, err := ParseWasmAccessConfigFlags(cmd.Flags())
		require.NoError(t, err)
		require.Nil(t, access)
	})

	t.Run("everybody", func(t *testing.T) {
		cmd := GetTxWasmStoreCodeCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagInstantiateByEverybody, "true"))
		access, err := ParseWasmAccessConfigFlags(cmd.Flags())
		require.NoError(t, err)
		require.Equal(t, wasmtypes.AllowEverybody, *access)
	})

	t.Run("nobody", func(t *testing.T) {
		cmd := GetTxWasmStoreCodeCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagInstantiateNobody, "true"))
		access, err := ParseWasmAccessConfigFlags(cmd.Flags())
		require.NoError(t, err)
		require.Equal(t, wasmtypes.AllowNobody, *access)
	})

	t.Run("any-of complete addresses", func(t *testing.T) {
		cmd := GetTxWasmStoreCodeCmd()
		require.NoError(t, cmd.Flags().Set(
			cflags.FlagInstantiateByAnyOfAddress,
			first.String()+","+second.String(),
		))
		access, err := ParseWasmAccessConfigFlags(cmd.Flags())
		require.NoError(t, err)
		require.Equal(t, wasmtypes.AccessTypeAnyOfAddresses, access.Permission)
		require.Equal(t, []string{first.String(), second.String()}, access.Addresses)
	})

	t.Run("any-of address count is bounded without panic", func(t *testing.T) {
		addresses := make([]string, wasmtypes.MaxAddressCount+1)
		for index := range addresses {
			addresses[index] = sdk.AccAddress(bytes.Repeat([]byte{byte(index + 1)}, 20)).String()
		}
		cmd := GetTxWasmStoreCodeCmd()
		require.NoError(t, cmd.Flags().Set(
			cflags.FlagInstantiateByAnyOfAddress,
			strings.Join(addresses, ","),
		))
		parsed, err := cmd.Flags().GetStringSlice(cflags.FlagInstantiateByAnyOfAddress)
		require.NoError(t, err)
		require.Len(t, parsed, wasmtypes.MaxAddressCount+1)

		err = nil
		require.NotPanics(t, func() {
			_, err = ParseWasmAccessConfigFlags(cmd.Flags())
		})
		require.ErrorContains(t, err, "greater than")
	})

	tests := []struct {
		name      string
		setFlags  map[string]string
		wantError string
	}{
		{
			name: "removed single address",
			setFlags: map[string]string{
				cflags.FlagInstantiateByAddress: first.String(),
			},
			wantError: "not supported anymore",
		},
		{
			name: "malformed everybody boolean",
			setFlags: map[string]string{
				cflags.FlagInstantiateByEverybody: "sometimes",
			},
			wantError: "boolean value expected",
		},
		{
			name: "malformed nobody boolean",
			setFlags: map[string]string{
				cflags.FlagInstantiateNobody: "sometimes",
			},
			wantError: "boolean value expected",
		},
		{
			name: "conflicting booleans",
			setFlags: map[string]string{
				cflags.FlagInstantiateByEverybody: "true",
				cflags.FlagInstantiateNobody:      "true",
			},
			wantError: "mutually exclusive",
		},
		{
			name: "addresses conflict with everybody",
			setFlags: map[string]string{
				cflags.FlagInstantiateByAnyOfAddress: first.String(),
				cflags.FlagInstantiateByEverybody:    "true",
			},
			wantError: "mutually exclusive",
		},
		{
			name: "invalid address",
			setFlags: map[string]string{
				cflags.FlagInstantiateByAnyOfAddress: "not-an-address",
			},
			wantError: "parse",
		},
		{
			name: "duplicate address",
			setFlags: map[string]string{
				cflags.FlagInstantiateByAnyOfAddress: first.String() + "," + first.String(),
			},
			wantError: "duplicated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := GetTxWasmStoreCodeCmd()
			for name, value := range test.setFlags {
				require.NoError(t, cmd.Flags().Set(name, value))
			}
			_, err := ParseWasmAccessConfigFlags(cmd.Flags())
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestParseWasmAuthorizationInputsPreserveExactPermissions(t *testing.T) {
	first := sdk.AccAddress(bytes.Repeat([]byte{8}, 20))
	second := sdk.AccAddress(bytes.Repeat([]byte{9}, 20))

	for name, test := range map[string]struct {
		raw  string
		want wasmtypes.AccessConfig
	}{
		"nobody": {
			raw:  "nobody",
			want: wasmtypes.AllowNobody,
		},
		"everybody": {
			raw:  "everybody",
			want: wasmtypes.AllowEverybody,
		},
		"address set": {
			raw: first.String() + "," + second.String(),
			want: wasmtypes.AccessConfig{
				Permission: wasmtypes.AccessTypeAnyOfAddresses,
				Addresses:  []string{first.String(), second.String()},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			access, err := ParseWasmAccessConfig(test.raw)
			require.NoError(t, err)
			require.Equal(t, test.want, access)
		})
	}

	updates, err := ParseWasmAccessConfigUpdates([]string{
		"7:nobody",
		"8:" + first.String() + "," + second.String(),
	})
	require.NoError(t, err)
	require.Equal(t, []wasmtypes.AccessConfigUpdate{
		{CodeID: 7, InstantiatePermission: wasmtypes.AllowNobody},
		{
			CodeID: 8,
			InstantiatePermission: wasmtypes.AccessConfig{
				Permission: wasmtypes.AccessTypeAnyOfAddresses,
				Addresses:  []string{first.String(), second.String()},
			},
		},
	}, updates)

	grants, err := ParseStoreCodeGrants([]string{
		"checksum-a:*",
		"checksum-b:everybody",
		"*:" + first.String(),
	})
	require.NoError(t, err)
	require.Equal(t, []wasmtypes.CodeGrant{
		{CodeHash: []byte("checksum-a")},
		{
			CodeHash:              []byte("checksum-b"),
			InstantiatePermission: &wasmtypes.AccessConfig{Permission: wasmtypes.AccessTypeEverybody},
		},
		{
			CodeHash: []byte("*"),
			InstantiatePermission: &wasmtypes.AccessConfig{
				Permission: wasmtypes.AccessTypeAnyOfAddresses,
				Addresses:  []string{first.String()},
			},
		},
	}, grants)
}

func TestParseWasmAuthorizationInputsRejectMalformedValues(t *testing.T) {
	address := sdk.AccAddress(bytes.Repeat([]byte{10}, 20)).String()

	_, err := ParseWasmAccessConfig("not-an-address")
	require.ErrorContains(t, err, "unable to parse address")
	_, err = ParseWasmAccessConfig(address + "," + address)
	require.Error(t, err)

	for _, raw := range []string{"7", "7:nobody:extra"} {
		_, err = ParseWasmAccessConfigUpdates([]string{raw})
		require.ErrorContains(t, err, "invalid format")
	}
	_, err = ParseWasmAccessConfigUpdates([]string{"not-a-code-id:nobody"})
	require.ErrorContains(t, err, "invalid code ID")
	_, err = ParseWasmAccessConfigUpdates([]string{"7:not-an-address"})
	require.ErrorContains(t, err, "unable to parse address")

	for _, raw := range []string{"checksum", "checksum:*:extra"} {
		_, err = ParseStoreCodeGrants([]string{raw})
		require.ErrorContains(t, err, "invalid format")
	}
	_, err = ParseStoreCodeGrants([]string{"checksum:not-an-address"})
	require.ErrorContains(t, err, "unable to parse address")
}

func TestParseWasmExecuteArgsPreservesMessageIntent(t *testing.T) {
	sender := sdk.AccAddress(bytes.Repeat([]byte{11}, 20))
	contract := sdk.AccAddress(bytes.Repeat([]byte{12}, 32)).String()
	cmd := GetTxWasmExecuteContractCmd()
	require.NoError(t, cmd.Flags().Set(cflags.FlagAmount, "9uakt,4uact"))

	msg, err := ParseWasmExecuteArgs(contract, `{"increment":{}}`, sender, cmd.Flags())
	require.NoError(t, err)
	require.Equal(t, sender.String(), msg.Sender)
	require.Equal(t, contract, msg.Contract)
	require.JSONEq(t, `{"increment":{}}`, string(msg.Msg))
	require.Equal(
		t,
		sdk.NewCoins(sdk.NewInt64Coin("uact", 4), sdk.NewInt64Coin("uakt", 9)),
		sdk.Coins(msg.Funds),
	)

	require.NoError(t, cmd.Flags().Set(cflags.FlagAmount, "not-a-coin"))
	_, err = ParseWasmExecuteArgs(contract, `{}`, sender, cmd.Flags())
	require.Error(t, err)
}

func wasmInstantiateCommand(
	t *testing.T,
	label string,
	amount string,
	admin string,
	noAdmin bool,
) *cobra.Command {
	t.Helper()

	cmd := GetTxWasmInstantiateContractCmd()
	require.NoError(t, cmd.Flags().Set(cflags.FlagLabel, label))
	require.NoError(t, cmd.Flags().Set(cflags.FlagAmount, amount))
	require.NoError(t, cmd.Flags().Set(cflags.FlagAdmin, admin))
	require.NoError(t, cmd.Flags().Set(cflags.FlagNoAdmin, boolString(noAdmin)))
	return cmd
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func TestParseWasmInstantiateArgsPreservesImmutableContractIntent(t *testing.T) {
	sender := sdk.AccAddress(bytes.Repeat([]byte{1}, 20))
	cmd := wasmInstantiateCommand(t, "immutable-counter", "7uakt,3uact", "", true)

	msg, err := ParseWasmInstantiateArgs(
		"42",
		`{"count":1}`,
		nil,
		sender.String(),
		cmd.Flags(),
	)
	require.NoError(t, err)
	require.Equal(t, uint64(42), msg.CodeID)
	require.Equal(t, sender.String(), msg.Sender)
	require.Equal(t, "immutable-counter", msg.Label)
	require.Empty(t, msg.Admin)
	require.JSONEq(t, `{"count":1}`, string(msg.Msg))
	require.Equal(
		t,
		sdk.NewCoins(sdk.NewInt64Coin("uact", 3), sdk.NewInt64Coin("uakt", 7)),
		sdk.Coins(msg.Funds),
	)
}

func TestParseWasmInstantiateArgsResolvesFullAdminAddress(t *testing.T) {
	sender := sdk.AccAddress(bytes.Repeat([]byte{2}, 20))
	directAdmin := sdk.AccAddress(bytes.Repeat([]byte{3}, 20))

	t.Run("bech32 address", func(t *testing.T) {
		cmd := wasmInstantiateCommand(t, "direct-admin", "", directAdmin.String(), false)
		msg, err := ParseWasmInstantiateArgs(
			"7",
			`{}`,
			nil,
			sender.String(),
			cmd.Flags(),
		)
		require.NoError(t, err)
		require.Equal(t, directAdmin.String(), msg.Admin)
	})

	t.Run("keyring name", func(t *testing.T) {
		keys := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
		record, _, err := keys.NewMnemonic(
			"alice",
			sdkkeyring.English,
			"m/44'/118'/0'/0/0",
			"",
			aktkeyring.DefaultAlgo(),
		)
		require.NoError(t, err)
		admin, err := record.GetAddress()
		require.NoError(t, err)

		cmd := wasmInstantiateCommand(t, "named-admin", "1uakt", "alice", false)
		msg, err := ParseWasmInstantiateArgs(
			"8",
			`{"owner":"alice"}`,
			keys,
			sender.String(),
			cmd.Flags(),
		)
		require.NoError(t, err)
		require.Equal(t, admin.String(), msg.Admin)
		require.NotEqual(t, "alice", msg.Admin)
	})
}

func TestParseWasmInstantiateArgsRejectsAmbiguousOrMalformedIntent(t *testing.T) {
	sender := sdk.AccAddress(bytes.Repeat([]byte{4}, 20)).String()
	admin := sdk.AccAddress(bytes.Repeat([]byte{5}, 20)).String()

	tests := []struct {
		name      string
		codeID    string
		initMsg   string
		label     string
		amount    string
		admin     string
		noAdmin   bool
		keyring   sdkkeyring.Keyring
		wantError string
	}{
		{
			name:      "code ID is unsigned decimal",
			codeID:    "-1",
			initMsg:   `{}`,
			label:     "contract",
			noAdmin:   true,
			wantError: "invalid syntax",
		},
		{
			name:      "funds are SDK coins",
			codeID:    "1",
			initMsg:   `{}`,
			label:     "contract",
			amount:    "not-a-coin",
			noAdmin:   true,
			wantError: "amount",
		},
		{
			name:      "label is required",
			codeID:    "1",
			initMsg:   `{}`,
			noAdmin:   true,
			wantError: "label is required",
		},
		{
			name:      "administration mode is explicit",
			codeID:    "1",
			initMsg:   `{}`,
			label:     "contract",
			wantError: "set an admin or explicitly pass --no-admin",
		},
		{
			name:      "admin and immutable are mutually exclusive",
			codeID:    "1",
			initMsg:   `{}`,
			label:     "contract",
			admin:     admin,
			noAdmin:   true,
			wantError: "cannot both be true",
		},
		{
			name:      "unknown admin key is rejected",
			codeID:    "1",
			initMsg:   `{}`,
			label:     "contract",
			admin:     "missing-key",
			keyring:   aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec),
			wantError: "admin",
		},
		{
			name:      "initialization message is JSON",
			codeID:    "1",
			initMsg:   `{`,
			label:     "contract",
			noAdmin:   true,
			wantError: "invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := wasmInstantiateCommand(
				t,
				test.label,
				test.amount,
				test.admin,
				test.noAdmin,
			)
			_, err := ParseWasmInstantiateArgs(
				test.codeID,
				test.initMsg,
				test.keyring,
				sender,
				cmd.Flags(),
			)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
