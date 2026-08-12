package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"

	rpcclientmock "github.com/cometbft/cometbft/rpc/client/mock"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	querytypes "github.com/cosmos/cosmos-sdk/types/query"

	ibctransfer "github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	ibccore "github.com/cosmos/ibc-go/v10/modules/core"
	ibcclienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v10/modules/core/03-connection/types"
	channeltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	clioutput "pkg.akt.dev/akt/internal/output"
)

type stubIBCClientQueryServer struct {
	ibcclienttypes.UnimplementedQueryServer
	err error
}

type blockQueryRPC struct {
	rpcclientmock.Client

	block               *coretypes.ResultBlock
	blockResults        *coretypes.ResultBlockResults
	blockResultsContext context.Context
}

func (client *blockQueryRPC) Block(context.Context, *int64) (*coretypes.ResultBlock, error) {
	return client.block, nil
}

func (client *blockQueryRPC) BlockResults(ctx context.Context, _ *int64) (*coretypes.ResultBlockResults, error) {
	client.blockResultsContext = ctx
	return client.blockResults, nil
}

func (server *stubIBCClientQueryServer) ClientParams(context.Context, *ibcclienttypes.QueryClientParamsRequest) (*ibcclienttypes.QueryClientParamsResponse, error) {
	if server.err != nil {
		return nil, server.err
	}

	return &ibcclienttypes.QueryClientParamsResponse{
		Params: &ibcclienttypes.Params{AllowedClients: []string{"07-tendermint"}},
	}, nil
}

func (*stubIBCClientQueryServer) ClientStates(context.Context, *ibcclienttypes.QueryClientStatesRequest) (*ibcclienttypes.QueryClientStatesResponse, error) {
	return &ibcclienttypes.QueryClientStatesResponse{
		ClientStates: []ibcclienttypes.IdentifiedClientState{
			{ClientId: "07-tendermint-0"},
			{ClientId: "07-tendermint-1"},
		},
		Pagination: &querytypes.PageResponse{NextKey: []byte("next"), Total: 2},
	}, nil
}

func (*stubIBCClientQueryServer) ConsensusStateHeights(context.Context, *ibcclienttypes.QueryConsensusStateHeightsRequest) (*ibcclienttypes.QueryConsensusStateHeightsResponse, error) {
	return &ibcclienttypes.QueryConsensusStateHeightsResponse{
		ConsensusStateHeights: []ibcclienttypes.Height{
			{RevisionHeight: 1},
			{RevisionHeight: 2},
			{RevisionHeight: 3},
			{RevisionHeight: 4},
		},
		Pagination: &querytypes.PageResponse{NextKey: []byte("next"), Total: 4},
	}, nil
}

func (*stubIBCClientQueryServer) ConsensusStates(context.Context, *ibcclienttypes.QueryConsensusStatesRequest) (*ibcclienttypes.QueryConsensusStatesResponse, error) {
	return &ibcclienttypes.QueryConsensusStatesResponse{
		ConsensusStates: []ibcclienttypes.ConsensusStateWithHeight{
			{Height: ibcclienttypes.Height{RevisionHeight: 1}},
			{Height: ibcclienttypes.Height{RevisionHeight: 2}},
		},
		Pagination: &querytypes.PageResponse{NextKey: []byte("next"), Total: 2},
	}, nil
}

type stubIBCConnectionQueryServer struct {
	connectiontypes.UnimplementedQueryServer
	err error
}

func (server *stubIBCConnectionQueryServer) ConnectionParams(context.Context, *connectiontypes.QueryConnectionParamsRequest) (*connectiontypes.QueryConnectionParamsResponse, error) {
	if server.err != nil {
		return nil, server.err
	}

	return &connectiontypes.QueryConnectionParamsResponse{
		Params: &connectiontypes.Params{MaxExpectedTimePerBlock: 30_000_000_000},
	}, nil
}

type stubIBCChannelQueryServer struct {
	channeltypes.UnimplementedQueryServer
}

