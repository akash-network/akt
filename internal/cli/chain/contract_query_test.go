package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	aktcodec "pkg.akt.dev/akt/internal/codec"
)

type contractQueryStub struct {
	wasmtypes.QueryClient
	codes     func(*wasmtypes.QueryCodesRequest) (*wasmtypes.QueryCodesResponse, error)
	contracts func(*wasmtypes.QueryContractsByCodeRequest) (*wasmtypes.QueryContractsByCodeResponse, error)
	info      func(*wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error)
	calls     []string
}

func (q *contractQueryStub) Codes(_ context.Context, req *wasmtypes.QueryCodesRequest, _ ...grpc.CallOption) (*wasmtypes.QueryCodesResponse, error) {
	q.calls = append(q.calls, "codes")
	return q.codes(req)
}

func (q *contractQueryStub) ContractsByCode(_ context.Context, req *wasmtypes.QueryContractsByCodeRequest, _ ...grpc.CallOption) (*wasmtypes.QueryContractsByCodeResponse, error) {
	q.calls = append(q.calls, fmt.Sprintf("code:%d", req.CodeId))
	return q.contracts(req)
}

func (q *contractQueryStub) ContractInfo(_ context.Context, req *wasmtypes.QueryContractInfoRequest, _ ...grpc.CallOption) (*wasmtypes.QueryContractInfoResponse, error) {
	q.calls = append(q.calls, "contract:"+req.Address)
	return q.info(req)
}

func newContractQueryStub(t *testing.T) *contractQueryStub {
	t.Helper()
	return &contractQueryStub{
		codes: func(req *wasmtypes.QueryCodesRequest) (*wasmtypes.QueryCodesResponse, error) {
			require.NotNil(t, req.Pagination)
			if len(req.Pagination.Key) == 0 {
				return &wasmtypes.QueryCodesResponse{CodeInfos: []wasmtypes.CodeInfoResponse{{CodeID: 1}}, Pagination: &querytypes.PageResponse{NextKey: []byte("next-code")}}, nil
			}
			require.Equal(t, []byte("next-code"), req.Pagination.Key)
			return &wasmtypes.QueryCodesResponse{CodeInfos: []wasmtypes.CodeInfoResponse{{CodeID: 2}}}, nil
		},
		contracts: func(req *wasmtypes.QueryContractsByCodeRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
			require.NotNil(t, req.Pagination)
			if req.CodeId == 1 && len(req.Pagination.Key) == 0 {
				return &wasmtypes.QueryContractsByCodeResponse{Contracts: []string{stateTestProvider}, Pagination: &querytypes.PageResponse{NextKey: []byte("next-contract")}}, nil
			}
			if req.CodeId == 1 {
				require.Equal(t, []byte("next-contract"), req.Pagination.Key)
			}
			// A repeated address across pages/codes must not become a duplicate label.
			return &wasmtypes.QueryContractsByCodeResponse{Contracts: []string{stateTestOwner}}, nil
		},
		info: func(req *wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
			label := "my contract"
			if req.Address == stateTestProvider {
				label = "another contract"
			}
			return &wasmtypes.QueryContractInfoResponse{Address: req.Address, ContractInfo: wasmtypes.ContractInfo{CodeID: 1, Label: label, Creator: stateTestOwner}}, nil
		},
	}
}

