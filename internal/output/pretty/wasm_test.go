package pretty

import (
	"testing"

	"github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/charmbracelet/x/exp/golden"
)

func TestRenderWasmCodeList(t *testing.T) {
	tests := map[string]struct {
		res *types.QueryCodesResponse
	}{
		"Empty": {
			res: &types.QueryCodesResponse{},
		},
		"WithCodes": {
			res: &types.QueryCodesResponse{
				CodeInfos: []types.CodeInfoResponse{
					{CodeID: 1, Creator: "akash1abc", DataHash: []byte("abcdef1234567890abcdef1234567890")},
					{CodeID: 2, Creator: "akash1def", DataHash: []byte("1234567890abcdef")},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderWasmCodeList(tc.res))
		})
	}
}

func TestRenderWasmContractsByCode(t *testing.T) {
	tests := map[string]struct {
		res *types.QueryContractsByCodeResponse
	}{
		"Empty": {
			res: &types.QueryContractsByCodeResponse{},
		},
		"WithContracts": {
			res: &types.QueryContractsByCodeResponse{
				Contracts: []string{
					"akash1contract1aaaaaaaaaaaaaaaaaaaaaaaaa",
					"akash1contract2bbbbbbbbbbbbbbbbbbbbbbbbb",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderWasmContractsByCode(tc.res))
		})
	}
}

func TestRenderWasmCodeInfo(t *testing.T) {
	tests := map[string]struct {
		res *types.QueryCodeInfoResponse
	}{
		"Basic": {
			res: &types.QueryCodeInfoResponse{
				CodeID:   1,
				Creator:  "akash1abc",
				Checksum: []byte("abcdef1234567890"),
				InstantiatePermission: types.AccessConfig{
					Permission: types.AccessTypeEverybody,
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderWasmCodeInfo(tc.res))
		})
	}
}

func TestRenderWasmContractInfo(t *testing.T) {
	tests := map[string]struct {
		res *types.QueryContractInfoResponse
	}{
		"Basic": {
			res: &types.QueryContractInfoResponse{
				Address: "akash1contract1",
				ContractInfo: types.ContractInfo{
					CodeID:  1,
					Label:   "my-contract",
					Creator: "akash1abc",
				},
			},
		},
		"WithAdminAndIBC": {
			res: &types.QueryContractInfoResponse{
				Address: "akash1contract2",
				ContractInfo: types.ContractInfo{
					CodeID:    2,
					Label:     "ibc-contract",
					Creator:   "akash1abc",
					Admin:     "akash1admin",
					IBCPortID: "wasm.akash1contract2",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderWasmContractInfo(tc.res))
		})
	}
}

func TestRenderWasmContractHistory(t *testing.T) {
	tests := map[string]struct {
		res *types.QueryContractHistoryResponse
	}{
		"Empty": {
			res: &types.QueryContractHistoryResponse{},
		},
		"WithEntries": {
			res: &types.QueryContractHistoryResponse{
				Entries: []types.ContractCodeHistoryEntry{
					{
						Operation: types.ContractCodeHistoryOperationTypeInit,
						CodeID:    1,
						Updated:   &types.AbsoluteTxPosition{BlockHeight: 100},
					},
					{
						Operation: types.ContractCodeHistoryOperationTypeMigrate,
						CodeID:    2,
						Updated:   &types.AbsoluteTxPosition{BlockHeight: 200},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderWasmContractHistory(tc.res))
		})
	}
}

func TestRenderWasmPinnedCodes(t *testing.T) {
	tests := map[string]struct {
		res *types.QueryPinnedCodesResponse
	}{
		"Empty": {
			res: &types.QueryPinnedCodesResponse{},
		},
		"WithCodes": {
			res: &types.QueryPinnedCodesResponse{
				CodeIDs: []uint64{1, 5, 10},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderWasmPinnedCodes(tc.res))
		})
	}
}

func TestRenderWasmContractsByCreator(t *testing.T) {
	tests := map[string]struct {
		res *types.QueryContractsByCreatorResponse
	}{
		"Empty": {
			res: &types.QueryContractsByCreatorResponse{},
		},
		"WithContracts": {
			res: &types.QueryContractsByCreatorResponse{
				ContractAddresses: []string{
					"akash1contract1",
					"akash1contract2",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderWasmContractsByCreator(tc.res))
		})
	}
}