func (*stubIBCChannelQueryServer) ConnectionChannels(context.Context, *channeltypes.QueryConnectionChannelsRequest) (*channeltypes.QueryConnectionChannelsResponse, error) {
	return &channeltypes.QueryConnectionChannelsResponse{
		Channels: []*channeltypes.IdentifiedChannel{
			{ChannelId: "channel-0", PortId: "transfer"},
			{ChannelId: "channel-1", PortId: "transfer"},
			{ChannelId: "channel-2", PortId: "transfer"},
		},
		Pagination: &querytypes.PageResponse{NextKey: []byte("next"), Total: 3},
		Height:     ibcclienttypes.Height{RevisionNumber: 2, RevisionHeight: 10},
	}, nil
}

func newIBCQueryTestConn(
	t *testing.T,
	clientServer ibcclienttypes.QueryServer,
	connectionServer connectiontypes.QueryServer,
	channelServer channeltypes.QueryServer,
) *grpc.ClientConn {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	ibcclienttypes.RegisterQueryServer(server, clientServer)
	connectiontypes.RegisterQueryServer(server, connectionServer)
	channeltypes.RegisterQueryServer(server, channelServer)

	go func() {
		_ = server.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///ibc-query-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})

	return conn
}

func queryTestClientContext(out *bytes.Buffer) sdkclient.Context {
	return sdkclient.Context{
		ChainID: "akashnet-2",
		Codec:   codec.NewProtoCodec(codectypes.NewInterfaceRegistry()),
		Output:  out,
	}
}

func executeQueryTestCommand(
	t *testing.T,
	cmd *cobra.Command,
	cctx sdkclient.Context,
	queryClient sdkclient.Context,
	args ...string,
) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cctx = cctx.WithOutput(&out)
	queryClient = queryClient.WithOutput(&out)

	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
	if queryClient.Codec != nil {
		cl := &stubLightClient{q: &stubQueryClient{}, cctx: queryClient}
		ctx = context.WithValue(ctx, ContextTypeQueryClient, cl)
	}

	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), err
}

func executeVendoredIBCCommand(t *testing.T, root *cobra.Command, conn *grpc.ClientConn, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cctx := queryTestClientContext(&out).WithGRPCClient(conn)
	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)

	root.SetContext(ctx)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)

	err := root.Execute()
	return out.String(), err
}

func TestVendoredIBCParamsReturnTransportErrorsWithoutPanicking(t *testing.T) {
	transportErr := errors.New("test transport unavailable")
	conn := newIBCQueryTestConn(
		t,
		&stubIBCClientQueryServer{err: transportErr},
		&stubIBCConnectionQueryServer{err: transportErr},
		&stubIBCChannelQueryServer{},
	)

	cases := []struct {
		name string
		root func() *cobra.Command
		args []string
	}{
		{
			name: "client params",
			root: func() *cobra.Command {
				return adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd())
			},
			args: []string{"client", "params"},
		},
		{
			name: "connection params",
			root: func() *cobra.Command {
				return adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd())
			},
			args: []string{"connection", "params"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() {
				_, err = executeVendoredIBCCommand(t, tc.root(), conn, tc.args...)
			})
			require.ErrorContains(t, err, "test transport unavailable")
		})
	}
}

func TestVendoredIBCYAMLEmitsYAML(t *testing.T) {
	conn := newIBCQueryTestConn(
		t,
		&stubIBCClientQueryServer{},
		&stubIBCConnectionQueryServer{},
		&stubIBCChannelQueryServer{},
	)
	root := adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd())
	var constrainOutput func(*cobra.Command)
	constrainOutput = func(cmd *cobra.Command) {
		clioutput.ConstrainFlag(cmd.LocalFlags().Lookup(cflags.FlagOutput), cflags.OutputPretty, cflags.OutputPretty, cflags.OutputJSON, cflags.OutputYAML)
		for _, child := range cmd.Commands() {
			constrainOutput(child)
		}
	}
	constrainOutput(root)

	out, err := executeVendoredIBCCommand(
		t,
		root,
		conn,
		"client", "params", "--output", "yaml",
	)
	require.NoError(t, err)
	require.Contains(t, out, "allowed_clients:")
	require.False(t, strings.HasPrefix(strings.TrimSpace(out), "{"), "YAML mode emitted JSON: %s", out)
}

