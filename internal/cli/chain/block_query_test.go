package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	rpcclientmock "github.com/cometbft/cometbft/rpc/client/mock"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	cmttypes "github.com/cometbft/cometbft/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

type blockRPCProbe struct {
	rpcclientmock.Client

	statusResponse *coretypes.ResultStatus
	statusErr      error
	statusContext  context.Context
	statusCalls    int

	searchResponse *coretypes.ResultBlockSearch
	searchErr      error
	searchContext  context.Context
	searchCalls    int
	searchQuery    string
	searchPage     int
	searchLimit    int
	searchOrder    string

	blockResponse *coretypes.ResultBlock
	blockErr      error
	blockContext  context.Context
	blockCalls    int
	blockHeight   int64

	hashResponse *coretypes.ResultBlock
	hashErr      error
	hashContext  context.Context
	hashCalls    int
	hash         []byte

	resultsResponse *coretypes.ResultBlockResults
	resultsErr      error
	resultsContext  context.Context
	resultsCalls    int
	resultsHeight   int64
}

func (probe *blockRPCProbe) Status(ctx context.Context) (*coretypes.ResultStatus, error) {
	probe.statusContext = ctx
	probe.statusCalls++
	return probe.statusResponse, probe.statusErr
}

func (probe *blockRPCProbe) BlockSearch(ctx context.Context, query string, page, limit *int, order string) (*coretypes.ResultBlockSearch, error) {
	probe.searchContext = ctx
	probe.searchCalls++
	probe.searchQuery = query
	probe.searchPage = intValue(page)
	probe.searchLimit = intValue(limit)
	probe.searchOrder = order
	return probe.searchResponse, probe.searchErr
}

func (probe *blockRPCProbe) Block(ctx context.Context, height *int64) (*coretypes.ResultBlock, error) {
	probe.blockContext = ctx
	probe.blockCalls++
	probe.blockHeight = int64Value(height)
	return probe.blockResponse, probe.blockErr
}

func (probe *blockRPCProbe) BlockByHash(ctx context.Context, hash []byte) (*coretypes.ResultBlock, error) {
	probe.hashContext = ctx
	probe.hashCalls++
	probe.hash = append([]byte(nil), hash...)
	return probe.hashResponse, probe.hashErr
}

func (probe *blockRPCProbe) BlockResults(ctx context.Context, height *int64) (*coretypes.ResultBlockResults, error) {
	probe.resultsContext = ctx
	probe.resultsCalls++
	probe.resultsHeight = int64Value(height)
	return probe.resultsResponse, probe.resultsErr
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func blockResult(height int64, appHash []byte) *coretypes.ResultBlock {
	return &coretypes.ResultBlock{
		Block: &cmttypes.Block{Header: cmttypes.Header{
			ChainID: "akt-block-query-test",
			Height:  height,
			Time:    time.Unix(1_750_000_000, 0).UTC(),
			AppHash: append([]byte(nil), appHash...),
		}},
	}
}

func unencodableBlock(height int64) *coretypes.ResultBlock {
	result := blockResult(height, []byte{0x01})
	result.Block.Evidence.Evidence = []cmttypes.Evidence{nil}
	return result
}

type blockCommandResult struct {
	stdout string
	stderr string
	stale  string
	err    error
}

func executeBlockCommand(t *testing.T, cmd *cobra.Command, node sdkclient.CometRPC, output io.Writer, args ...string) blockCommandResult {
	return executeBlockCommandOnContext(t, context.Background(), cmd, node, output, args...)
}

func executeBlockCommandOnContext(
	t *testing.T,
	base context.Context,
	cmd *cobra.Command,
	node sdkclient.CometRPC,
	output io.Writer,
	args ...string,
) blockCommandResult {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var stale bytes.Buffer
	cctx := queryTestClientContext(&stale)
	if node != nil {
		cctx = cctx.WithClient(node)
	}

	cmd.SetContext(context.WithValue(base, ClientContextKey, &cctx))
	if output != nil {
		cmd.SetOut(output)
	} else {
		cmd.SetOut(&stdout)
	}
	cmd.SetErr(&stderr)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)

	err := cmd.Execute()
	if stale.Len() != 0 {
		t.Fatalf("command wrote %q to the stale client-context writer instead of Cobra stdout", stale.String())
	}
	return blockCommandResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		stale:  stale.String(),
		err:    err,
	}
}

func executeBlockHandler(t *testing.T, cmd *cobra.Command, node sdkclient.CometRPC, args ...string) error {
	t.Helper()

	var output bytes.Buffer
	cctx := queryTestClientContext(&output)
	if node != nil {
		cctx = cctx.WithClient(node)
	}
	cmd.SetContext(context.WithValue(context.Background(), ClientContextKey, &cctx))
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	return cmd.RunE(cmd, args)
}

