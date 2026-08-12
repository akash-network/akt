package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	chain "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	chaintest "pkg.akt.dev/akt/internal/cli/chain/testutil"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	escrowtypes "pkg.akt.dev/go/node/escrow/v1"
)

const authzFutureExpiration = int64(4102444800)

type authzTxFixture struct {
	cctx       sdkclient.Context
	from       sdk.AccAddress
	grantee    sdk.AccAddress
	recipientA sdk.AccAddress
	recipientB sdk.AccAddress
	validatorA sdk.ValAddress
	validatorB sdk.ValAddress
}

func newAuthzTxFixture(t *testing.T) authzTxFixture {
	t.Helper()

	encoding := aktcodec.MakeEncodingConfig()
	from := sdk.AccAddress(bytes.Repeat([]byte{31}, 20))

	return authzTxFixture{
		cctx: sdkclient.Context{}.
			WithCodec(encoding.Codec).
			WithLegacyAmino(encoding.Amino).
			WithInterfaceRegistry(encoding.InterfaceRegistry).
			WithTxConfig(encoding.TxConfig).
			WithChainID("authz-semantic-test").
			WithFromAddress(from).
			WithHomeDir(t.TempDir()),
		from:       from,
		grantee:    sdk.AccAddress(bytes.Repeat([]byte{32}, 20)),
		recipientA: sdk.AccAddress(bytes.Repeat([]byte{33}, 20)),
		recipientB: sdk.AccAddress(bytes.Repeat([]byte{34}, 20)),
		validatorA: sdk.ValAddress(bytes.Repeat([]byte{35}, 20)),
		validatorB: sdk.ValAddress(bytes.Repeat([]byte{36}, 20)),
	}
}

func (fixture authzTxFixture) executeOffline(
	t *testing.T,
	cmd *cobra.Command,
	args ...string,
) ([]byte, error) {
	t.Helper()

	callArgs := append([]string{}, args...)
	callArgs = append(callArgs,
		fmt.Sprintf("--%s=%s", cflags.FlagFrom, fixture.from.String()),
		fmt.Sprintf("--%s=true", cflags.FlagGenerateOnly),
		fmt.Sprintf("--%s=true", cflags.FlagOffline),
		fmt.Sprintf("--%s=200000", cflags.FlagGas),
		fmt.Sprintf("--%s=%s", cflags.FlagChainID, fixture.cctx.ChainID),
		fmt.Sprintf("--%s=%s", cflags.FlagOutput, cflags.OutputJSON),
	)

	out, err := chaintest.ExecTestCLICmd(context.Background(), fixture.cctx, cmd, callArgs...)
	if out == nil {
		return nil, err
	}

	return append([]byte(nil), out.Bytes()...), err
}

func (fixture authzTxFixture) generateGrant(
	t *testing.T,
	cmd *cobra.Command,
	args ...string,
) (*authz.MsgGrant, authz.Authorization) {
	t.Helper()

	payload, err := fixture.executeOffline(t, cmd, args...)
	require.NoError(t, err, "command output:\n%s", payload)

	tx, err := fixture.cctx.TxConfig.TxJSONDecoder()(bytes.TrimSpace(payload))
	require.NoError(t, err, "decode generated transaction:\n%s", payload)
	require.Len(t, tx.GetMsgs(), 1)

	msg, ok := tx.GetMsgs()[0].(*authz.MsgGrant)
	require.Truef(t, ok, "message type = %T", tx.GetMsgs()[0])
	require.Equal(t, fixture.from.String(), msg.Granter)
	require.Equal(t, fixture.grantee.String(), msg.Grantee)

	msgsV2, err := tx.GetMsgsV2()
	require.NoError(t, err)
	require.Len(t, msgsV2, 1)
	signers, err := fixture.cctx.TxConfig.SigningContext().GetSigners(msgsV2[0])
	require.NoError(t, err)
	require.Equal(t, [][]byte{fixture.from}, signers)

	var authorization authz.Authorization
	require.NoError(
		t,
		fixture.cctx.InterfaceRegistry.UnpackAny(msg.Grant.Authorization, &authorization),
	)
	require.NoError(t, authorization.ValidateBasic())

	return msg, authorization
}