func TestVendoredIBCVerboseReportsSelectedEndpoint(t *testing.T) {
	conn := newIBCQueryTestConn(
		t,
		&stubIBCClientQueryServer{},
		&stubIBCConnectionQueryServer{},
		&stubIBCChannelQueryServer{},
	)
	root := adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd())
	root.PersistentFlags().CountP("verbose", "v", "verbose")

	var out bytes.Buffer
	cctx := queryTestClientContext(&out).
		WithGRPCClient(conn).
		WithNodeURI("https://rpc.test.invalid")
	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
	root.SetContext(ctx)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs([]string{"-v", "--node", "https://rpc.test.invalid", "client", "params"})

	require.NoError(t, root.Execute())
	require.Contains(t, out.String(), "querying https://rpc.test.invalid (chain akashnet-2)")
	require.Equal(t, 1, strings.Count(out.String(), "querying https://rpc.test.invalid"))
}

func TestAlternateQueryPreRunsHonorVerbose(t *testing.T) {
	t.Run("local derivation", func(t *testing.T) {
		cmd := GetQueryModuleNameToAddressCmd()
		cmd.Flags().CountP("verbose", "v", "verbose")

		var out bytes.Buffer
		cctx := queryTestClientContext(&out)
		ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
		cmd.SetContext(ctx)
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"gov", "-v"})

		require.NoError(t, cmd.Execute())
		require.Contains(t, out.String(), "querying locally (chain akashnet-2)")
	})

	for _, tc := range []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{"blocks", QueryBlocksCmd, []string{"block.height > 1"}},
		{"block", QueryBlockCmd, []string{"1"}},
		{"block results", QueryBlockResultsCmd, []string{"1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.cmd()
			cmd.Flags().CountP("verbose", "v", "verbose")
			require.NoError(t, cmd.Flags().Set("verbose", "1"))
			require.NoError(t, cmd.Flags().Set(cflags.FlagNode, "https://rpc.test.invalid"))

			var stderr bytes.Buffer
			cctx := queryTestClientContext(&stderr).WithNodeURI("https://rpc.test.invalid")
			ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
			cmd.SetContext(ctx)
			cmd.SetErr(&stderr)

			require.NoError(t, cmd.PreRunE(cmd, tc.args))
			require.Contains(t, stderr.String(), "querying https://rpc.test.invalid (chain akashnet-2)")
		})
	}

}