func requireJSONInt64(t *testing.T, raw json.RawMessage, want int64) {
	t.Helper()

	value := string(bytes.Trim(raw, `"`))
	got, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("decode JSON integer %q: %v", raw, err)
	}
	if got != want {
		t.Fatalf("JSON integer = %d, want %d", got, want)
	}
}

func requireBlockHeightOutput(t *testing.T, output string, want int64) {
	t.Helper()

	var response struct {
		Header struct {
			Height json.RawMessage `json:"height"`
		} `json:"header"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode block output: %v\noutput: %s", err, output)
	}
	requireJSONInt64(t, response.Header.Height, want)
}

func TestBlockQueryHandlersPropagateClientContextErrors(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{name: "block search", cmd: QueryBlocksCmd, args: []string{"block.height > 1"}},
		{name: "block", cmd: QueryBlockCmd, args: []string{"1"}},
		{name: "block results", cmd: QueryBlockResultsCmd, args: []string{"1"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.cmd()
			if err := cmd.Flags().Set(cflags.FlagNode, ""); err != nil {
				t.Fatalf("set empty node: %v", err)
			}

			err := executeBlockHandler(t, cmd, &blockRPCProbe{Client: rpcclientmock.New()}, test.args...)
			if err == nil || err.Error() != "--node cannot be empty" {
				t.Fatalf("RunE() error = %v, want empty-node error", err)
			}
		})
	}
}

func TestQueryBlocksCmdSearchesAndPrintsExactPage(t *testing.T) {
	probe := &blockRPCProbe{
		Client: rpcclientmock.New(),
		searchResponse: &coretypes.ResultBlockSearch{
			Blocks:     []*coretypes.ResultBlock{blockResult(29, []byte{0x29})},
			TotalCount: 15,
		},
	}
	result := executeBlockCommand(
		t,
		QueryBlocksCmd(),
		probe,
		nil,
		"block.height > 7", "--page", "2", "--limit", "7", "--order_by", "asc", "--output", "json",
	)
	if result.err != nil {
		t.Fatalf("query blocks: %v", result.err)
	}
	if probe.searchCalls != 1 || probe.searchQuery != "block.height > 7" || probe.searchPage != 2 || probe.searchLimit != 7 || probe.searchOrder != "asc" {
		t.Fatalf("BlockSearch call = count %d query %q page %d limit %d order %q",
			probe.searchCalls, probe.searchQuery, probe.searchPage, probe.searchLimit, probe.searchOrder)
	}

	var response struct {
		TotalCount json.RawMessage `json:"total_count"`
		Count      json.RawMessage `json:"count"`
		PageNumber json.RawMessage `json:"page_number"`
		Limit      json.RawMessage `json:"limit"`
		Blocks     []struct {
			Header struct {
				Height json.RawMessage `json:"height"`
			} `json:"header"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &response); err != nil {
		t.Fatalf("decode search output: %v\noutput: %s", err, result.stdout)
	}
	requireJSONInt64(t, response.TotalCount, 15)
	requireJSONInt64(t, response.Count, 1)
	requireJSONInt64(t, response.PageNumber, 2)
	requireJSONInt64(t, response.Limit, 7)
	if len(response.Blocks) != 1 {
		t.Fatalf("search returned %d blocks, want 1", len(response.Blocks))
	}
	requireJSONInt64(t, response.Blocks[0].Header.Height, 29)
}

