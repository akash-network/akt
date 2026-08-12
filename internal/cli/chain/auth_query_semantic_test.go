package cli

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type semanticAuthQuery struct {
	authtypes.QueryClient

	method       string
	request      interface{}
	err          error
	nilResponse  bool
	missingField bool
	account      *types.Any
}

func (query *semanticAuthQuery) result(method string, request interface{}) bool {
	query.method = method
	query.request = request
	return query.nilResponse
}

func (query *semanticAuthQuery) Params(
	_ context.Context,
	request *authtypes.QueryParamsRequest,
	_ ...grpc.CallOption,
) (*authtypes.QueryParamsResponse, error) {
	if query.result("params", request) || query.err != nil {
		return nil, query.err
	}
	return &authtypes.QueryParamsResponse{}, nil
}

func (query *semanticAuthQuery) Account(
	_ context.Context,
	request *authtypes.QueryAccountRequest,
	_ ...grpc.CallOption,
) (*authtypes.QueryAccountResponse, error) {
	if query.result("account", request) || query.err != nil {
		return nil, query.err
	}
	response := &authtypes.QueryAccountResponse{Account: query.account}
	if query.missingField {
		response.Account = nil
	}
	return response, nil
}

func (query *semanticAuthQuery) AccountAddressByID(
	_ context.Context,
	request *authtypes.QueryAccountAddressByIDRequest,
	_ ...grpc.CallOption,
) (*authtypes.QueryAccountAddressByIDResponse, error) {
	if query.result("address-by-acc-num", request) || query.err != nil {
		return nil, query.err
	}
	return &authtypes.QueryAccountAddressByIDResponse{AccountAddress: stateTestOwner}, nil
}

func (query *semanticAuthQuery) Accounts(
	_ context.Context,
	request *authtypes.QueryAccountsRequest,
	_ ...grpc.CallOption,
) (*authtypes.QueryAccountsResponse, error) {
	if query.result("accounts", request) || query.err != nil {
		return nil, query.err
	}
	return &authtypes.QueryAccountsResponse{Accounts: []*types.Any{query.account}}, nil
}

func (query *semanticAuthQuery) ModuleAccounts(
	_ context.Context,
	request *authtypes.QueryModuleAccountsRequest,
	_ ...grpc.CallOption,
) (*authtypes.QueryModuleAccountsResponse, error) {
	if query.result("module-accounts", request) || query.err != nil {
		return nil, query.err
	}
	return &authtypes.QueryModuleAccountsResponse{Accounts: []*types.Any{query.account}}, nil
}

func (query *semanticAuthQuery) ModuleAccountByName(
	_ context.Context,
	request *authtypes.QueryModuleAccountByNameRequest,
	_ ...grpc.CallOption,
) (*authtypes.QueryModuleAccountByNameResponse, error) {
	if query.result("module-account", request) || query.err != nil {
		return nil, query.err
	}
	response := &authtypes.QueryModuleAccountByNameResponse{Account: query.account}
	if query.missingField {
		response.Account = nil
	}
	return response, nil
}

type semanticAuthAggregate struct {
	clientv1beta3.QueryClient
	auth authtypes.QueryClient
}

func (query *semanticAuthAggregate) Auth() authtypes.QueryClient { return query.auth }

func newSemanticAuthQuery(t *testing.T) *semanticAuthQuery {
	t.Helper()

	account, err := types.NewAnyWithValue(&authtypes.BaseAccount{Address: stateTestOwner})
	require.NoError(t, err)
	return &semanticAuthQuery{account: account}
}

func runAuthSemanticQuery(t *testing.T, commandFactory func() *cobra.Command, query *semanticAuthQuery, output io.Writer, args ...string) error {
	t.Helper()
	return runSemanticQuery(t, commandFactory(), &semanticAuthAggregate{auth: query}, nil, output, args...)
}