func TestVendoredIBCPaginationAppliesHardLimit(t *testing.T) {
	conn := newIBCQueryTestConn(
		t,
		&stubIBCClientQueryServer{},
		&stubIBCConnectionQueryServer{},
		&stubIBCChannelQueryServer{},
	)

	t.Run("height offset excludes prefix and lookahead", func(t *testing.T) {
		out, err := executeVendoredIBCCommand(
			t,
			adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd()),
			conn,
			"client", "consensus-state-heights", "07-tendermint-0",
			"--offset", "2", "--limit", "1", "--output", "json",
		)
		require.NoError(t, err)

		var response struct {
			Heights    []map[string]string `json:"consensus_state_heights"`
			Pagination struct {
				NextKey string `json:"next_key"`
				Total   string `json:"total"`
			} `json:"pagination"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &response))
		require.Len(t, response.Heights, 1)
		require.Equal(t, "3", response.Heights[0]["revision_height"])
		require.Equal(t, "bmV4dA==", response.Pagination.NextKey)
		require.Equal(t, "4", response.Pagination.Total)
	})

	t.Run("height offset excludes prefix on a short final page", func(t *testing.T) {
		out, err := executeVendoredIBCCommand(
			t,
			adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd()),
			conn,
			"client", "consensus-state-heights", "07-tendermint-0",
			"--offset", "2", "--limit", "5", "--output", "json",
		)
		require.NoError(t, err)

		var response struct {
			Heights []map[string]string `json:"consensus_state_heights"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &response))
		require.Len(t, response.Heights, 2)
		require.Equal(t, "3", response.Heights[0]["revision_height"])
		require.Equal(t, "4", response.Heights[1]["revision_height"])
	})

	t.Run("consensus states excludes lookahead", func(t *testing.T) {
		out, err := executeVendoredIBCCommand(
			t,
			adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd()),
			conn,
			"client", "consensus-states", "07-tendermint-0",
			"--limit", "1", "--output", "json",
		)
		require.NoError(t, err)

		var response struct {
			States     []json.RawMessage `json:"consensus_states"`
			Pagination struct {
				NextKey string `json:"next_key"`
				Total   string `json:"total"`
			} `json:"pagination"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &response))
		require.Len(t, response.States, 1)
		require.Equal(t, "bmV4dA==", response.Pagination.NextKey)
		require.Equal(t, "2", response.Pagination.Total)
	})

	t.Run("client states excludes lookahead", func(t *testing.T) {
		out, err := executeVendoredIBCCommand(
			t,
			adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd()),
			conn,
			"client", "states", "--limit", "1", "--output", "json",
		)
		require.NoError(t, err)

		var response struct {
			States     []json.RawMessage `json:"client_states"`
			Pagination struct {
				NextKey string `json:"next_key"`
				Total   string `json:"total"`
			} `json:"pagination"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &response))
		require.Len(t, response.States, 1)
		require.Equal(t, "bmV4dA==", response.Pagination.NextKey)
		require.Equal(t, "2", response.Pagination.Total)
	})

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"connection channels excludes offset prefix", []string{"--offset", "1"}},
		{"connection channels applies page offset", []string{"--page", "2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"channel", "connections", "connection-2", "--limit", "1", "--output", "json"}
			args = append(args, tc.args...)
			out, err := executeVendoredIBCCommand(
				t,
				adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd()),
				conn,
				args...,
			)
			require.NoError(t, err)

			var response struct {
				Channels []struct {
					ChannelID string `json:"channel_id"`
				} `json:"channels"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &response))
			require.Len(t, response.Channels, 1)
			require.Equal(t, "channel-1", response.Channels[0].ChannelID)
		})
	}

	t.Run("connection channels beyond final page is empty", func(t *testing.T) {
		out, err := executeVendoredIBCCommand(
			t,
			adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd()),
			conn,
			"channel", "connections", "connection-2",
			"--offset", "3", "--limit", "1", "--output", "json",
		)
		require.NoError(t, err)

		var response struct {
			Channels []json.RawMessage `json:"channels"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &response))
		require.Empty(t, response.Channels)
	})
}

func TestVendoredQueryTreeHasUniqueSiblingNames(t *testing.T) {
	root := adoptVendoredQueryCmd(ibccore.AppModuleBasic{}.GetQueryCmd())

	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		seen := make(map[string]struct{})
		for _, child := range cmd.Commands() {
			name := child.Name()
			if name != "" {
				_, exists := seen[name]
				require.Falsef(t, exists, "duplicate child %q under %q", name, cmd.CommandPath())
				seen[name] = struct{}{}
			}
			walk(child)
		}
	}

	walk(root)
}

func TestQueryNodeFlagOverridesContextEndpoint(t *testing.T) {
	t.Run("explicit endpoint wins", func(t *testing.T) {
		cmd := &cobra.Command{Use: "query"}
		cflags.AddQueryFlagsToCmd(cmd)
		require.NoError(t, cmd.Flags().Set(cflags.FlagNode, "http://127.0.0.1:29999"))

		var out bytes.Buffer
		cctx := queryTestClientContext(&out)
		ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
		ctx = context.WithValue(ctx, ContextTypeRPCURI, "http://127.0.0.1:29998")
		cmd.SetContext(ctx)

		got, err := GetClientQueryContext(cmd)
		require.NoError(t, err)
		require.Equal(t, "http://127.0.0.1:29999", got.NodeURI)
	})

	t.Run("empty endpoint is refused", func(t *testing.T) {
		cmd := &cobra.Command{Use: "query"}
		cflags.AddQueryFlagsToCmd(cmd)
		require.NoError(t, cmd.Flags().Set(cflags.FlagNode, ""))

		var out bytes.Buffer
		cctx := queryTestClientContext(&out)
		ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
		ctx = context.WithValue(ctx, ContextTypeRPCURI, "http://127.0.0.1:29998")
		cmd.SetContext(ctx)

		_, err := GetClientQueryContext(cmd)
		require.ErrorContains(t, err, "--node")
	})
}