func TestQueryBlocksCmdRejectsInvalidInputAndRPCFailures(t *testing.T) {
	t.Run("query is required", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlocksCmd(), probe, nil)
		if result.err == nil || result.err.Error() != "query expression is required: pass it positionally" {
			t.Fatalf("error = %v, want missing-query error", result.err)
		}
		if probe.searchCalls != 0 {
			t.Fatalf("missing query made %d BlockSearch calls", probe.searchCalls)
		}
	})

	t.Run("too many positional arguments", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlocksCmd(), probe, nil, "one", "two")
		if result.err == nil {
			t.Fatal("two query expressions were accepted")
		}
		if probe.searchCalls != 0 {
			t.Fatalf("invalid arguments made %d BlockSearch calls", probe.searchCalls)
		}
	})

	t.Run("RPC error", func(t *testing.T) {
		rpcErr := errors.New("block search unavailable")
		probe := &blockRPCProbe{Client: rpcclientmock.New(), searchErr: rpcErr}
		result := executeBlockCommand(t, QueryBlocksCmd(), probe, nil, "block.height > 7")
		if !errors.Is(result.err, rpcErr) {
			t.Fatalf("error = %v, want %v", result.err, rpcErr)
		}
	})

	t.Run("missing RPC client", func(t *testing.T) {
		cmd := QueryBlocksCmd()
		nodeFlag := cmd.Flags().Lookup(cflags.FlagNode)
		if err := nodeFlag.Value.Set(""); err != nil {
			t.Fatalf("clear node default: %v", err)
		}
		err := executeBlockHandler(t, cmd, nil, "block.height > 7")
		if err == nil || err.Error() != "no RPC client is defined in offline mode" {
			t.Fatalf("RunE() error = %v, want missing-RPC-client error", err)
		}
	})

	t.Run("missing search result", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlocksCmd(), probe, nil, "block.height > 7")
		if result.err == nil || result.err.Error() != "missing block search result" {
			t.Fatalf("error = %v, want missing-result error", result.err)
		}
	})

	for _, test := range []struct {
		name  string
		block *coretypes.ResultBlock
		want  string
	}{
		{name: "nil block entry", block: nil, want: "invalid block at index 0: missing block body"},
		{name: "nil block body", block: &coretypes.ResultBlock{}, want: "invalid block at index 0: missing block body"},
		{name: "unencodable block", block: unencodableBlock(7), want: "invalid block at index 0: cannot encode block"},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := &blockRPCProbe{
				Client:         rpcclientmock.New(),
				searchResponse: &coretypes.ResultBlockSearch{Blocks: []*coretypes.ResultBlock{test.block}},
			}
			result := executeBlockCommand(t, QueryBlocksCmd(), probe, nil, "block.height > 7")
			if result.err == nil || result.err.Error() != test.want {
				t.Fatalf("error = %v, want %q", result.err, test.want)
			}
		})
	}

	t.Run("YAML output", func(t *testing.T) {
		probe := &blockRPCProbe{
			Client:         rpcclientmock.New(),
			searchResponse: &coretypes.ResultBlockSearch{Blocks: []*coretypes.ResultBlock{blockResult(7, []byte{0x01})}, TotalCount: 1},
		}
		result := executeBlockCommand(t, QueryBlocksCmd(), probe, nil, "block.height > 1", "--output", "yaml")
		if result.err != nil {
			t.Fatalf("query blocks YAML: %v", result.err)
		}
		var document map[string]any
		if err := yaml.Unmarshal([]byte(result.stdout), &document); err != nil {
			t.Fatalf("decode YAML output: %v", err)
		}
		if document["total_count"] != "1" {
			t.Fatalf("YAML total_count = %#v, want 1", document["total_count"])
		}
	})

	t.Run("short print is rejected", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New(), searchResponse: &coretypes.ResultBlockSearch{}}
		result := executeBlockCommand(t, QueryBlocksCmd(), probe, outputBoundaryShortWriter{}, "block.height > 7", "--output", "json")
		if !errors.Is(result.err, io.ErrShortWrite) {
			t.Fatalf("error = %v, want %v", result.err, io.ErrShortWrite)
		}
	})

	t.Run("print error", func(t *testing.T) {
		printErr := errors.New("block search output failed")
		probe := &blockRPCProbe{
			Client:         rpcclientmock.New(),
			searchResponse: &coretypes.ResultBlockSearch{},
		}
		result := executeBlockCommand(t, QueryBlocksCmd(), probe, outputBoundaryErrorWriter{err: printErr}, "block.height > 7", "--output", "json")
		if !errors.Is(result.err, printErr) {
			t.Fatalf("error = %v, want %v", result.err, printErr)
		}
	})
}

func TestQueryBlockCmdHeightSelection(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantHeight  int64
		latest      bool
		statusCalls int
	}{
		{name: "positional height", args: []string{"17", "--type", "height", "--output", "json"}, wantHeight: 17},
		{name: "height flag", args: []string{"--height", "19", "--output", "json"}, wantHeight: 19},
		{name: "latest height", args: []string{"--output", "json"}, wantHeight: 31, latest: true, statusCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &blockRPCProbe{
				Client:        rpcclientmock.New(),
				blockResponse: blockResult(test.wantHeight, []byte{0x01}),
			}
			if test.latest {
				probe.statusResponse = &coretypes.ResultStatus{SyncInfo: coretypes.SyncInfo{LatestBlockHeight: test.wantHeight}}
			}

			result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, test.args...)
			if result.err != nil {
				t.Fatalf("query block: %v", result.err)
			}
			if probe.blockCalls != 1 || probe.blockHeight != test.wantHeight {
				t.Fatalf("Block call = count %d height %d, want one call at %d", probe.blockCalls, probe.blockHeight, test.wantHeight)
			}
			if probe.statusCalls != test.statusCalls {
				t.Fatalf("Status calls = %d, want %d", probe.statusCalls, test.statusCalls)
			}
			requireBlockHeightOutput(t, result.stdout, test.wantHeight)
		})
	}
}