func TestAuthQueryRequestsPreserveInputs(t *testing.T) {
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
		flags          map[string]string
		method         string
		assertRequest  func(*testing.T, interface{})
	}{
		{
			name: "params", commandFactory: GetQueryAuthParamsCmd, method: "params",
			assertRequest: func(t *testing.T, request interface{}) { require.Equal(t, &authtypes.QueryParamsRequest{}, request) },
		},
		{
			name: "account", commandFactory: GetQueryAuthAccountCmd, args: []string{stateTestOwner}, method: "account",
			assertRequest: func(t *testing.T, request interface{}) {
				require.Equal(t, stateTestOwner, request.(*authtypes.QueryAccountRequest).Address)
			},
		},
		{
			name: "address by account number", commandFactory: GetQueryAuthAccountAddressByIDCmd, args: []string{"42"}, method: "address-by-acc-num",
			assertRequest: func(t *testing.T, request interface{}) {
				require.Equal(t, uint64(42), request.(*authtypes.QueryAccountAddressByIDRequest).AccountId)
			},
		},
		{
			name: "accounts pagination", commandFactory: GetQueryAuthAccountsCmd, method: "accounts",
			flags: map[string]string{"page": "3", "limit": "7", "count-total": "true", "reverse": "true"},
			assertRequest: func(t *testing.T, request interface{}) {
				require.Equal(t, &sdkquery.PageRequest{Offset: 14, Limit: 7, CountTotal: true, Reverse: true}, request.(*authtypes.QueryAccountsRequest).Pagination)
			},
		},
		{
			name: "module accounts", commandFactory: GetQueryAuthModuleAccountsCmd, method: "module-accounts",
			assertRequest: func(t *testing.T, request interface{}) {
				require.Equal(t, &authtypes.QueryModuleAccountsRequest{}, request)
			},
		},
		{
			name: "module account", commandFactory: GetQueryAuthModuleAccountByNameCmd, args: []string{"distribution"}, method: "module-account",
			assertRequest: func(t *testing.T, request interface{}) {
				require.Equal(t, "distribution", request.(*authtypes.QueryModuleAccountByNameRequest).Name)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := newSemanticAuthQuery(t)
			cmd := test.commandFactory()
			for name, value := range test.flags {
				require.NoError(t, cmd.Flags().Set(name, value))
			}

			err := runSemanticQuery(t, cmd, &semanticAuthAggregate{auth: query}, nil, io.Discard, test.args...)
			require.NoError(t, err)
			require.Equal(t, test.method, query.method)
			test.assertRequest(t, query.request)
		})
	}
}

func TestAuthQueryRejectsInvalidInputsBeforeTransport(t *testing.T) {
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
		flags          map[string]string
	}{
		{name: "account address", commandFactory: GetQueryAuthAccountCmd, args: []string{"not-an-address"}},
		{name: "account number", commandFactory: GetQueryAuthAccountAddressByIDCmd, args: []string{"18446744073709551616"}},
		{name: "blank module name", commandFactory: GetQueryAuthModuleAccountByNameCmd, args: []string{""}},
		{name: "page and offset", commandFactory: GetQueryAuthAccountsCmd, flags: map[string]string{"page": "2", "offset": "1"}},
		{name: "invalid page key", commandFactory: GetQueryAuthAccountsCmd, flags: map[string]string{"page-key": "%%%"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := newSemanticAuthQuery(t)
			cmd := test.commandFactory()
			for name, value := range test.flags {
				require.NoError(t, cmd.Flags().Set(name, value))
			}

			err := runSemanticQuery(t, cmd, &semanticAuthAggregate{auth: query}, nil, io.Discard, test.args...)
			require.Error(t, err)
			require.Empty(t, query.method)
		})
	}
}

func TestAuthQueryRejectsNilSuccessfulResponses(t *testing.T) {
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
	}{
		{name: "params", commandFactory: GetQueryAuthParamsCmd},
		{name: "account", commandFactory: GetQueryAuthAccountCmd, args: []string{stateTestOwner}},
		{name: "address", commandFactory: GetQueryAuthAccountAddressByIDCmd, args: []string{"7"}},
		{name: "accounts", commandFactory: GetQueryAuthAccountsCmd},
		{name: "module accounts", commandFactory: GetQueryAuthModuleAccountsCmd},
		{name: "module account", commandFactory: GetQueryAuthModuleAccountByNameCmd, args: []string{"auth"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := newSemanticAuthQuery(t)
			query.nilResponse = true
			var err error
			require.NotPanics(t, func() {
				err = runAuthSemanticQuery(t, test.commandFactory, query, io.Discard, test.args...)
			})
			require.ErrorContains(t, err, "malformed node response")
		})
	}
}

func TestAuthQueryRejectsMissingRequiredAccount(t *testing.T) {
	for _, test := range []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
	}{
		{name: "account", commandFactory: GetQueryAuthAccountCmd, args: []string{stateTestOwner}},
		{name: "module account", commandFactory: GetQueryAuthModuleAccountByNameCmd, args: []string{"auth"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := newSemanticAuthQuery(t)
			query.missingField = true
			err := runAuthSemanticQuery(t, test.commandFactory, query, io.Discard, test.args...)
			require.ErrorContains(t, err, "malformed node response")
			require.ErrorContains(t, err, "missing account")
		})
	}
}

func TestAuthQueryPreservesTransportAndOutputErrors(t *testing.T) {
	t.Run("transport cause", func(t *testing.T) {
		cause := errors.New("auth transport failed")
		query := newSemanticAuthQuery(t)
		query.err = cause
		err := runAuthSemanticQuery(t, GetQueryAuthParamsCmd, query, io.Discard)
		require.ErrorIs(t, err, cause)
	})

	t.Run("short output", func(t *testing.T) {
		query := newSemanticAuthQuery(t)
		err := runAuthSemanticQuery(t, GetQueryAuthParamsCmd, query, outputBoundaryShortWriter{})
		require.ErrorIs(t, err, io.ErrShortWrite)
	})
}

func TestAuthQueryCommandRegistersLeaves(t *testing.T) {
	cmd := GetQueryAuthCmd()
	for _, name := range []string{"account", "address-by-acc-num", "accounts", "params", "module-accounts", "module-account"} {
		child, _, err := cmd.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, child.Name())
	}
}