type authzStakingParamsServer struct {
	stakingtypes.UnimplementedQueryServer
	bondDenom string
	err       error
}

func (server *authzStakingParamsServer) Params(
	context.Context,
	*stakingtypes.QueryParamsRequest,
) (*stakingtypes.QueryParamsResponse, error) {
	if server.err != nil {
		return nil, server.err
	}
	return &stakingtypes.QueryParamsResponse{
		Params: stakingtypes.Params{BondDenom: server.bondDenom},
	}, nil
}

func newAuthzStakingConnection(t *testing.T, bondDenom string, queryErr ...error) *grpc.ClientConn {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	paramsServer := &authzStakingParamsServer{bondDenom: bondDenom}
	if len(queryErr) != 0 {
		paramsServer.err = queryErr[0]
	}
	stakingtypes.RegisterQueryServer(server, paramsServer)

	go func() {
		_ = server.Serve(listener)
	}()

	connection, err := grpc.NewClient(
		"passthrough:///authz-staking-params-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = connection.Close()
		server.Stop()
		_ = listener.Close()
	})

	return connection
}

func TestAuthzGrantGeneratesExactCoreAuthorizations(t *testing.T) {
	fixture := newAuthzTxFixture(t)

	t.Run("send", func(t *testing.T) {
		msg, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantAuthorizationCmd(),
			fixture.grantee.String(),
			"send",
			"--spend-limit=7uakt,3uact",
			fmt.Sprintf("--allow-list=%s,%s", fixture.recipientA, fixture.recipientB),
			fmt.Sprintf("--expiration=%d", authzFutureExpiration),
		)

		send, ok := authorization.(*banktypes.SendAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Equal(t, sdk.NewCoins(
			sdk.NewInt64Coin("uact", 3),
			sdk.NewInt64Coin("uakt", 7),
		), send.SpendLimit)
		require.Equal(t, []string{
			fixture.recipientA.String(),
			fixture.recipientB.String(),
		}, send.AllowList)
		require.NotNil(t, msg.Grant.Expiration)
		require.True(t, msg.Grant.Expiration.Equal(time.Unix(authzFutureExpiration, 0)))
	})

	t.Run("generic", func(t *testing.T) {
		msg, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantAuthorizationCmd(),
			fixture.grantee.String(),
			"generic",
			"--msg-type=/cosmos.gov.v1.MsgVote",
		)

		generic, ok := authorization.(*authz.GenericAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Equal(t, "/cosmos.gov.v1.MsgVote", generic.Msg)
		require.Nil(t, msg.Grant.Expiration)
	})

	t.Run("deposit", func(t *testing.T) {
		_, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantAuthorizationCmd(),
			fixture.grantee.String(),
			"deposit",
			"--scope=deployment,bid",
			"--spend-limit=19uakt,2uact",
		)

		deposit, ok := authorization.(*escrowtypes.DepositAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Equal(t, escrowtypes.DepositAuthorizationScopes{
			escrowtypes.DepositScopeDeployment,
			escrowtypes.DepositScopeBid,
		}, deposit.Scopes)
		require.Equal(t, sdk.NewCoins(
			sdk.NewInt64Coin("uact", 2),
			sdk.NewInt64Coin("uakt", 19),
		), deposit.SpendLimits)
	})
}