func TestLocalQueryLeavesRejectTransportAndSnapshotFlags(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		flag string
	}{
		{
			name: "module address node",
			cmd:  GetQueryModuleNameToAddressCmd,
			args: []string{"gov", "--node", "http://127.0.0.1:29999"},
			flag: cflags.FlagNode,
		},
		{
			name: "module address height",
			cmd:  GetQueryModuleNameToAddressCmd,
			args: []string{"gov", "--height", "10"},
			flag: cflags.FlagHeight,
		},
		{
			name: "transfer escrow node",
			cmd: func() *cobra.Command {
				return adoptVendoredQueryCmd(ibctransfer.AppModuleBasic{}.GetQueryCmd())
			},
			args: []string{"escrow-address", "transfer", "channel-0", "--node", "http://127.0.0.1:29999"},
			flag: cflags.FlagNode,
		},
		{
			name: "transfer escrow height",
			cmd: func() *cobra.Command {
				return adoptVendoredQueryCmd(ibctransfer.AppModuleBasic{}.GetQueryCmd())
			},
			args: []string{"escrow-address", "transfer", "channel-0", "--height", "10"},
			flag: cflags.FlagHeight,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cctx := queryTestClientContext(&out)
			_, err := executeQueryTestCommand(t, tc.cmd(), cctx, sdkclient.Context{}, tc.args...)
			require.ErrorContains(t, err, "--"+tc.flag)
		})
	}
}

func TestQueriesRejectUnsupportedOrConflictingHeight(t *testing.T) {
	tests := []struct {
		name        string
		cmd         func() *cobra.Command
		args        []string
		queryClient bool
		want        string
	}{
		{
			name: "block positional and flag",
			cmd:  QueryBlockCmd,
			args: []string{"not-a-height", "--type", "height", "--height", "10"},
			want: "both",
		},
		{
			name: "block results positional and flag",
			cmd:  QueryBlockResultsCmd,
			args: []string{"not-a-height", "--height", "10"},
			want: "both",
		},
		{
			name: "blocks",
			cmd:  QueryBlocksCmd,
			args: []string{"--height", "10"},
			want: "--height",
		},
		{
			name: "gov proposer",
			cmd:  GetQueryGovProposerCmd,
			args: []string{"1", "--height", "10"},
			want: "--height",
		},
		{
			name:        "tx",
			cmd:         GetQueryAuthTxCmd,
			args:        []string{"not-hex", "--height", "10"},
			queryClient: true,
			want:        "--height",
		},
		{
			name:        "txs",
			cmd:         GetQueryAuthTxsByEventsCmd,
			args:        []string{"not-an-event", "--height", "10"},
			queryClient: true,
			want:        "--height",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cctx := queryTestClientContext(&out)
			queryClient := sdkclient.Context{}
			if tc.queryClient {
				queryClient = cctx
			}

			_, err := executeQueryTestCommand(t, tc.cmd(), cctx, queryClient, tc.args...)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestQueryBlockWithoutPositionalHeightHandlesMissingBlock(t *testing.T) {
	var out bytes.Buffer
	rpc := &blockQueryRPC{
		Client: rpcclientmock.New(),
		block: &coretypes.ResultBlock{
			Block: &cmttypes.Block{Header: cmttypes.Header{Height: 0}},
		},
	}
	cctx := queryTestClientContext(&out).WithClient(rpc)
	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)

	cmd := QueryBlockCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--height", "17"})

	var err error
	require.NotPanics(t, func() {
		err = cmd.Execute()
	})
	require.ErrorContains(t, err, "no block found with height 17")
}

func TestQueryBlockResultsPropagatesCommandContext(t *testing.T) {
	type contextKey struct{}

	var out bytes.Buffer
	rpc := &blockQueryRPC{
		Client:       rpcclientmock.New(),
		blockResults: &coretypes.ResultBlockResults{Height: 23},
	}
	cctx := queryTestClientContext(&out).WithClient(rpc)
	marker := &struct{}{}
	ctx := context.WithValue(context.Background(), contextKey{}, marker)
	ctx = context.WithValue(ctx, ClientContextKey, &cctx)

	cmd := QueryBlockResultsCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"23"})

	require.NoError(t, cmd.Execute())
	require.Same(t, marker, rpc.blockResultsContext.Value(contextKey{}))
}

