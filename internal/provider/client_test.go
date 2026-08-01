package provider

import (
	"context"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestGatewayClientRequiresKeyring(t *testing.T) {
	addr := sdk.AccAddress([]byte("authenticated-owner"))
	cctx := sdkclient.Context{}.WithFromAddress(addr)

	_, err := NewGatewayClient(
		context.Background(),
		cctx,
		addr,
		"https://provider.example.com:8443",
		"jwt",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "requires a keyring") {
		t.Fatalf("error = %v, want a keyring remedy", err)
	}
}

func TestGatewayClientRequiresSelectedKey(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	_, _, err := kr.NewMnemonic(
		"different-key",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create unrelated key: %v", err)
	}

	addr := sdk.AccAddress([]byte("missing-signing-key"))
	for _, authType := range []string{"jwt", "mtls"} {
		t.Run(authType, func(t *testing.T) {
			_, err := NewGatewayClient(
				context.Background(),
				sdkclient.Context{}.WithFromAddress(addr),
				addr,
				"https://provider.example.com:8443",
				authType,
				kr,
			)
			if err == nil || !strings.Contains(err.Error(), "selected account") ||
				!strings.Contains(err.Error(), "configured keyring") {
				t.Fatalf("error = %v, want a provider signing-key remedy", err)
			}
		})
	}
}
