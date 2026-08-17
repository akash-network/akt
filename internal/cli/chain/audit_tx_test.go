package cli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	audittypes "pkg.akt.dev/go/node/audit/v1"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	providertypes "pkg.akt.dev/go/node/provider/v1beta4"
	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"
)

type auditProviderQueryServer struct {
	providertypes.UnimplementedQueryServer

	mu       sync.Mutex
	response *providertypes.QueryProviderResponse
	err      error
	owner    string
}

func (server *auditProviderQueryServer) Provider(_ context.Context, req *providertypes.QueryProviderRequest) (*providertypes.QueryProviderResponse, error) {
	server.mu.Lock()
	server.owner = req.Owner
	server.mu.Unlock()

	return server.response, server.err
}

func (server *auditProviderQueryServer) requestedOwner() string {
	server.mu.Lock()
	defer server.mu.Unlock()

	return server.owner
}

type auditCaptureTxClient struct {
	response interface{}
	err      error
	messages [][]sdk.Msg
}

func (client *auditCaptureTxClient) BroadcastMsgs(_ context.Context, messages []sdk.Msg, _ ...clientv1beta3.BroadcastOption) (interface{}, error) {
	client.messages = append(client.messages, append([]sdk.Msg(nil), messages...))
	return client.response, client.err
}

func (client *auditCaptureTxClient) BroadcastTx(_ context.Context, _ sdk.Tx, _ ...clientv1beta3.BroadcastOption) (interface{}, error) {
	return client.response, client.err
}

type auditCommandClient struct {
	cctx sdkclient.Context
	tx   clientv1beta3.TxClient
}

func (*auditCommandClient) Query() clientv1beta3.QueryClient { return nil }

func (*auditCommandClient) Node() clientv1beta3.NodeClient { return nil }

func (client *auditCommandClient) ClientContext() sdkclient.Context { return client.cctx }

func (*auditCommandClient) PrintMessage(interface{}) error { return nil }

func (*auditCommandClient) PrintJSON(interface{}) error { return nil }

func (client *auditCommandClient) Tx() clientv1beta3.TxClient { return client.tx }

func newAuditProviderQueryContext(t *testing.T, server providertypes.QueryServer) sdkclient.Context {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	providertypes.RegisterQueryServer(grpcServer, server)

	go func() {
		_ = grpcServer.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///audit-provider-query-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		grpcServer.Stop()
		_ = listener.Close()
	})

	return sdkclient.Context{}.WithGRPCClient(conn)
}

func auditTestAddresses(t *testing.T) (string, string, sdk.AccAddress) {
	t.Helper()

	auditor := "akash1qqqsyqcyq5rqwzqfpg9scrgwpugpzysnwduagr"
	provider := "akash1v3jkvemgd94xkmrddehhqutjwd682anh9zw2p2"
	auditorAddress, err := sdk.AccAddressFromBech32(auditor)
	require.NoError(t, err)

	return auditor, provider, auditorAddress
}

func auditTestCommandContext() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

func runAuditCommand(t *testing.T, cmd *cobra.Command, client *auditCommandClient, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd.SetContext(context.WithValue(context.Background(), ContextTypeClient, client))
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.RunE(cmd, args)
	return out.String(), err
}

func TestReadAttributesFromArgumentsSortsAndPreservesValues(t *testing.T) {
	attributes, err := readAttributes(
		&cobra.Command{},
		sdkclient.Context{},
		"unused-provider",
		[]string{"region", "us-west", "Region", "case-sensitive", "gpu", "a100"},
	)
	require.NoError(t, err)
	require.Equal(t, attrtypes.Attributes{
		{Key: "Region", Value: "case-sensitive"},
		{Key: "gpu", Value: "a100"},
		{Key: "region", Value: "us-west"},
	}, attributes)
}

func TestReadAttributesRejectsNonAdjacentDuplicateKeys(t *testing.T) {
	attributes, err := readAttributes(
		&cobra.Command{},
		sdkclient.Context{},
		"unused-provider",
		[]string{"region", "us-west", "gpu", "a100", "region", "eu-west"},
	)
	require.Nil(t, attributes)
	require.EqualError(t, err, "supplied attributes with duplicate keys")
}

