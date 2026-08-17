package node

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/mark3labs/mcp-go/mcp"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type nodeClientStub struct {
	syncInfo *ctypes.SyncInfo
	height   int64
	err      error
}

func (stub nodeClientStub) SyncInfo(context.Context) (*ctypes.SyncInfo, error) {
	return stub.syncInfo, stub.err
}

func (stub nodeClientStub) CurrentBlockHeight(context.Context) (int64, error) {
	return stub.height, stub.err
}

type lightClientStub struct {
	node v1beta3.NodeClient
}

func (stub lightClientStub) Query() v1beta3.QueryClient  { return nil }
func (stub lightClientStub) Node() v1beta3.NodeClient    { return stub.node }
func (lightClientStub) ClientContext() sdkclient.Context { return sdkclient.Context{} }
func (lightClientStub) PrintMessage(interface{}) error   { return nil }
func (lightClientStub) PrintJSON(interface{}) error      { return nil }

func TestNodeToolsReturnSemanticResults(t *testing.T) {
	if tool := ToolNodeStatus(); tool.Name != "akash_node_status" || tool.Description == "" {
		t.Fatalf("node status tool metadata = %#v", tool)
	}
	if tool := ToolBlockHeight(); tool.Name != "akash_block_height" || tool.Description == "" {
		t.Fatalf("block height tool metadata = %#v", tool)
	}

	cl := lightClientStub{node: nodeClientStub{
		syncInfo: &ctypes.SyncInfo{LatestBlockHeight: 42, CatchingUp: true},
		height:   42,
	}}

	status, err := HandleNodeStatus(cl)(context.Background(), mcp.CallToolRequest{})
	if err != nil || status == nil || status.IsError {
		t.Fatalf("node status result = %#v, err = %v", status, err)
	}
	var syncInfo ctypes.SyncInfo
	if err := json.Unmarshal([]byte(toolResultText(t, status)), &syncInfo); err != nil {
		t.Fatalf("decode node status: %v", err)
	}
	if syncInfo.LatestBlockHeight != 42 || !syncInfo.CatchingUp {
		t.Fatalf("sync info = %#v, want height 42 and catching up", syncInfo)
	}

	height, err := HandleBlockHeight(cl)(context.Background(), mcp.CallToolRequest{})
	if err != nil || height == nil || height.IsError {
		t.Fatalf("block height result = %#v, err = %v", height, err)
	}
	var payload map[string]int64
	if err := json.Unmarshal([]byte(toolResultText(t, height)), &payload); err != nil {
		t.Fatalf("decode block height: %v", err)
	}
	if payload["block_height"] != 42 {
		t.Fatalf("block height = %d, want 42", payload["block_height"])
	}
}

func TestNodeToolsReturnDependencyErrorsAsToolResults(t *testing.T) {
	cl := lightClientStub{node: nodeClientStub{err: errors.New("node unavailable")}}
	tests := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		want    string
	}{
		{name: "status", handler: HandleNodeStatus(cl), want: "failed to get sync info"},
		{name: "height", handler: HandleBlockHeight(cl), want: "failed to get block height"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.handler(context.Background(), mcp.CallToolRequest{})
			if err != nil {
				t.Fatalf("transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("result = %#v, want MCP error result", result)
			}
			text := toolResultText(t, result)
			if !strings.Contains(text, tc.want) || !strings.Contains(text, "node unavailable") {
				t.Fatalf("error result = %q", text)
			}
		})
	}
}

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v, want one text result", result.Content)
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want mcp.TextContent", result.Content[0])
	}
	return text.Text
}
