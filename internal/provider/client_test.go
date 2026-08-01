package provider

import (
	"context"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
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