func TestAuthzGrantGeneratesExactStakingAuthorizations(t *testing.T) {
	fixture := newAuthzTxFixture(t)
	fixture.cctx = fixture.cctx.WithGRPCClient(newAuthzStakingConnection(t, "uakt"))

	t.Run("delegate with allow list and limit", func(t *testing.T) {
		_, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantAuthorizationCmd(),
			fixture.grantee.String(),
			"delegate",
			"--spend-limit=23uakt",
			fmt.Sprintf("--allowed-validators=%s,%s", fixture.validatorA, fixture.validatorB),
		)

		stake, ok := authorization.(*stakingtypes.StakeAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Equal(t, stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_DELEGATE, stake.AuthorizationType)
		require.Equal(t, sdk.NewInt64Coin("uakt", 23), *stake.MaxTokens)
		require.Equal(t, []string{
			fixture.validatorA.String(),
			fixture.validatorB.String(),
		}, stake.GetAllowList().Address)
		require.Nil(t, stake.GetDenyList())
	})

	t.Run("unbond with deny list", func(t *testing.T) {
		_, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantAuthorizationCmd(),
			fixture.grantee.String(),
			"unbond",
			fmt.Sprintf("--deny-validators=%s", fixture.validatorA),
		)

		stake, ok := authorization.(*stakingtypes.StakeAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Equal(t, stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_UNDELEGATE, stake.AuthorizationType)
		require.Nil(t, stake.MaxTokens)
		require.Equal(t, []string{fixture.validatorA.String()}, stake.GetDenyList().Address)
		require.Nil(t, stake.GetAllowList())
	})

	t.Run("redelegate with allow list", func(t *testing.T) {
		_, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantAuthorizationCmd(),
			fixture.grantee.String(),
			"redelegate",
			fmt.Sprintf("--allowed-validators=%s", fixture.validatorB),
		)

		stake, ok := authorization.(*stakingtypes.StakeAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Equal(t, stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_REDELEGATE, stake.AuthorizationType)
		require.Nil(t, stake.MaxTokens)
		require.Equal(t, []string{fixture.validatorB.String()}, stake.GetAllowList().Address)
		require.Nil(t, stake.GetDenyList())
	})
}

func TestAuthzGrantGeneratesExactContractAuthorizations(t *testing.T) {
	fixture := newAuthzTxFixture(t)
	contract := fixture.recipientA

	t.Run("execution max calls allow all", func(t *testing.T) {
		msg, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantContractAuthorizationCmd(),
			fixture.grantee.String(),
			"execution",
			contract.String(),
			"--max-calls=4",
			"--no-token-transfer=true",
			"--allow-all-messages=true",
			fmt.Sprintf("--expiration=%d", authzFutureExpiration),
		)

		execution, ok := authorization.(*wasmtypes.ContractExecutionAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Len(t, execution.Grants, 1)
		require.Equal(t, contract.String(), execution.Grants[0].Contract)
		limit, ok := execution.Grants[0].GetLimit().(*wasmtypes.MaxCallsLimit)
		require.Truef(t, ok, "limit type = %T", execution.Grants[0].GetLimit())
		require.Equal(t, uint64(4), limit.Remaining)
		require.IsType(t, &wasmtypes.AllowAllMessagesFilter{}, execution.Grants[0].GetFilter())
		require.True(t, msg.Grant.Expiration.Equal(time.Unix(authzFutureExpiration, 0)))
	})

	t.Run("migration max funds accepted keys", func(t *testing.T) {
		_, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantContractAuthorizationCmd(),
			fixture.grantee.String(),
			"migration",
			contract.String(),
			"--max-funds=9uakt,2uact",
			"--allow-msg-keys=update_config,transfer",
			fmt.Sprintf("--expiration=%d", authzFutureExpiration),
		)

		migration, ok := authorization.(*wasmtypes.ContractMigrationAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Len(t, migration.Grants, 1)
		limit, ok := migration.Grants[0].GetLimit().(*wasmtypes.MaxFundsLimit)
		require.Truef(t, ok, "limit type = %T", migration.Grants[0].GetLimit())
		require.Equal(t, sdk.NewCoins(
			sdk.NewInt64Coin("uact", 2),
			sdk.NewInt64Coin("uakt", 9),
		), limit.Amounts)
		filter, ok := migration.Grants[0].GetFilter().(*wasmtypes.AcceptedMessageKeysFilter)
		require.Truef(t, ok, "filter type = %T", migration.Grants[0].GetFilter())
		require.Equal(t, []string{"update_config", "transfer"}, filter.Keys)
	})

	t.Run("execution combined limit accepted raw messages", func(t *testing.T) {
		_, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantContractAuthorizationCmd(),
			fixture.grantee.String(),
			"execution",
			contract.String(),
			"--max-calls=3",
			"--max-funds=11uakt",
			`--allow-raw-msgs={},[]`,
			fmt.Sprintf("--expiration=%d", authzFutureExpiration),
		)

		execution, ok := authorization.(*wasmtypes.ContractExecutionAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Len(t, execution.Grants, 1)
		limit, ok := execution.Grants[0].GetLimit().(*wasmtypes.CombinedLimit)
		require.Truef(t, ok, "limit type = %T", execution.Grants[0].GetLimit())
		require.Equal(t, uint64(3), limit.CallsRemaining)
		require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 11)), limit.Amounts)
		filter, ok := execution.Grants[0].GetFilter().(*wasmtypes.AcceptedMessagesFilter)
		require.Truef(t, ok, "filter type = %T", execution.Grants[0].GetFilter())
		require.Equal(t, []wasmtypes.RawContractMessage{
			wasmtypes.RawContractMessage(`{}`),
			wasmtypes.RawContractMessage(`[]`),
		}, filter.Messages)
	})
}