func TestQueryBlockCmdHeightFailures(t *testing.T) {
	t.Run("invalid positional height", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "not-a-height", "--type", "height")
		if result.err == nil || !bytes.Contains([]byte(result.err.Error()), []byte("failed to parse block height")) {
			t.Fatalf("error = %v, want height parse error", result.err)
		}
		if probe.blockCalls != 0 {
			t.Fatalf("invalid height made %d Block calls", probe.blockCalls)
		}
	})

	for _, value := range []string{"0", "-1"} {
		t.Run("non-positive positional height "+value, func(t *testing.T) {
			probe := &blockRPCProbe{Client: rpcclientmock.New()}
			args := []string{value, "--type", "height"}
			if value[0] == '-' {
				args = []string{"--type", "height", "--", value}
			}
			result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, args...)
			if result.err == nil || !bytes.Contains([]byte(result.err.Error()), []byte("block height must be positive")) {
				t.Fatalf("error = %v, want positive-height error", result.err)
			}
		})
	}

	t.Run("negative height flag", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "--height", "-1")
		if result.err == nil || result.err.Error() != "block height must be positive: -1" {
			t.Fatalf("error = %v, want negative-height error", result.err)
		}
	})

	t.Run("missing RPC client", func(t *testing.T) {
		cmd := QueryBlockCmd()
		nodeFlag := cmd.Flags().Lookup(cflags.FlagNode)
		if err := nodeFlag.Value.Set(""); err != nil {
			t.Fatalf("clear node default: %v", err)
		}
		err := executeBlockHandler(t, cmd, nil, "17")
		if err == nil || err.Error() != "no RPC client is defined in offline mode" {
			t.Fatalf("RunE() error = %v, want missing-RPC-client error", err)
		}
	})

	t.Run("latest height RPC error", func(t *testing.T) {
		statusErr := errors.New("status unavailable")
		probe := &blockRPCProbe{Client: rpcclientmock.New(), statusErr: statusErr}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil)
		if !errors.Is(result.err, statusErr) || !bytes.Contains([]byte(result.err.Error()), []byte("failed to get chain height")) {
			t.Fatalf("error = %v, want wrapped status error", result.err)
		}
		if probe.blockCalls != 0 {
			t.Fatalf("failed status made %d Block calls", probe.blockCalls)
		}
	})

	t.Run("latest status is missing", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil)
		if result.err == nil || !bytes.Contains([]byte(result.err.Error()), []byte("missing status result")) {
			t.Fatalf("error = %v, want missing-status error", result.err)
		}
	})

	t.Run("latest status has no committed height", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New(), statusResponse: &coretypes.ResultStatus{}}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil)
		if result.err == nil || !bytes.Contains([]byte(result.err.Error()), []byte("node has no committed block height")) {
			t.Fatalf("error = %v, want no-height error", result.err)
		}
	})

	t.Run("block RPC error", func(t *testing.T) {
		blockErr := errors.New("block unavailable")
		probe := &blockRPCProbe{Client: rpcclientmock.New(), blockErr: blockErr}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "17", "--type", "height")
		if !errors.Is(result.err, blockErr) {
			t.Fatalf("error = %v, want %v", result.err, blockErr)
		}
	})

	t.Run("zero-height response is not found", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New(), blockResponse: blockResult(0, []byte{0x01})}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "17", "--type", "height")
		if result.err == nil || result.err.Error() != "no block found with height 17" {
			t.Fatalf("error = %v, want exact not-found height", result.err)
		}
	})

	t.Run("nil block response is not found", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "17", "--type", "height")
		if result.err == nil || result.err.Error() != "no block found with height 17" {
			t.Fatalf("error = %v, want exact not-found height", result.err)
		}
	})

	t.Run("unencodable block is rejected", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New(), blockResponse: unencodableBlock(17)}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "17", "--type", "height")
		if result.err == nil || result.err.Error() != "invalid block returned for height 17" {
			t.Fatalf("error = %v, want encoding error", result.err)
		}
	})

	t.Run("print error", func(t *testing.T) {
		printErr := errors.New("block output failed")
		probe := &blockRPCProbe{Client: rpcclientmock.New(), blockResponse: blockResult(17, []byte{0x01})}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, outputBoundaryErrorWriter{err: printErr}, "17", "--type", "height", "--output", "json")
		if !errors.Is(result.err, printErr) {
			t.Fatalf("error = %v, want %v", result.err, printErr)
		}
	})
}

