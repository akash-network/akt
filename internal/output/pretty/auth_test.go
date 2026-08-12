package pretty

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/codec"
)

func TestFormatAccountResponsePreservesFullIdentityAndAccountMetadata(t *testing.T) {
	t.Parallel()

	encoding := codec.MakeEncodingConfig()
	privateKey := secp256k1.GenPrivKey()
	address := sdk.AccAddress(privateKey.PubKey().Address())
	base := authtypes.NewBaseAccount(address, privateKey.PubKey(), 42, 7)
	account, err := codectypes.NewAnyWithValue(base)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = formatAccountResponse(
		&output,
		&cobra.Command{},
		sdkclient.Context{}.WithInterfaceRegistry(encoding.InterfaceRegistry),
		&authtypes.QueryAccountResponse{Account: account},
	)
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(output.String())
	for _, want := range []string{address.String(), "42", "7", "secp256k1.PubKey"} {
		if !strings.Contains(plain, want) {
			t.Errorf("account output does not contain %q:\n%s", want, plain)
		}
	}
}

func TestFormatAccountResponseCoversEmptyFallbackAndModuleAccounts(t *testing.T) {
	t.Parallel()

	encoding := codec.MakeEncodingConfig()
	cctx := sdkclient.Context{}.WithInterfaceRegistry(encoding.InterfaceRegistry)
	command := &cobra.Command{}

	t.Run("empty", func(t *testing.T) {
		var output bytes.Buffer
		if err := formatAccountResponse(&output, command, cctx, &authtypes.QueryAccountResponse{}); err != nil {
			t.Fatal(err)
		}
		if plain := ansi.Strip(output.String()); !strings.Contains(plain, "(no account)") {
			t.Fatalf("empty account output = %q", plain)
		}
	})

	t.Run("unknown type falls back", func(t *testing.T) {
		const typeURL = "/unknown.account.v1.Account"
		var output bytes.Buffer
		if err := formatAccountResponse(&output, command, cctx, &authtypes.QueryAccountResponse{
			Account: &codectypes.Any{TypeUrl: typeURL, Value: []byte{1}},
		}); err != nil {
			t.Fatal(err)
		}
		plain := ansi.Strip(output.String())
		if !strings.Contains(plain, "Account") || !strings.Contains(plain, typeURL) {
			t.Fatalf("fallback output = %q", plain)
		}
	})

	t.Run("module", func(t *testing.T) {
		module := authtypes.NewEmptyModuleAccount("distribution", authtypes.Burner, authtypes.Staking)
		account, err := codectypes.NewAnyWithValue(module)
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := formatAccountResponse(&output, command, cctx, &authtypes.QueryAccountResponse{Account: account}); err != nil {
			t.Fatal(err)
		}
		plain := ansi.Strip(output.String())
		for _, want := range []string{module.GetAddress().String(), "distribution", authtypes.Burner, authtypes.Staking} {
			if !strings.Contains(plain, want) {
				t.Errorf("module account output does not contain %q:\n%s", want, plain)
			}
		}
	})
}

func TestAccountCollectionsRenderValidAndUndecodableEntries(t *testing.T) {
	t.Parallel()

	encoding := codec.MakeEncodingConfig()
	cctx := sdkclient.Context{}.WithInterfaceRegistry(encoding.InterfaceRegistry)
	command := &cobra.Command{}
	privateKey := secp256k1.GenPrivKey()
	base := authtypes.NewBaseAccount(sdk.AccAddress(privateKey.PubKey().Address()), privateKey.PubKey(), 9, 11)
	baseAny, err := codectypes.NewAnyWithValue(base)
	if err != nil {
		t.Fatal(err)
	}
	module := authtypes.NewEmptyModuleAccount("gov", authtypes.Burner)
	moduleAny, err := codectypes.NewAnyWithValue(module)
	if err != nil {
		t.Fatal(err)
	}
	unknown := &codectypes.Any{TypeUrl: "/unknown.v1.Account", Value: []byte{1}}

	var accountsOutput bytes.Buffer
	if err := formatAccountsResponse(&accountsOutput, command, cctx, &authtypes.QueryAccountsResponse{
		Accounts: []*codectypes.Any{baseAny, unknown},
	}); err != nil {
		t.Fatal(err)
	}
	accountsPlain := ansi.Strip(accountsOutput.String())
	for _, want := range []string{base.GetAddress().String(), "9", "11", "?", "Account"} {
		if !strings.Contains(accountsPlain, want) {
			t.Errorf("accounts output does not contain %q:\n%s", want, accountsPlain)
		}
	}

	var modulesOutput bytes.Buffer
	if err := formatModuleAccountsResponse(&modulesOutput, command, cctx, &authtypes.QueryModuleAccountsResponse{
		Accounts: []*codectypes.Any{moduleAny, baseAny, unknown},
	}); err != nil {
		t.Fatal(err)
	}
	modulesPlain := ansi.Strip(modulesOutput.String())
	for _, want := range []string{"gov", module.GetAddress().String(), authtypes.Burner, base.GetAddress().String(), "Account"} {
		if !strings.Contains(modulesPlain, want) {
			t.Errorf("module accounts output does not contain %q:\n%s", want, modulesPlain)
		}
	}
}

func TestUnpackAccountRequiresRegistryAndRegisteredType(t *testing.T) {
	t.Parallel()

	if _, err := unpackAccount(nil, &codectypes.Any{}); err == nil || !strings.Contains(err.Error(), "no interface registry") {
		t.Fatalf("nil registry error = %v", err)
	}
	encoding := codec.MakeEncodingConfig()
	if _, err := unpackAccount(encoding.InterfaceRegistry, &codectypes.Any{TypeUrl: "/unknown.v1.Account"}); err == nil {
		t.Fatal("unknown account type unexpectedly unpacked")
	}
}
