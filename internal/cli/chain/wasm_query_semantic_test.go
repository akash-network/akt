package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type semanticWasmQuery struct {
	wasmtypes.QueryClient

	method      string
	request     interface{}
	err         error
	nilResponse bool
	emptyCode   bool
}

func (query *semanticWasmQuery) result(method string, request interface{}) bool {
	query.method = method
	query.request = request
	return query.nilResponse
}

func (query *semanticWasmQuery) Codes(_ context.Context, request *wasmtypes.QueryCodesRequest, _ ...grpc.CallOption) (*wasmtypes.QueryCodesResponse, error) {
	if query.result("codes", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryCodesResponse{}, nil
}

func (query *semanticWasmQuery) ContractsByCode(_ context.Context, request *wasmtypes.QueryContractsByCodeRequest, _ ...grpc.CallOption) (*wasmtypes.QueryContractsByCodeResponse, error) {
	if query.result("contracts-by-code", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryContractsByCodeResponse{}, nil
}

func (query *semanticWasmQuery) Code(_ context.Context, request *wasmtypes.QueryCodeRequest, _ ...grpc.CallOption) (*wasmtypes.QueryCodeResponse, error) {
	if query.result("code", request) || query.err != nil {
		return nil, query.err
	}
	if query.emptyCode {
		return &wasmtypes.QueryCodeResponse{}, nil
	}
	return &wasmtypes.QueryCodeResponse{Data: []byte("wasm-bytecode")}, nil
}

func (query *semanticWasmQuery) CodeInfo(_ context.Context, request *wasmtypes.QueryCodeInfoRequest, _ ...grpc.CallOption) (*wasmtypes.QueryCodeInfoResponse, error) {
	if query.result("code-info", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryCodeInfoResponse{}, nil
}

func (query *semanticWasmQuery) ContractInfo(_ context.Context, request *wasmtypes.QueryContractInfoRequest, _ ...grpc.CallOption) (*wasmtypes.QueryContractInfoResponse, error) {
	if query.result("contract", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryContractInfoResponse{}, nil
}

func (query *semanticWasmQuery) AllContractState(_ context.Context, request *wasmtypes.QueryAllContractStateRequest, _ ...grpc.CallOption) (*wasmtypes.QueryAllContractStateResponse, error) {
	if query.result("all-state", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryAllContractStateResponse{}, nil
}

func (query *semanticWasmQuery) RawContractState(_ context.Context, request *wasmtypes.QueryRawContractStateRequest, _ ...grpc.CallOption) (*wasmtypes.QueryRawContractStateResponse, error) {
	if query.result("raw-state", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryRawContractStateResponse{}, nil
}

func (query *semanticWasmQuery) SmartContractState(_ context.Context, request *wasmtypes.QuerySmartContractStateRequest, _ ...grpc.CallOption) (*wasmtypes.QuerySmartContractStateResponse, error) {
	if query.result("smart-state", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QuerySmartContractStateResponse{Data: wasmtypes.RawContractMessage(`{"ok":true}`)}, nil
}

func (query *semanticWasmQuery) ContractHistory(_ context.Context, request *wasmtypes.QueryContractHistoryRequest, _ ...grpc.CallOption) (*wasmtypes.QueryContractHistoryResponse, error) {
	if query.result("history", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryContractHistoryResponse{}, nil
}

func (query *semanticWasmQuery) PinnedCodes(_ context.Context, request *wasmtypes.QueryPinnedCodesRequest, _ ...grpc.CallOption) (*wasmtypes.QueryPinnedCodesResponse, error) {
	if query.result("pinned", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryPinnedCodesResponse{}, nil
}

func (query *semanticWasmQuery) ContractsByCreator(_ context.Context, request *wasmtypes.QueryContractsByCreatorRequest, _ ...grpc.CallOption) (*wasmtypes.QueryContractsByCreatorResponse, error) {
	if query.result("contracts-by-creator", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryContractsByCreatorResponse{}, nil
}

func (query *semanticWasmQuery) Params(_ context.Context, request *wasmtypes.QueryParamsRequest, _ ...grpc.CallOption) (*wasmtypes.QueryParamsResponse, error) {
	if query.result("params", request) || query.err != nil {
		return nil, query.err
	}
	return &wasmtypes.QueryParamsResponse{}, nil
}

type semanticWasmAggregate struct {
	clientv1beta3.QueryClient
	wasm wasmtypes.QueryClient
}

func (query *semanticWasmAggregate) Wasm() wasmtypes.QueryClient { return query.wasm }

func runWasmSemanticQuery(t *testing.T, commandFactory func() *cobra.Command, query *semanticWasmQuery, output io.Writer, args ...string) error {
	t.Helper()
	return runSemanticQuery(t, commandFactory(), &semanticWasmAggregate{wasm: query}, nil, output, args...)
}

func TestWasmQueryRequestsPreserveInputs(t *testing.T) {
	page := &sdkquery.PageRequest{Offset: 8, Limit: 4, CountTotal: true, Reverse: true}
	pageFlags := map[string]string{"page": "3", "limit": "4", "count-total": "true", "reverse": "true"}
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
		flags          map[string]string
		method         string
		assertRequest  func(*testing.T, interface{})
	}{
		{name: "codes", commandFactory: GetQueryWasmListCodeCmd, flags: pageFlags, method: "codes", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, page, got.(*wasmtypes.QueryCodesRequest).Pagination)
		}},
		{name: "contracts by code", commandFactory: GetQueryWasmListContractByCodeCmd, args: []string{"17"}, flags: pageFlags, method: "contracts-by-code", assertRequest: func(t *testing.T, got interface{}) {
			request := got.(*wasmtypes.QueryContractsByCodeRequest)
			require.Equal(t, uint64(17), request.CodeId)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "code info", commandFactory: GetQueryWasmCodeInfoCmd, args: []string{"18"}, method: "code-info", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, uint64(18), got.(*wasmtypes.QueryCodeInfoRequest).CodeId)
		}},
		{name: "contract", commandFactory: GetQueryWasmContractInfoCmd, args: []string{stateTestOwner}, method: "contract", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, stateTestOwner, got.(*wasmtypes.QueryContractInfoRequest).Address)
		}},
		{name: "all state", commandFactory: GetQueryWasmContractStateAllCmd, args: []string{stateTestOwner}, flags: pageFlags, method: "all-state", assertRequest: func(t *testing.T, got interface{}) {
			request := got.(*wasmtypes.QueryAllContractStateRequest)
			require.Equal(t, stateTestOwner, request.Address)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "raw state", commandFactory: GetQueryWasmContractStateRawCmd, args: []string{stateTestOwner, "6162"}, method: "raw-state", assertRequest: func(t *testing.T, got interface{}) {
			request := got.(*wasmtypes.QueryRawContractStateRequest)
			require.Equal(t, stateTestOwner, request.Address)
			require.Equal(t, []byte("ab"), request.QueryData)
		}},
		{name: "smart state", commandFactory: GetQueryWasmContractStateSmartCmd, args: []string{stateTestOwner, `{"balance":{}}`}, method: "smart-state", assertRequest: func(t *testing.T, got interface{}) {
			request := got.(*wasmtypes.QuerySmartContractStateRequest)
			require.Equal(t, stateTestOwner, request.Address)
			require.JSONEq(t, `{"balance":{}}`, string(request.QueryData))
		}},
		{name: "history", commandFactory: GetQueryWasmContractHistoryCmd, args: []string{stateTestOwner}, flags: pageFlags, method: "history", assertRequest: func(t *testing.T, got interface{}) {
			request := got.(*wasmtypes.QueryContractHistoryRequest)
			require.Equal(t, stateTestOwner, request.Address)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "pinned", commandFactory: GetQueryWasmListPinnedCodeCmd, flags: pageFlags, method: "pinned", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, page, got.(*wasmtypes.QueryPinnedCodesRequest).Pagination)
		}},
		{name: "contracts by creator", commandFactory: GetQueryWasmListContractsByCreatorCmd, args: []string{stateTestOwner}, flags: pageFlags, method: "contracts-by-creator", assertRequest: func(t *testing.T, got interface{}) {
			request := got.(*wasmtypes.QueryContractsByCreatorRequest)
			require.Equal(t, stateTestOwner, request.CreatorAddress)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "params", commandFactory: GetQueryWasmParamsCmd, method: "params", assertRequest: func(t *testing.T, got interface{}) { require.Equal(t, &wasmtypes.QueryParamsRequest{}, got) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &semanticWasmQuery{}
			cmd := test.commandFactory()
			for name, value := range test.flags {
				flags := cmd.Flags()
				if flags.Lookup(name) == nil {
					flags = cmd.PersistentFlags()
				}
				require.NoError(t, flags.Set(name, value))
			}
			err := runSemanticQuery(t, cmd, &semanticWasmAggregate{wasm: query}, nil, io.Discard, test.args...)
			require.NoError(t, err)
			require.Equal(t, test.method, query.method)
			test.assertRequest(t, query.request)
		})
	}
}

func TestWasmCodeQueryWritesExactResponse(t *testing.T) {
	query := &semanticWasmQuery{}
	destination := filepath.Join(t.TempDir(), "contract.wasm")
	err := runWasmSemanticQuery(t, GetQueryWasmCodeCmd, query, io.Discard, "23", destination)
	require.NoError(t, err)
	require.Equal(t, uint64(23), query.request.(*wasmtypes.QueryCodeRequest).CodeId)
	data, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, []byte("wasm-bytecode"), data)
	info, err := os.Stat(destination)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWasmQueryRejectsInvalidInputsBeforeTransport(t *testing.T) {
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
		flags          map[string]string
	}{
		{name: "zero code id", commandFactory: GetQueryWasmListContractByCodeCmd, args: []string{"0"}},
		{name: "invalid code id", commandFactory: GetQueryWasmCodeInfoCmd, args: []string{"x"}},
		{name: "contract", commandFactory: GetQueryWasmContractInfoCmd, args: []string{"bad"}},
		{name: "all state address", commandFactory: GetQueryWasmContractStateAllCmd, args: []string{"bad"}},
		{name: "raw key", commandFactory: GetQueryWasmContractStateRawCmd, args: []string{stateTestOwner, "xyz"}},
		{name: "raw multiple encodings", commandFactory: GetQueryWasmContractStateRawCmd, args: []string{stateTestOwner, "ab"}, flags: map[string]string{"ascii": "true", "hex": "true"}},
		{name: "smart empty", commandFactory: GetQueryWasmContractStateSmartCmd, args: []string{stateTestOwner, ""}},
		{name: "smart json", commandFactory: GetQueryWasmContractStateSmartCmd, args: []string{stateTestOwner, "not-json"}},
		{name: "history address", commandFactory: GetQueryWasmContractHistoryCmd, args: []string{"bad"}},
		{name: "creator address", commandFactory: GetQueryWasmListContractsByCreatorCmd, args: []string{"bad"}},
		{name: "page and offset", commandFactory: GetQueryWasmListCodeCmd, flags: map[string]string{"page": "2", "offset": "1"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &semanticWasmQuery{}
			cmd := test.commandFactory()
			for name, value := range test.flags {
				flags := cmd.Flags()
				if flags.Lookup(name) == nil {
					flags = cmd.PersistentFlags()
				}
				require.NoError(t, flags.Set(name, value))
			}
			err := runSemanticQuery(t, cmd, &semanticWasmAggregate{wasm: query}, nil, io.Discard, test.args...)
			require.Error(t, err)
			require.Empty(t, query.method)
		})
	}
}

func TestWasmQueryRejectsNilSuccessfulResponses(t *testing.T) {
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           func(*testing.T) []string
	}{
		{name: "codes", commandFactory: GetQueryWasmListCodeCmd},
		{name: "contracts by code", commandFactory: GetQueryWasmListContractByCodeCmd, args: func(*testing.T) []string { return []string{"7"} }},
		{name: "code", commandFactory: GetQueryWasmCodeCmd, args: func(t *testing.T) []string { return []string{"7", filepath.Join(t.TempDir(), "contract.wasm")} }},
		{name: "code info", commandFactory: GetQueryWasmCodeInfoCmd, args: func(*testing.T) []string { return []string{"7"} }},
		{name: "contract", commandFactory: GetQueryWasmContractInfoCmd, args: func(*testing.T) []string { return []string{stateTestOwner} }},
		{name: "all state", commandFactory: GetQueryWasmContractStateAllCmd, args: func(*testing.T) []string { return []string{stateTestOwner} }},
		{name: "raw state", commandFactory: GetQueryWasmContractStateRawCmd, args: func(*testing.T) []string { return []string{stateTestOwner, "6162"} }},
		{name: "smart state", commandFactory: GetQueryWasmContractStateSmartCmd, args: func(*testing.T) []string { return []string{stateTestOwner, `{}`} }},
		{name: "history", commandFactory: GetQueryWasmContractHistoryCmd, args: func(*testing.T) []string { return []string{stateTestOwner} }},
		{name: "pinned", commandFactory: GetQueryWasmListPinnedCodeCmd},
		{name: "contracts by creator", commandFactory: GetQueryWasmListContractsByCreatorCmd, args: func(*testing.T) []string { return []string{stateTestOwner} }},
		{name: "params", commandFactory: GetQueryWasmParamsCmd},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var args []string
			if test.args != nil {
				args = test.args(t)
			}
			query := &semanticWasmQuery{nilResponse: true}
			var err error
			require.NotPanics(t, func() {
				err = runWasmSemanticQuery(t, test.commandFactory, query, io.Discard, args...)
			})
			require.ErrorContains(t, err, "malformed node response")
		})
	}
}

func TestWasmQueryPreservesTransportOutputAndNotFoundErrors(t *testing.T) {
	t.Run("transport cause", func(t *testing.T) {
		cause := errors.New("wasm transport failed")
		err := runWasmSemanticQuery(t, GetQueryWasmParamsCmd, &semanticWasmQuery{err: cause}, io.Discard)
		require.ErrorIs(t, err, cause)
	})

	t.Run("short output", func(t *testing.T) {
		err := runWasmSemanticQuery(t, GetQueryWasmParamsCmd, &semanticWasmQuery{}, outputBoundaryShortWriter{})
		require.ErrorIs(t, err, io.ErrShortWrite)
	})

	t.Run("empty code", func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "missing.wasm")
		err := runWasmSemanticQuery(t, GetQueryWasmCodeCmd, &semanticWasmQuery{emptyCode: true}, io.Discard, "7", destination)
		require.EqualError(t, err, "contract not found")
		_, statErr := os.Stat(destination)
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})
}

func TestWasmQueryCommandRegistersLeaves(t *testing.T) {
	cmd := GetQueryWasmCmd()
	for _, name := range []string{"list-code", "list-contract-by-code", "code", "code-info", "contract", "contract-history", "contract-state", "pinned", "params", "build-address", "list-contracts-by-creator"} {
		child, _, err := cmd.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, child.Name())
	}

	state := GetQueryWasmContractStateCmd()
	for _, name := range []string{"all", "raw", "smart"} {
		child, _, err := state.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, child.Name())
	}
}