func TestQueryBlockCmdHashSelectionAndFailures(t *testing.T) {
	t.Run("hash success", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New(), hashResponse: blockResult(41, []byte{0x41})}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "a0b1", "--output", "json")
		if result.err != nil {
			t.Fatalf("query block by hash: %v", result.err)
		}
		if probe.hashCalls != 1 || !bytes.Equal(probe.hash, []byte{0xa0, 0xb1}) {
			t.Fatalf("BlockByHash call = count %d hash %x, want one call for a0b1", probe.hashCalls, probe.hash)
		}
		requireBlockHeightOutput(t, result.stdout, 41)
	})

	t.Run("empty hash", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "--type", "hash", "")
		if result.err == nil || result.err.Error() != "block hash is required when --type=hash" {
			t.Fatalf("error = %v, want empty-hash error", result.err)
		}
		if probe.hashCalls != 0 {
			t.Fatalf("empty hash made %d BlockByHash calls", probe.hashCalls)
		}
	})

	t.Run("malformed hexadecimal hash", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "not-hex")
		if result.err == nil {
			t.Fatal("malformed hash was accepted")
		}
		if probe.hashCalls != 0 {
			t.Fatalf("malformed hash made %d BlockByHash calls", probe.hashCalls)
		}
	})

	t.Run("hash RPC error", func(t *testing.T) {
		hashErr := errors.New("block by hash unavailable")
		probe := &blockRPCProbe{Client: rpcclientmock.New(), hashErr: hashErr}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "a0b1")
		if !errors.Is(result.err, hashErr) {
			t.Fatalf("error = %v, want %v", result.err, hashErr)
		}
	})

	t.Run("nil block is not found", func(t *testing.T) {
		probe := &blockRPCProbe{
			Client:       rpcclientmock.New(),
			hashResponse: &coretypes.ResultBlock{},
		}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "a0b1")
		if result.err == nil || result.err.Error() != "no block found with hash a0b1" {
			t.Fatalf("error = %v, want upstream not-found error", result.err)
		}
	})

	t.Run("nil app hash is a valid block", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New(), hashResponse: blockResult(41, nil)}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "a0b1", "--output", "json")
		if result.err != nil {
			t.Fatalf("error = %v, want valid block", result.err)
		}
		requireBlockHeightOutput(t, result.stdout, 41)
	})

	t.Run("unencodable block is rejected", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New(), hashResponse: unencodableBlock(41)}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "a0b1")
		if result.err == nil || result.err.Error() != "invalid block returned for hash a0b1" {
			t.Fatalf("error = %v, want encoding error", result.err)
		}
	})

	t.Run("RunE rejects malformed hexadecimal hash", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		err := executeBlockHandler(t, QueryBlockCmd(), probe, "not-hex")
		if err == nil || !bytes.Contains([]byte(err.Error()), []byte("failed to decode block hash")) {
			t.Fatalf("RunE() error = %v, want hash decode error", err)
		}
	})

	t.Run("unknown type", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, nil, "a0b1", "--type", "unknown")
		if result.err == nil || !bytes.Contains([]byte(result.err.Error()), []byte("must be one of height, hash")) {
			t.Fatalf("error = %v, want unknown-type error", result.err)
		}
		if probe.hashCalls != 0 || probe.blockCalls != 0 {
			t.Fatalf("unknown type made block calls: height=%d hash=%d", probe.blockCalls, probe.hashCalls)
		}
	})

	t.Run("print error", func(t *testing.T) {
		printErr := errors.New("hash block output failed")
		probe := &blockRPCProbe{Client: rpcclientmock.New(), hashResponse: blockResult(41, []byte{0x41})}
		result := executeBlockCommand(t, QueryBlockCmd(), probe, outputBoundaryErrorWriter{err: printErr}, "a0b1", "--output", "json")
		if !errors.Is(result.err, printErr) {
			t.Fatalf("error = %v, want %v", result.err, printErr)
		}
	})
}