func TestLocalModuleAddressValidatesChainID(t *testing.T) {
	root := &cobra.Command{Use: "query"}
	root.PersistentFlags().String(cflags.FlagChainID, "", "chain ID")
	root.AddCommand(GetQueryModuleNameToAddressCmd())

	var out bytes.Buffer
	cctx := queryTestClientContext(&out)
	_, err := executeQueryTestCommand(
		t,
		root,
		cctx,
		sdkclient.Context{},
		"module-name-to-address", "gov", "--chain-id", "not-akashnet-2",
	)
	require.ErrorContains(t, err, "does not match")
}

func TestScalarQueriesHonorStructuredOutput(t *testing.T) {
	t.Run("module address json", func(t *testing.T) {
		var buffer bytes.Buffer
		cctx := queryTestClientContext(&buffer)
		out, err := executeQueryTestCommand(
			t,
			GetQueryModuleNameToAddressCmd(),
			cctx,
			sdkclient.Context{},
			"gov", "--output", "json",
		)
		require.NoError(t, err)

		var address string
		require.NoError(t, json.Unmarshal([]byte(out), &address))
		require.True(t, strings.HasPrefix(address, "akash1"))
	})

	t.Run("transfer escrow json", func(t *testing.T) {
		var buffer bytes.Buffer
		cctx := queryTestClientContext(&buffer)
		out, err := executeQueryTestCommand(
			t,
			adoptVendoredQueryCmd(ibctransfer.AppModuleBasic{}.GetQueryCmd()),
			cctx,
			sdkclient.Context{},
			"escrow-address", "transfer", "channel-0", "--output", "json",
		)
		require.NoError(t, err)

		var address string
		require.NoError(t, json.Unmarshal([]byte(out), &address))
		require.True(t, strings.HasPrefix(address, "akash1"))
	})

	t.Run("module address yaml", func(t *testing.T) {
		var buffer bytes.Buffer
		cctx := queryTestClientContext(&buffer)
		out, err := executeQueryTestCommand(
			t,
			GetQueryModuleNameToAddressCmd(),
			cctx,
			sdkclient.Context{},
			"gov", "--output", "yaml",
		)
		require.NoError(t, err)
		require.Contains(t, out, "akash1")
		require.False(t, strings.HasPrefix(strings.TrimSpace(out), "{"))
	})

	t.Run("transfer escrow yaml", func(t *testing.T) {
		var buffer bytes.Buffer
		cctx := queryTestClientContext(&buffer)
		out, err := executeQueryTestCommand(
			t,
			adoptVendoredQueryCmd(ibctransfer.AppModuleBasic{}.GetQueryCmd()),
			cctx,
			sdkclient.Context{},
			"escrow-address", "transfer", "channel-0", "--output", "yaml",
		)
		require.NoError(t, err)
		require.Contains(t, out, "akash1")
		require.False(t, strings.HasPrefix(strings.TrimSpace(out), "{"))
	})
}

func TestWasmCodeRejectsStructuredStdoutBeforeQuery(t *testing.T) {
	for _, format := range []string{cflags.OutputJSON, cflags.OutputYAML} {
		t.Run(format, func(t *testing.T) {
			cmd := GetQueryWasmCodeCmd()
			require.NoError(t, cmd.Flags().Set(cflags.FlagOutput, format))

			var out bytes.Buffer
			cctx := queryTestClientContext(&out)
			ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
			ctx = context.WithValue(ctx, ContextTypeQueryClient, &stubLightClient{
				q:    &stubQueryClient{},
				cctx: cctx,
			})
			cmd.SetContext(ctx)

			require.NotNil(t, cmd.PersistentPreRunE)
			err := cmd.PersistentPreRunE(cmd, []string{"1", "contract.wasm"})
			require.ErrorContains(t, err, "--output")
		})
	}
}
