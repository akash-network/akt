package e2e

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"
)

// Exercise the built binary, including root context setup and --height routing.
func TestContractQueryShortcutsOffline(t *testing.T) {
	address := "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"
	label := "my contract"
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/websocket" {
			// Context setup also starts the SDK event client; these queries use HTTP.
			http.Error(w, "events unavailable in query fixture", http.StatusServiceUnavailable)
			return
		}
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Path   string `json:"path"`
				Data   string `json:"data"`
				Height string `json:"height"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode RPC request %s %s: %v", r.Method, r.URL, err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		result := any(map[string]any{"client_info": map[string]string{"api_version": "v1beta3"}})
		if request.Method != "akash" {
			if request.Method != "abci_query" || request.Params.Height != "47" {
				t.Errorf("unexpected RPC request: %+v", request)
				http.Error(w, "unexpected query", http.StatusBadRequest)
				return
			}
			mu.Lock()
			paths = append(paths, request.Params.Path)
			mu.Unlock()
			var message proto.Message
			switch request.Params.Path {
			case "/cosmwasm.wasm.v1.Query/Codes":
				message = &wasmtypes.QueryCodesResponse{CodeInfos: []wasmtypes.CodeInfoResponse{{CodeID: 7}}}
			case "/cosmwasm.wasm.v1.Query/ContractsByCode":
				message = &wasmtypes.QueryContractsByCodeResponse{Contracts: []string{address}}
			case "/cosmwasm.wasm.v1.Query/ContractInfo":
				data, err := hex.DecodeString(strings.TrimPrefix(request.Params.Data, "0x"))
				if err != nil {
					t.Errorf("decode contract request: %v", err)
					http.Error(w, "bad contract request", http.StatusBadRequest)
					return
				}
				var query wasmtypes.QueryContractInfoRequest
				if err := proto.Unmarshal(data, &query); err != nil || query.Address != address {
					t.Errorf("contract request = %+v, error = %v", query, err)
					http.Error(w, "wrong contract", http.StatusBadRequest)
					return
				}
				message = &wasmtypes.QueryContractInfoResponse{Address: address, ContractInfo: wasmtypes.ContractInfo{CodeID: 7, Label: label, Creator: address}}
			default:
				t.Errorf("unexpected query path %q", request.Params.Path)
				http.Error(w, "unknown query", http.StatusBadRequest)
				return
			}
			payload, err := proto.Marshal(message)
			if err != nil {
				t.Errorf("marshal RPC response: %v", err)
				http.Error(w, "response error", http.StatusInternalServerError)
				return
			}
			result = map[string]any{"response": map[string]any{"code": 0, "height": "47", "value": base64.StdEncoding.EncodeToString(payload)}}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			t.Errorf("write RPC response: %v", err)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	config := fmt.Sprintf(`version: 1
current-context: contracts
networks:
  - name: fixture
    chain-id: akashnet-2
    endpoints:
      rpc:
        - %s
    gas-prices: 0.025uakt
    gas-adjustment: "1.5"
keyrings:
  - name: fixture
    backend: test
contexts:
  - name: contracts
    network: fixture
    keyring: fixture
defaults:
  output: pretty
`, server.URL)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte(config), 0o600))
	for name, args := range map[string][]string{
		"list":               {"contracts"},
		"singular list":      {"contract"},
		"address":            {"contract", address},
		"label":              {"contract", label},
		"bare address":       {address},
		"wasm compatibility": {"wasm", "contract", address},
	} {
		t.Run(name, func(t *testing.T) {
			mu.Lock()
			paths = nil
			mu.Unlock()
			command := append([]string{"q"}, args...)
			command = append(command, "--height", "47")
			stdout, stderr, code := runAkt(t, home, command...)
			require.Equal(t, 0, code, stderr)
			require.Empty(t, stderr)
			require.Contains(t, stdout, address)
			require.Contains(t, stdout, label)
			require.NotContains(t, stdout, "\x1b")
			if name == "address" || name == "bare address" {
				mu.Lock()
				got := append([]string(nil), paths...)
				mu.Unlock()
				require.Equal(t, []string{"/cosmwasm.wasm.v1.Query/ContractInfo"}, got)
			}
		})
	}
}