func TestQueryBlockResultsCmdHeightSelection(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantHeight  int64
		latest      bool
		statusCalls int
	}{
		{name: "positional height", args: []string{"23", "--output", "json"}, wantHeight: 23},
		{name: "height flag", args: []string{"--height", "24", "--output", "json"}, wantHeight: 24},
		{name: "latest height", args: []string{"--output", "json"}, wantHeight: 25, latest: true, statusCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &blockRPCProbe{
				Client:          rpcclientmock.New(),
				resultsResponse: &coretypes.ResultBlockResults{Height: test.wantHeight, AppHash: []byte{0x01}},
			}
			if test.latest {
				probe.statusResponse = &coretypes.ResultStatus{SyncInfo: coretypes.SyncInfo{LatestBlockHeight: test.wantHeight}}
			}

			result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, nil, test.args...)
			if result.err != nil {
				t.Fatalf("query block results: %v", result.err)
			}
			if probe.resultsCalls != 1 || probe.resultsHeight != test.wantHeight {
				t.Fatalf("BlockResults call = count %d height %d, want one call at %d", probe.resultsCalls, probe.resultsHeight, test.wantHeight)
			}
			if probe.statusCalls != test.statusCalls {
				t.Fatalf("Status calls = %d, want %d", probe.statusCalls, test.statusCalls)
			}

			var response struct {
				Height json.RawMessage `json:"height"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &response); err != nil {
				t.Fatalf("decode block-results output: %v\noutput: %s", err, result.stdout)
			}
			requireJSONInt64(t, response.Height, test.wantHeight)
		})
	}
}

func TestQueryBlockResultsCmdFailures(t *testing.T) {
	t.Run("invalid positional height", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, nil, "not-a-height")
		if result.err == nil {
			t.Fatal("invalid block-results height was accepted")
		}
		if probe.resultsCalls != 0 {
			t.Fatalf("invalid height made %d BlockResults calls", probe.resultsCalls)
		}
	})

	t.Run("too many positional heights", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, nil, "1", "2")
		if result.err == nil {
			t.Fatal("two heights were accepted")
		}
	})

	for _, value := range []string{"0", "-1"} {
		t.Run("non-positive positional height "+value, func(t *testing.T) {
			probe := &blockRPCProbe{Client: rpcclientmock.New()}
			args := []string{value}
			if value[0] == '-' {
				args = []string{"--", value}
			}
			result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, nil, args...)
			if result.err == nil || !bytes.Contains([]byte(result.err.Error()), []byte("block height must be positive")) {
				t.Fatalf("error = %v, want positive-height error", result.err)
			}
		})
	}

	t.Run("negative height flag", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, nil, "--height", "-1")
		if result.err == nil || result.err.Error() != "block height must be positive: -1" {
			t.Fatalf("error = %v, want negative-height error", result.err)
		}
	})

	t.Run("RunE rejects negative context height", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		cmd := QueryBlockResultsCmd()
		if err := cmd.Flags().Lookup(cflags.FlagHeight).Value.Set("-1"); err != nil {
			t.Fatalf("set height value: %v", err)
		}
		err := executeBlockHandler(t, cmd, probe)
		if err == nil || err.Error() != "block height must be positive: -1" {
			t.Fatalf("RunE() error = %v, want negative-height error", err)
		}
	})

	t.Run("missing RPC client", func(t *testing.T) {
		cmd := QueryBlockResultsCmd()
		nodeFlag := cmd.Flags().Lookup(cflags.FlagNode)
		if nodeFlag == nil {
			t.Fatal("block-results command has no node flag")
		}
		if err := nodeFlag.Value.Set(""); err != nil {
			t.Fatalf("clear node default: %v", err)
		}
		if nodeFlag.Changed {
			t.Fatal("direct flag value update unexpectedly marked --node changed")
		}

		err := executeBlockHandler(t, cmd, nil, "23")
		if err == nil || err.Error() != "no RPC client is defined in offline mode" {
			t.Fatalf("RunE() error = %v, want missing-RPC-client error", err)
		}
	})

	t.Run("latest height RPC error", func(t *testing.T) {
		statusErr := errors.New("status unavailable")
		probe := &blockRPCProbe{Client: rpcclientmock.New(), statusErr: statusErr}
		result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, nil)
		if !errors.Is(result.err, statusErr) || !bytes.Contains([]byte(result.err.Error()), []byte("failed to get chain height")) {
			t.Fatalf("error = %v, want wrapped status error", result.err)
		}
		if probe.resultsCalls != 0 {
			t.Fatalf("failed status made %d BlockResults calls", probe.resultsCalls)
		}
	})

	t.Run("latest lookup without RPC client", func(t *testing.T) {
		cmd := QueryBlockResultsCmd()
		nodeFlag := cmd.Flags().Lookup(cflags.FlagNode)
		if err := nodeFlag.Value.Set(""); err != nil {
			t.Fatalf("clear node default: %v", err)
		}
		err := executeBlockHandler(t, cmd, nil)
		if err == nil || err.Error() != "no RPC client is defined in offline mode" {
			t.Fatalf("RunE() error = %v, want missing-RPC-client error", err)
		}
	})

	t.Run("block-results RPC error", func(t *testing.T) {
		resultsErr := errors.New("block results unavailable")
		probe := &blockRPCProbe{Client: rpcclientmock.New(), resultsErr: resultsErr}
		result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, nil, "23")
		if !errors.Is(result.err, resultsErr) {
			t.Fatalf("error = %v, want %v", result.err, resultsErr)
		}
	})

	t.Run("missing block-results response", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New()}
		result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, nil, "23")
		if result.err == nil || result.err.Error() != "no block results found with height 23" {
			t.Fatalf("error = %v, want missing-results error", result.err)
		}
	})

	t.Run("print error", func(t *testing.T) {
		printErr := errors.New("block results output failed")
		probe := &blockRPCProbe{
			Client:          rpcclientmock.New(),
			resultsResponse: &coretypes.ResultBlockResults{Height: 23},
		}
		result := executeBlockCommand(t, QueryBlockResultsCmd(), probe, outputBoundaryErrorWriter{err: printErr}, "23", "--output", "json")
		if !errors.Is(result.err, printErr) {
			t.Fatalf("error = %v, want %v", result.err, printErr)
		}
	})
}

func TestBlockQueryAdditionalArgumentBoundariesStopBeforeRPC(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		want string
	}{
		{
			name: "blocks invalid order",
			cmd:  QueryBlocksCmd,
			args: []string{"block.height > 1", "--order_by", "dsc"},
			want: "must be one of , asc, desc",
		},
		{
			name: "block too many identifiers",
			cmd:  QueryBlockCmd,
			args: []string{"1", "2", "--type", "height"},
			want: "accepts at most 1 arg",
		},
		{
			name: "block positional and height flag",
			cmd:  QueryBlockCmd,
			args: []string{"1", "--height", "2"},
			want: "height cannot be supplied both positionally",
		},
		{
			name: "block zero height flag",
			cmd:  QueryBlockCmd,
			args: []string{"--height", "0"},
			want: "block height must be positive: 0",
		},
		{
			name: "block results positional and height flag",
			cmd:  QueryBlockResultsCmd,
			args: []string{"1", "--height", "2"},
			want: "height cannot be supplied both positionally",
		},
		{
			name: "block results zero height flag",
			cmd:  QueryBlockResultsCmd,
			args: []string{"--height", "0"},
			want: "block height must be positive: 0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &blockRPCProbe{Client: rpcclientmock.New()}
			result := executeBlockCommand(t, test.cmd(), probe, nil, test.args...)
			if result.err == nil || !bytes.Contains([]byte(result.err.Error()), []byte(test.want)) {
				t.Fatalf("error = %v, want error containing %q", result.err, test.want)
			}
			if calls := probe.searchCalls + probe.statusCalls + probe.blockCalls + probe.hashCalls + probe.resultsCalls; calls != 0 {
				t.Fatalf("invalid input made %d RPC calls", calls)
			}
		})
	}
}

func TestBlockQueryHandlersDefendAgainstInvalidProgrammaticArguments(t *testing.T) {
	tests := []struct {
		name  string
		cmd   func() *cobra.Command
		setup func(*testing.T, *cobra.Command)
		args  []string
		want  string
	}{
		{
			name: "block invalid height",
			cmd:  QueryBlockCmd,
			setup: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				if err := cmd.Flags().Set(cflags.FlagType, cflags.TypeHeight); err != nil {
					t.Fatalf("set block type: %v", err)
				}
			},
			args: []string{"not-a-height"},
			want: "failed to parse block height",
		},
		{
			name: "block negative context height",
			cmd:  QueryBlockCmd,
			setup: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				if err := cmd.Flags().Lookup(cflags.FlagHeight).Value.Set("-1"); err != nil {
					t.Fatalf("set height value: %v", err)
				}
			},
			want: "block height must be positive: -1",
		},
		{
			name:  "block results invalid height",
			cmd:   QueryBlockResultsCmd,
			setup: func(*testing.T, *cobra.Command) {},
			args:  []string{"not-a-height"},
			want:  "failed to parse block height",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.cmd()
			test.setup(t, cmd)
			probe := &blockRPCProbe{Client: rpcclientmock.New()}
			err := executeBlockHandler(t, cmd, probe, test.args...)
			if err == nil || !bytes.Contains([]byte(err.Error()), []byte(test.want)) {
				t.Fatalf("RunE() error = %v, want error containing %q", err, test.want)
			}
			if calls := probe.searchCalls + probe.statusCalls + probe.blockCalls + probe.hashCalls + probe.resultsCalls; calls != 0 {
				t.Fatalf("invalid handler arguments made %d RPC calls", calls)
			}
		})
	}
}

func TestBlockQueryRPCsReceiveTheCommandContext(t *testing.T) {
	type contextKey struct{}
	marker := &struct{}{}
	base := context.WithValue(context.Background(), contextKey{}, marker)

	t.Run("block search", func(t *testing.T) {
		probe := &blockRPCProbe{
			Client:         rpcclientmock.New(),
			searchResponse: &coretypes.ResultBlockSearch{},
		}
		result := executeBlockCommandOnContext(t, base, QueryBlocksCmd(), probe, nil, "block.height > 1")
		if result.err != nil {
			t.Fatalf("query blocks: %v", result.err)
		}
		if probe.searchContext.Value(contextKey{}) != marker {
			t.Fatal("BlockSearch did not receive the command context")
		}
	})

	t.Run("latest block", func(t *testing.T) {
		probe := &blockRPCProbe{
			Client:         rpcclientmock.New(),
			statusResponse: &coretypes.ResultStatus{SyncInfo: coretypes.SyncInfo{LatestBlockHeight: 31}},
			blockResponse:  blockResult(31, nil),
		}
		result := executeBlockCommandOnContext(t, base, QueryBlockCmd(), probe, nil)
		if result.err != nil {
			t.Fatalf("query latest block: %v", result.err)
		}
		if probe.statusContext.Value(contextKey{}) != marker || probe.blockContext.Value(contextKey{}) != marker {
			t.Fatal("Status or Block did not receive the command context")
		}
	})

	t.Run("block hash", func(t *testing.T) {
		probe := &blockRPCProbe{Client: rpcclientmock.New(), hashResponse: blockResult(32, nil)}
		result := executeBlockCommandOnContext(t, base, QueryBlockCmd(), probe, nil, "a0b1")
		if result.err != nil {
			t.Fatalf("query block by hash: %v", result.err)
		}
		if probe.hashContext.Value(contextKey{}) != marker {
			t.Fatal("BlockByHash did not receive the command context")
		}
	})

	t.Run("block results", func(t *testing.T) {
		probe := &blockRPCProbe{
			Client:          rpcclientmock.New(),
			resultsResponse: &coretypes.ResultBlockResults{Height: 33},
		}
		result := executeBlockCommandOnContext(t, base, QueryBlockResultsCmd(), probe, nil, "33")
		if result.err != nil {
			t.Fatalf("query block results: %v", result.err)
		}
		if probe.resultsContext.Value(contextKey{}) != marker {
			t.Fatal("BlockResults did not receive the command context")
		}
	})
}

func TestBlockQueriesRenderYAMLThroughCobraStdout(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		set  func(*blockRPCProbe)
		want string
	}{
		{
			name: "blocks",
			cmd:  QueryBlocksCmd,
			args: []string{"block.height = 90", "--output", "yaml"},
			set: func(probe *blockRPCProbe) {
				probe.searchResponse = &coretypes.ResultBlockSearch{Blocks: []*coretypes.ResultBlock{blockResult(90, nil)}, TotalCount: 1}
			},
			want: "total_count:",
		},
		{
			name: "block",
			cmd:  QueryBlockCmd,
			args: []string{"90", "--type", "height", "--output", "yaml"},
			set: func(probe *blockRPCProbe) {
				probe.blockResponse = blockResult(90, nil)
			},
			want: "height:",
		},
		{
			name: "block results",
			cmd:  QueryBlockResultsCmd,
			args: []string{"90", "--output", "yaml"},
			set: func(probe *blockRPCProbe) {
				probe.resultsResponse = &coretypes.ResultBlockResults{Height: 90}
			},
			want: "height: 90",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &blockRPCProbe{Client: rpcclientmock.New()}
			test.set(probe)
			result := executeBlockCommand(t, test.cmd(), probe, nil, test.args...)
			if result.err != nil {
				t.Fatalf("render YAML: %v", result.err)
			}
			if !bytes.Contains([]byte(result.stdout), []byte(test.want)) {
				t.Fatalf("YAML output %q does not contain %q", result.stdout, test.want)
			}
			if json.Valid([]byte(result.stdout)) {
				t.Fatalf("--output yaml emitted JSON: %s", result.stdout)
			}
		})
	}
}

func TestBlockAndBlockResultsReturnShortWriteErrors(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		set  func(*blockRPCProbe)
	}{
		{
			name: "block",
			cmd:  QueryBlockCmd,
			args: []string{"2", "--type", "height", "--output", "json"},
			set: func(probe *blockRPCProbe) {
				probe.blockResponse = blockResult(2, nil)
			},
		},
		{
			name: "block results",
			cmd:  QueryBlockResultsCmd,
			args: []string{"2", "--output", "json"},
			set: func(probe *blockRPCProbe) {
				probe.resultsResponse = &coretypes.ResultBlockResults{Height: 2}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &blockRPCProbe{Client: rpcclientmock.New()}
			test.set(probe)
			result := executeBlockCommand(t, test.cmd(), probe, outputBoundaryShortWriter{}, test.args...)
			if !errors.Is(result.err, io.ErrShortWrite) {
				t.Fatalf("error = %v, want io.ErrShortWrite", result.err)
			}
		})
	}
}