func TestReadAttributesQueriesProviderAndSortsResponse(t *testing.T) {
	_, provider, _ := auditTestAddresses(t)
	server := &auditProviderQueryServer{
		response: &providertypes.QueryProviderResponse{
			Provider: providertypes.Provider{Attributes: attrtypes.Attributes{
				{Key: "region", Value: "us-west"},
				{Key: "gpu", Value: "a100"},
			}},
		},
	}
	cctx := newAuditProviderQueryContext(t, server)

	attributes, err := readAttributes(auditTestCommandContext(), cctx, provider, nil)
	require.NoError(t, err)
	require.Equal(t, provider, server.requestedOwner())
	require.Equal(t, attrtypes.Attributes{
		{Key: "gpu", Value: "a100"},
		{Key: "region", Value: "us-west"},
	}, attributes)
}

func TestReadAttributesRejectsDuplicateKeysFromProviderQuery(t *testing.T) {
	_, provider, _ := auditTestAddresses(t)
	server := &auditProviderQueryServer{
		response: &providertypes.QueryProviderResponse{
			Provider: providertypes.Provider{Attributes: attrtypes.Attributes{
				{Key: "region", Value: "us-west"},
				{Key: "gpu", Value: "a100"},
				{Key: "region", Value: "eu-west"},
			}},
		},
	}

	attributes, err := readAttributes(auditTestCommandContext(), newAuditProviderQueryContext(t, server), provider, nil)
	require.Nil(t, attributes)
	require.EqualError(t, err, "supplied attributes with duplicate keys")
}

func TestReadAttributesPreservesProviderQueryErrors(t *testing.T) {
	_, provider, _ := auditTestAddresses(t)
	server := &auditProviderQueryServer{err: status.Error(codes.Unavailable, "provider query unavailable")}

	attributes, err := readAttributes(auditTestCommandContext(), newAuditProviderQueryContext(t, server), provider, nil)
	require.Nil(t, attributes)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.ErrorContains(t, err, "provider query unavailable")
}

func TestReadKeysSortsExactKeysAndRejectsDuplicates(t *testing.T) {
	t.Run("sorts while treating case as significant", func(t *testing.T) {
		keys, err := readKeys([]string{"region", "Region", "gpu"})
		require.NoError(t, err)
		require.Equal(t, []string{"Region", "gpu", "region"}, keys)
	})

	t.Run("rejects a non-adjacent duplicate", func(t *testing.T) {
		keys, err := readKeys([]string{"region", "gpu", "region"})
		require.Nil(t, keys)
		require.EqualError(t, err, "supplied attributes with duplicate keys")
	})
}

func TestAuditAttributeCommandsRequireProviderAndPairs(t *testing.T) {
	create := CmdCreateProviderAttributes()
	require.Error(t, create.Args(create, nil))

	deleteCommand := CmdDeleteProviderAttributes()
	require.Error(t, deleteCommand.Args(deleteCommand, nil))

	_, provider, _ := auditTestAddresses(t)
	var err error
	require.NotPanics(t, func() {
		err = create.RunE(create, []string{provider, "region"})
	})
	require.EqualError(t, err, "attributes must be provided as pairs")
}

func TestCreateProviderAttributesRejectsInvalidInputBeforeBroadcast(t *testing.T) {
	_, provider, auditorAddress := auditTestAddresses(t)

	tests := []struct {
		name    string
		args    []string
		context sdkclient.Context
		wantErr string
	}{
		{
			name:    "invalid provider address",
			args:    []string{"not-an-address", "region", "us-west"},
			context: sdkclient.Context{FromAddress: auditorAddress},
			wantErr: "decoding bech32 failed",
		},
		{
			name:    "duplicate attribute key",
			args:    []string{provider, "region", "us-west", "gpu", "a100", "region", "eu-west"},
			context: sdkclient.Context{FromAddress: auditorAddress},
			wantErr: "supplied attributes with duplicate keys",
		},
		{
			name:    "invalid auditor address",
			args:    []string{provider, "region", "us-west"},
			context: sdkclient.Context{},
			wantErr: "Invalid Auditor Address",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := &auditCaptureTxClient{response: &sdk.TxResponse{TxHash: "UNEXPECTED"}}
			client := &auditCommandClient{cctx: test.context, tx: tx}

			_, err := runAuditCommand(t, CmdCreateProviderAttributes(), client, test.args...)
			require.ErrorContains(t, err, test.wantErr)
			require.Empty(t, tx.messages, "invalid input must not reach broadcast")
		})
	}
}

