package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	rpcclientmock "github.com/cometbft/cometbft/rpc/client/mock"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	flagdefs "pkg.akt.dev/akt/internal/flags"
)

type authQueryRPC struct {
	rpcclientmock.Client
	err     error
	query   string
	page    int
	perPage int
}

func (rpc *authQueryRPC) TxSearch(
	_ context.Context,
	query string,
	_ bool,
	page,
	perPage *int,
	_ string,
) (*coretypes.ResultTxSearch, error) {
	rpc.query = query
	rpc.page = *page
	rpc.perPage = *perPage
	return nil, rpc.err
}

func TestAuthQueriesReadCanonicalSearchFlags(t *testing.T) {
	wantErr := errors.New("stop after tx search")
	rpc := &authQueryRPC{err: wantErr}
	encoding := aktcodec.MakeEncodingConfig()
	var output bytes.Buffer
	cctx := sdkclient.Context{}.
		WithCodec(encoding.Codec).
		WithLegacyAmino(encoding.Amino).
		WithInterfaceRegistry(encoding.InterfaceRegistry).
		WithTxConfig(encoding.TxConfig).
		WithClient(rpc).
		WithOutput(&output)
	client := &semanticQueryLightClient{cctx: cctx}

	cmd := GetQueryAuthTxsByEventsCmd()
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagPage, "4"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagLimit, "9"))
	ctx := context.WithValue(context.Background(), ContextTypeQueryClient, client)
	cmd.SetContext(ctx)
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	err := cmd.RunE(cmd, []string{"message.sender=akash1sender"})
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "message.sender='akash1sender'", rpc.query)
	require.Equal(t, 4, rpc.page)
	require.Equal(t, 9, rpc.perPage)

	unknown := GetQueryAuthTxCmd()
	require.NoError(t, unknown.Flags().Set(flagdefs.FlagType, "unknown"))
	unknown.SetContext(ctx)
	require.ErrorContains(t, unknown.RunE(unknown, []string{"value"}), "unknown --"+flagdefs.FlagType)
}