func TestAuthzGrantGeneratesExactStoreCodeAuthorizations(t *testing.T) {
	fixture := newAuthzTxFixture(t)

	t.Run("specific checksums", func(t *testing.T) {
		msg, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantStoreCodeAuthorizationCmd(),
			fixture.grantee.String(),
			"checksum-a:everybody",
			"checksum-b:nobody",
			fmt.Sprintf("--expiration=%d", authzFutureExpiration),
		)

		storeCode, ok := authorization.(*wasmtypes.StoreCodeAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Len(t, storeCode.Grants, 2)
		require.Equal(t, []byte("checksum-a"), storeCode.Grants[0].CodeHash)
		require.Equal(t, wasmtypes.AccessTypeEverybody, storeCode.Grants[0].InstantiatePermission.Permission)
		require.Equal(t, []byte("checksum-b"), storeCode.Grants[1].CodeHash)
		require.Equal(t, wasmtypes.AccessTypeNobody, storeCode.Grants[1].InstantiatePermission.Permission)
		require.True(t, msg.Grant.Expiration.Equal(time.Unix(authzFutureExpiration, 0)))
	})

	t.Run("wildcard", func(t *testing.T) {
		msg, authorization := fixture.generateGrant(
			t,
			chain.GetTxAuthzGrantStoreCodeAuthorizationCmd(),
			fixture.grantee.String(),
			"*:*",
		)

		storeCode, ok := authorization.(*wasmtypes.StoreCodeAuthorization)
		require.Truef(t, ok, "authorization type = %T", authorization)
		require.Len(t, storeCode.Grants, 1)
		require.Equal(t, []byte("*"), storeCode.Grants[0].CodeHash)
		require.Nil(t, storeCode.Grants[0].InstantiatePermission)
		require.Nil(t, msg.Grant.Expiration)
	})
}

type authzCaptureTxClient struct {
	messages         [][]sdk.Msg
	broadcastTxCalls int
	err              error
}

func (client *authzCaptureTxClient) BroadcastMsgs(
	_ context.Context,
	messages []sdk.Msg,
	_ ...clientv1beta3.BroadcastOption,
) (interface{}, error) {
	client.messages = append(client.messages, append([]sdk.Msg(nil), messages...))
	return nil, client.err
}

func (client *authzCaptureTxClient) BroadcastTx(
	_ context.Context,
	_ sdk.Tx,
	_ ...clientv1beta3.BroadcastOption,
) (interface{}, error) {
	client.broadcastTxCalls++
	return nil, client.err
}

type authzCommandClient struct {
	cctx sdkclient.Context
	tx   clientv1beta3.TxClient
}

func (*authzCommandClient) Query() clientv1beta3.QueryClient { return nil }
func (*authzCommandClient) Node() clientv1beta3.NodeClient   { return nil }
func (client *authzCommandClient) ClientContext() sdkclient.Context {
	return client.cctx
}
func (*authzCommandClient) PrintMessage(interface{}) error { return nil }
func (*authzCommandClient) PrintJSON(interface{}) error    { return nil }
func (client *authzCommandClient) Tx() clientv1beta3.TxClient {
	return client.tx
}