func TestCreateProviderAttributesBuildsSortedSemanticMessage(t *testing.T) {
	auditor, provider, auditorAddress := auditTestAddresses(t)
	tx := &auditCaptureTxClient{response: &sdk.TxResponse{TxHash: "CREATE123"}}
	client := &auditCommandClient{
		cctx: sdkclient.Context{FromAddress: auditorAddress},
		tx:   tx,
	}

	output, err := runAuditCommand(
		t,
		CmdCreateProviderAttributes(),
		client,
		provider,
		"region", "us-west",
		"gpu", "a100",
	)
	require.NoError(t, err)
	require.Contains(t, output, "CREATE123")
	require.Len(t, tx.messages, 1)
	require.Len(t, tx.messages[0], 1)

	message, ok := tx.messages[0][0].(*audittypes.MsgSignProviderAttributes)
	require.True(t, ok)
	require.Equal(t, auditor, message.Auditor)
	require.Equal(t, provider, message.Owner)
	require.Equal(t, attrtypes.Attributes{
		{Key: "gpu", Value: "a100"},
		{Key: "region", Value: "us-west"},
	}, message.Attributes)
}

func TestCreateProviderAttributesUsesLiveProviderAttributes(t *testing.T) {
	auditor, provider, auditorAddress := auditTestAddresses(t)
	server := &auditProviderQueryServer{
		response: &providertypes.QueryProviderResponse{
			Provider: providertypes.Provider{Attributes: attrtypes.Attributes{
				{Key: "region", Value: "us-west"},
				{Key: "gpu", Value: "a100"},
			}},
		},
	}
	tx := &auditCaptureTxClient{response: &sdk.TxResponse{TxHash: "QUERY123"}}
	cctx := newAuditProviderQueryContext(t, server)
	cctx = cctx.WithFromAddress(auditorAddress)
	client := &auditCommandClient{cctx: cctx, tx: tx}

	_, err := runAuditCommand(t, CmdCreateProviderAttributes(), client, provider)
	require.NoError(t, err)
	require.Len(t, tx.messages, 1)

	message, ok := tx.messages[0][0].(*audittypes.MsgSignProviderAttributes)
	require.True(t, ok)
	require.Equal(t, auditor, message.Auditor)
	require.Equal(t, provider, message.Owner)
	require.Equal(t, attrtypes.Attributes{
		{Key: "gpu", Value: "a100"},
		{Key: "region", Value: "us-west"},
	}, message.Attributes)
}

func TestCreateProviderAttributesRejectsEmptyProviderQuery(t *testing.T) {
	_, provider, auditorAddress := auditTestAddresses(t)
	server := &auditProviderQueryServer{response: &providertypes.QueryProviderResponse{}}
	tx := &auditCaptureTxClient{response: &sdk.TxResponse{TxHash: "UNEXPECTED"}}
	cctx := newAuditProviderQueryContext(t, server).WithFromAddress(auditorAddress)

	_, err := runAuditCommand(t, CmdCreateProviderAttributes(), &auditCommandClient{cctx: cctx, tx: tx}, provider)
	require.EqualError(t, err, "no attributes provided|found")
	require.Empty(t, tx.messages)
}