func runContractQuery(t *testing.T, q *contractQueryStub, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cctx := sdkclient.Context{}.WithCodec(aktcodec.MakeEncodingConfig().Codec)
	cl := &semanticQueryLightClient{query: &semanticWasmAggregate{wasm: q}, cctx: cctx}
	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
	ctx = context.WithValue(ctx, ContextTypeQueryClient, cl)
	root := &cobra.Command{Use: "akt", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(QueryCmd())
	root.SetContext(ctx)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"q"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestContractQueryShortcuts(t *testing.T) {
	for name, args := range map[string][]string{
		"address":      {"contract", stateTestOwner},
		"bare address": {stateTestOwner},
		"label":        {"contract", "my contract"},
		"plural label": {"contracts", "my contract"},
	} {
		t.Run(name, func(t *testing.T) {
			q := newContractQueryStub(t)
			out, err := runContractQuery(t, q, append(args, "-o", "json")...)
			require.NoError(t, err)
			var result struct {
				Address string `json:"address"`
				Info    struct {
					Label string `json:"label"`
				} `json:"contract_info"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			require.Equal(t, stateTestOwner, result.Address)
			require.Equal(t, "my contract", result.Info.Label)
			if name == "address" || name == "bare address" {
				require.Equal(t, []string{"contract:" + stateTestOwner}, q.calls)
			}
		})
	}
}

func TestContractLabelResolutionErrors(t *testing.T) {
	for _, label := range []string{"missing", "MY CONTRACT", "duplicate"} {
		t.Run(label, func(t *testing.T) {
			q := newContractQueryStub(t)
			if label == "duplicate" {
				original := q.info
				q.info = func(req *wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
					res, err := original(req)
					res.Label = label
					return res, err
				}
			}
			out, err := runContractQuery(t, q, "contract", label)
			require.Error(t, err)
			require.Empty(t, out)
			if label == "duplicate" {
				require.ErrorContains(t, err, "matches multiple addresses")
				require.ErrorContains(t, err, stateTestOwner)
				require.ErrorContains(t, err, stateTestProvider)
				require.ErrorContains(t, err, "akt q contract <address>")
			} else {
				require.ErrorContains(t, err, "no contract with label")
				require.ErrorContains(t, err, "akt q contracts")
			}
		})
	}
}

func TestContractQueryInputValidation(t *testing.T) {
	for _, args := range [][]string{{"bnak"}, {stateTestOwner, "extra"}, {"contract", ""}, {"contract", " "}, {"contract", "one", "two"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			q := newContractQueryStub(t)
			_, err := runContractQuery(t, q, args...)
			require.Error(t, err)
			require.Empty(t, q.calls)
		})
	}
	q := newContractQueryStub(t)
	out, err := runContractQuery(t, q)
	require.NoError(t, err)
	require.Contains(t, out, "Usage:")
	require.Empty(t, q.calls)
}

func TestContractQueryEmptyList(t *testing.T) {
	for _, format := range []string{"pretty", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			q := newContractQueryStub(t)
			q.codes = func(*wasmtypes.QueryCodesRequest) (*wasmtypes.QueryCodesResponse, error) {
				return &wasmtypes.QueryCodesResponse{Pagination: &querytypes.PageResponse{}}, nil
			}
			out, err := runContractQuery(t, q, "contract", "-o", format)
			require.NoError(t, err)
			switch format {
			case "pretty":
				require.Equal(t, "(no contracts)\n", out)
			case "yaml":
				require.Equal(t, "---\n[]\n", out)
			default:
				require.Equal(t, "[]\n", out)
			}
		})
	}
}

func TestContractQueryRejectsIncompleteResults(t *testing.T) {
	failure := errors.New("RPC unavailable")
	tests := []struct {
		name  string
		setup func(*contractQueryStub)
		want  string
		cause error
	}{
		{"codes error", func(q *contractQueryStub) {
			q.codes = func(*wasmtypes.QueryCodesRequest) (*wasmtypes.QueryCodesResponse, error) { return nil, failure }
		}, "list contract codes", failure},
		{"missing codes", func(q *contractQueryStub) {
			q.codes = func(*wasmtypes.QueryCodesRequest) (*wasmtypes.QueryCodesResponse, error) { return nil, nil }
		}, "missing response", nil},
		{"contracts error", func(q *contractQueryStub) {
			q.contracts = func(*wasmtypes.QueryContractsByCodeRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
				return nil, failure
			}
		}, "list contracts for code", failure},
		{"missing contracts", func(q *contractQueryStub) {
			q.contracts = func(*wasmtypes.QueryContractsByCodeRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
				return nil, nil
			}
		}, "missing response", nil},
		{"info error", func(q *contractQueryStub) {
			q.info = func(*wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
				return nil, failure
			}
		}, "query contract", failure},
		{"missing info", func(q *contractQueryStub) {
			q.info = func(*wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
				return nil, nil
			}
		}, "missing response", nil},
		{"wrong address", func(q *contractQueryStub) {
			q.info = func(*wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
				return &wasmtypes.QueryContractInfoResponse{}, nil
			}
		}, "malformed node response: address", nil},
		{"empty contract address", func(q *contractQueryStub) {
			q.contracts = func(*wasmtypes.QueryContractsByCodeRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
				return &wasmtypes.QueryContractsByCodeResponse{Contracts: []string{""}}, nil
			}
		}, "malformed node response: address", nil},
		{"repeated code page", func(q *contractQueryStub) {
			q.codes = func(*wasmtypes.QueryCodesRequest) (*wasmtypes.QueryCodesResponse, error) {
				return &wasmtypes.QueryCodesResponse{Pagination: &querytypes.PageResponse{NextKey: []byte("repeat")}}, nil
			}
		}, "repeated pagination key", nil},
		{"repeated contract page", func(q *contractQueryStub) {
			q.contracts = func(*wasmtypes.QueryContractsByCodeRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
				return &wasmtypes.QueryContractsByCodeResponse{Pagination: &querytypes.PageResponse{NextKey: []byte("repeat")}}, nil
			}
		}, "repeated pagination key", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := newContractQueryStub(t)
			tc.setup(q)
			out, err := runContractQuery(t, q, "contracts", "-o", "json")
			require.ErrorContains(t, err, tc.want)
			require.Empty(t, out)
			if tc.cause != nil {
				require.ErrorIs(t, err, tc.cause)
			}
		})
	}
	t.Run("direct lookup error", func(t *testing.T) {
		q := newContractQueryStub(t)
		q.info = func(*wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
			return nil, failure
		}
		out, err := runContractQuery(t, q, stateTestOwner)
		require.ErrorIs(t, err, failure)
		require.Empty(t, out)
	})
}

func TestContractListIncludesLabels(t *testing.T) {
	for _, format := range []string{"pretty", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			q := newContractQueryStub(t)
			out, err := runContractQuery(t, q, "contracts", "-o", format)
			require.NoError(t, err)
			require.Contains(t, out, "my contract")
			require.Contains(t, out, "another contract")
			require.Contains(t, out, stateTestOwner)
			require.Contains(t, out, stateTestProvider)
			if format != "pretty" {
				var records []map[string]any
				if format == "json" {
					require.NoError(t, json.Unmarshal([]byte(out), &records))
				} else {
					require.NoError(t, yaml.Unmarshal([]byte(out), &records))
				}
				require.Len(t, records, 2)
				require.Equal(t, stateTestOwner, records[0]["address"])
				require.Equal(t, "1", records[0]["code_id"])
			}
			require.Equal(t, []string{"codes", "code:1", "contract:" + stateTestProvider, "code:1", "contract:" + stateTestOwner, "codes", "code:2"}, q.calls)
		})
	}
}