func runAuthzHandler(
	t *testing.T,
	fixture authzTxFixture,
	cmd *cobra.Command,
	txClient *authzCaptureTxClient,
	args ...string,
) (string, error) {
	t.Helper()

	var output bytes.Buffer
	cctx := fixture.cctx.WithOutput(&output)
	signingContext := cctx.TxConfig.SigningContext()
	ctx := context.WithValue(
		context.Background(),
		chain.ContextTypeClient,
		&authzCommandClient{cctx: cctx, tx: txClient},
	)
	ctx = context.WithValue(ctx, chain.ContextTypeAddressCodec, signingContext.AddressCodec())
	ctx = context.WithValue(ctx, chain.ContextTypeValidatorCodec, signingContext.ValidatorAddressCodec())
	cmd.SetContext(ctx)
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	err := cmd.RunE(cmd, args)
	return output.String(), err
}

func setAuthzFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set(name, value))
}

func TestAuthzGrantRejectsInvalidOrAmbiguousAuthorizationsBeforeBroadcast(t *testing.T) {
	fixture := newAuthzTxFixture(t)
	fixture.cctx = fixture.cctx.WithGRPCClient(newAuthzStakingConnection(t, "uakt"))

	tests := []struct {
		name      string
		command   func() *cobra.Command
		args      []string
		configure func(*testing.T, *cobra.Command)
	}{
		{
			name:    "self grant",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.from.String(), "generic"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagMsgType, "/cosmos.gov.v1.MsgVote")
			},
		},
		{
			name:    "malformed grantee",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{"not-an-address", "generic"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagMsgType, "/cosmos.gov.v1.MsgVote")
			},
		},
		{
			name:    "empty generic message type",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "generic"},
		},
		{
			name:    "unknown authorization type",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "superuser"},
		},
		{
			name:    "invalid deposit scope",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "deposit"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagScope, "lease")
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "1uakt")
			},
		},
		{
			name:    "duplicate deposit scope",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "deposit"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagScope, "deployment,deployment")
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "1uakt")
			},
		},
		{
			name:    "empty deposit spend limit",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "deposit"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagScope, "deployment")
			},
		},
		{
			name:    "malformed deposit spend limit",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "deposit"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagScope, "deployment")
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "not-coins")
			},
		},
		{
			name:    "duplicate send allow list",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "send"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "1uakt")
				setAuthzFlag(t, cmd, cflags.FlagAllowList, fixture.recipientA.String()+","+fixture.recipientA.String())
			},
		},
		{
			name:    "malformed send allow list",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "send"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "1uakt")
				setAuthzFlag(t, cmd, cflags.FlagAllowList, "not-an-address")
			},
		},
		{
			name:    "zero send spend limit",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "send"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "0uakt")
			},
		},
		{
			name:    "malformed send spend limit",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "send"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "not-coins")
			},
		},
		{
			name:    "staking denom differs from bond denom",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "delegate"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "1uatom")
			},
		},
		{
			name:    "zero staking spend limit",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "delegate"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "0uakt")
			},
		},
		{
			name:    "malformed staking spend limit",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "delegate"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "not-a-coin")
			},
		},
		{
			name:    "staking allow and deny lists conflict",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "unbond"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagAllowedValidators, fixture.validatorA.String())
				setAuthzFlag(t, cmd, cflags.FlagDenyValidators, fixture.validatorB.String())
			},
		},
		{
			name:    "malformed staking validator",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "redelegate"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagAllowedValidators, "not-a-validator")
			},
		},
		{
			name:    "malformed denied staking validator",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "redelegate"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagDenyValidators, "not-a-validator")
			},
		},
		{
			name:    "contract requires expiration",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagMaxCalls, "1")
				setAuthzFlag(t, cmd, cflags.FlagNoTokenTransfer, "true")
				setAuthzFlag(t, cmd, cflags.FlagAllowAllMsgs, "true")
			},
		},
		{
			name:    "contract limit omitted",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagAllowAllMsgs, "true")
			},
		},
		{
			name:    "contract limit combines no-transfer and funds",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxCalls, "1")
				setAuthzFlag(t, cmd, cflags.FlagMaxFunds, "1uakt")
				setAuthzFlag(t, cmd, cflags.FlagNoTokenTransfer, "true")
				setAuthzFlag(t, cmd, cflags.FlagAllowAllMsgs, "true")
			},
		},
		{
			name:    "contract combined limit has malformed funds",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxCalls, "1")
				setAuthzFlag(t, cmd, cflags.FlagMaxFunds, "not-coins")
				setAuthzFlag(t, cmd, cflags.FlagAllowAllMsgs, "true")
			},
		},
		{
			name:    "contract max funds limit is malformed",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "migration", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxFunds, "not-coins")
				setAuthzFlag(t, cmd, cflags.FlagAllowedMsgKeys, "migrate")
			},
		},
		{
			name:    "contract filters conflict",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxCalls, "1")
				setAuthzFlag(t, cmd, cflags.FlagNoTokenTransfer, "true")
				setAuthzFlag(t, cmd, cflags.FlagAllowAllMsgs, "true")
				setAuthzFlag(t, cmd, cflags.FlagAllowedMsgKeys, "execute")
			},
		},
		{
			name:    "contract filter omitted",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxCalls, "1")
				setAuthzFlag(t, cmd, cflags.FlagNoTokenTransfer, "true")
			},
		},
		{
			name:    "contract message keys contain duplicate",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "migration", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxFunds, "1uakt")
				setAuthzFlag(t, cmd, cflags.FlagAllowedMsgKeys, "migrate,migrate")
			},
		},
		{
			name:    "contract raw message is invalid json",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxCalls, "1")
				setAuthzFlag(t, cmd, cflags.FlagNoTokenTransfer, "true")
				setAuthzFlag(t, cmd, cflags.FlagAllowedRawMsgs, `{not-json}`)
			},
		},
		{
			name:    "unsupported contract authorization type",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "instantiate", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxCalls, "1")
				setAuthzFlag(t, cmd, cflags.FlagNoTokenTransfer, "true")
				setAuthzFlag(t, cmd, cflags.FlagAllowAllMsgs, "true")
			},
		},
		{
			name:    "malformed contract address",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", "not-a-contract"},
		},
		{
			name:    "malformed contract grantee",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{"not-a-grantee", "execution", fixture.recipientA.String()},
		},
		{
			name:    "store code duplicate checksum",
			command: chain.GetTxAuthzGrantStoreCodeAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "checksum:everybody", "CHECKSUM:nobody"},
		},
		{
			name:    "store code wildcard mixed with checksum",
			command: chain.GetTxAuthzGrantStoreCodeAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "*:*", "checksum:everybody"},
		},
		{
			name:    "store code empty checksum",
			command: chain.GetTxAuthzGrantStoreCodeAuthorizationCmd,
			args:    []string{fixture.grantee.String(), ":everybody"},
		},
		{
			name:    "store code invalid format",
			command: chain.GetTxAuthzGrantStoreCodeAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "checksum"},
		},
		{
			name:    "store code invalid permission",
			command: chain.GetTxAuthzGrantStoreCodeAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "checksum:not-a-permission"},
		},
		{
			name:    "store code malformed grantee",
			command: chain.GetTxAuthzGrantStoreCodeAuthorizationCmd,
			args:    []string{"not-an-address", "checksum:everybody"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.command()
			if test.configure != nil {
				test.configure(t, cmd)
			}
			txClient := &authzCaptureTxClient{err: errors.New("unexpected authz broadcast")}

			output, err := runAuthzHandler(t, fixture, cmd, txClient, test.args...)

			require.Error(t, err)
			require.NotErrorIs(t, err, txClient.err)
			require.Empty(t, txClient.messages, "rejected input reached BroadcastMsgs")
			require.Zero(t, txClient.broadcastTxCalls, "rejected input reached BroadcastTx")
			require.Empty(t, output, "rejected input wrote transaction output")
		})
	}
}