func TestCreateProviderAttributesPreservesBroadcastError(t *testing.T) {
	_, provider, auditorAddress := auditTestAddresses(t)
	broadcastErr := errors.New("broadcast rejected")
	tx := &auditCaptureTxClient{err: broadcastErr}
	client := &auditCommandClient{cctx: sdkclient.Context{FromAddress: auditorAddress}, tx: tx}

	_, err := runAuditCommand(t, CmdCreateProviderAttributes(), client, provider, "region", "us-west")
	require.ErrorIs(t, err, broadcastErr)
	require.Len(t, tx.messages, 1)
}

func TestDeleteProviderAttributesRejectsInvalidInputBeforeBroadcast(t *testing.T) {
	_, provider, auditorAddress := auditTestAddresses(t)

	tests := []struct {
		name    string
		args    []string
		context sdkclient.Context
		wantErr string
	}{
		{
			name:    "invalid provider address",
			args:    []string{"not-an-address", "region"},
			context: sdkclient.Context{FromAddress: auditorAddress},
			wantErr: "decoding bech32 failed",
		},
		{
			name:    "duplicate attribute key",
			args:    []string{provider, "region", "gpu", "region"},
			context: sdkclient.Context{FromAddress: auditorAddress},
			wantErr: "supplied attributes with duplicate keys",
		},
		{
			name:    "invalid auditor address",
			args:    []string{provider, "region"},
			context: sdkclient.Context{},
			wantErr: "Invalid Auditor Address",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := &auditCaptureTxClient{response: &sdk.TxResponse{TxHash: "UNEXPECTED"}}
			client := &auditCommandClient{cctx: test.context, tx: tx}

			_, err := runAuditCommand(t, CmdDeleteProviderAttributes(), client, test.args...)
			require.ErrorContains(t, err, test.wantErr)
			require.Empty(t, tx.messages, "invalid input must not reach broadcast")
		})
	}
}

func TestDeleteProviderAttributesBuildsSortedSemanticMessage(t *testing.T) {
	auditor, provider, auditorAddress := auditTestAddresses(t)
	tx := &auditCaptureTxClient{response: &sdk.TxResponse{TxHash: "DELETE123"}}
	client := &auditCommandClient{
		cctx: sdkclient.Context{FromAddress: auditorAddress},
		tx:   tx,
	}

	output, err := runAuditCommand(t, CmdDeleteProviderAttributes(), client, provider, "region", "Region", "gpu")
	require.NoError(t, err)
	require.Contains(t, output, "DELETE123")
	require.Len(t, tx.messages, 1)
	require.Len(t, tx.messages[0], 1)

	message, ok := tx.messages[0][0].(*audittypes.MsgDeleteProviderAttributes)
	require.True(t, ok)
	require.Equal(t, auditor, message.Auditor)
	require.Equal(t, provider, message.Owner)
	require.Equal(t, []string{"Region", "gpu", "region"}, message.Keys)
}

func TestDeleteProviderAttributesAllowsDeletingAllProviderAttributes(t *testing.T) {
	auditor, provider, auditorAddress := auditTestAddresses(t)
	tx := &auditCaptureTxClient{response: &sdk.TxResponse{TxHash: "DELETEALL123"}}
	client := &auditCommandClient{cctx: sdkclient.Context{FromAddress: auditorAddress}, tx: tx}

	_, err := runAuditCommand(t, CmdDeleteProviderAttributes(), client, provider)
	require.NoError(t, err)
	require.Len(t, tx.messages, 1)

	message, ok := tx.messages[0][0].(*audittypes.MsgDeleteProviderAttributes)
	require.True(t, ok)
	require.Equal(t, auditor, message.Auditor)
	require.Equal(t, provider, message.Owner)
	require.Empty(t, message.Keys)
}

func TestDeleteProviderAttributesPreservesBroadcastError(t *testing.T) {
	_, provider, auditorAddress := auditTestAddresses(t)
	broadcastErr := errors.New("broadcast rejected")
	tx := &auditCaptureTxClient{err: broadcastErr}
	client := &auditCommandClient{cctx: sdkclient.Context{FromAddress: auditorAddress}, tx: tx}

	_, err := runAuditCommand(t, CmdDeleteProviderAttributes(), client, provider, "region")
	require.ErrorIs(t, err, broadcastErr)
	require.Len(t, tx.messages, 1)
}