func TestAuthzStakingGrantPropagatesParamsFailureBeforeBroadcast(t *testing.T) {
	fixture := newAuthzTxFixture(t)
	queryErr := errors.New("staking params unavailable")
	fixture.cctx = fixture.cctx.WithGRPCClient(newAuthzStakingConnection(t, "uakt", queryErr))
	cmd := chain.GetTxAuthzGrantAuthorizationCmd()
	setAuthzFlag(t, cmd, cflags.FlagSpendLimit, "1uakt")
	setAuthzFlag(t, cmd, cflags.FlagAllowedValidators, fixture.validatorA.String())
	txClient := &authzCaptureTxClient{err: errors.New("unexpected authz broadcast")}

	output, err := runAuthzHandler(
		t,
		fixture,
		cmd,
		txClient,
		fixture.grantee.String(),
		"delegate",
	)

	require.ErrorContains(t, err, queryErr.Error())
	require.Empty(t, txClient.messages)
	require.Zero(t, txClient.broadcastTxCalls)
	require.Empty(t, output)
}

func TestAuthzGrantPropagatesBroadcastFailureAfterExactMessageConstruction(t *testing.T) {
	fixture := newAuthzTxFixture(t)
	tests := []struct {
		name      string
		command   func() *cobra.Command
		args      []string
		configure func(*testing.T, *cobra.Command)
		wantType  interface{}
	}{
		{
			name:    "generic",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "generic"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagMsgType, "/cosmos.gov.v1.MsgVote")
			},
			wantType: &authz.GenericAuthorization{},
		},
		{
			name:    "contract",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "execution", fixture.recipientA.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				setAuthzFlag(t, cmd, cflags.FlagExpiration, fmt.Sprint(authzFutureExpiration))
				setAuthzFlag(t, cmd, cflags.FlagMaxCalls, "1")
				setAuthzFlag(t, cmd, cflags.FlagNoTokenTransfer, "true")
				setAuthzFlag(t, cmd, cflags.FlagAllowAllMsgs, "true")
			},
			wantType: &wasmtypes.ContractExecutionAuthorization{},
		},
		{
			name:     "store code",
			command:  chain.GetTxAuthzGrantStoreCodeAuthorizationCmd,
			args:     []string{fixture.grantee.String(), "checksum:everybody"},
			wantType: &wasmtypes.StoreCodeAuthorization{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.command()
			if test.configure != nil {
				test.configure(t, cmd)
			}
			broadcastErr := errors.New("transaction transport failed")
			txClient := &authzCaptureTxClient{err: broadcastErr}

			output, err := runAuthzHandler(t, fixture, cmd, txClient, test.args...)

			require.ErrorIs(t, err, broadcastErr)
			require.Empty(t, output)
			require.Zero(t, txClient.broadcastTxCalls)
			require.Len(t, txClient.messages, 1)
			require.Len(t, txClient.messages[0], 1)
			grant, ok := txClient.messages[0][0].(*authz.MsgGrant)
			require.Truef(t, ok, "broadcast message type = %T", txClient.messages[0][0])
			require.Equal(t, fixture.from.String(), grant.Granter)
			require.Equal(t, fixture.grantee.String(), grant.Grantee)
			authorization, unpackErr := grant.GetAuthorization()
			require.NoError(t, unpackErr)
			require.IsType(t, test.wantType, authorization)
			require.NoError(t, authorization.ValidateBasic())
		})
	}
}

func TestAuthzGrantRejectedAuthorizationsEmitNoGeneratedTransaction(t *testing.T) {
	fixture := newAuthzTxFixture(t)
	tests := []struct {
		name    string
		command func() *cobra.Command
		args    []string
	}{
		{
			name:    "generic without message type",
			command: chain.GetTxAuthzGrantAuthorizationCmd,
			args:    []string{fixture.grantee.String(), "generic"},
		},
		{
			name:    "contract with invalid raw message",
			command: chain.GetTxAuthzGrantContractAuthorizationCmd,
			args: []string{
				fixture.grantee.String(),
				"execution",
				fixture.recipientA.String(),
				"--max-calls=1",
				"--no-token-transfer=true",
				`--allow-raw-msgs={not-json}`,
				fmt.Sprintf("--expiration=%d", authzFutureExpiration),
			},
		},
		{
			name:    "store code with duplicate checksum",
			command: chain.GetTxAuthzGrantStoreCodeAuthorizationCmd,
			args: []string{
				fixture.grantee.String(),
				"checksum:everybody",
				"CHECKSUM:nobody",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := fixture.executeOffline(t, test.command(), test.args...)
			require.Error(t, err)
			require.NotContains(t, string(payload), `"body":`, "rejected input emitted a transaction document")
			require.NotContains(t, string(payload), "/cosmos.authz.v1beta1.MsgGrant")
		})
	}
}
