package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestFormatDuration(t *testing.T) {
	tests := map[string]struct {
		d time.Duration
	}{
		"Milliseconds": {500 * time.Millisecond},
		"Seconds":      {3500 * time.Millisecond},
		"Minutes":      {2*time.Minute + 30*time.Second},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatDuration(tc.d))
		})
	}
}

func TestFormatNumber(t *testing.T) {
	tests := map[string]struct {
		n int64
	}{
		"Small":     {999},
		"Thousands": {18234},
		"Millions":  {18234567},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatNumber(tc.n))
		})
	}
}

func TestFormatPower(t *testing.T) {
	tests := map[string]struct {
		power int64
	}{
		"Raw":  {500},
		"Kilo": {15000},
		"Mega": {5000000},
		"Giga": {2500000000},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatPower(tc.power))
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := map[string]struct {
		bytes uint64
	}{
		"MiB": {512 * 1024 * 1024},
		"GiB": {64 * 1024 * 1024 * 1024},
		"TiB": {2 * 1024 * 1024 * 1024 * 1024},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatBytes(tc.bytes))
		})
	}
}

func TestFormatResourceRatio(t *testing.T) {
	tests := map[string]struct {
		avail uint64
		total uint64
	}{
		"Normal":    {8, 16},
		"ZeroTotal": {0, 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatResourceRatio(tc.avail, tc.total))
		})
	}
}

func TestFormatMemoryRatio(t *testing.T) {
	tests := map[string]struct {
		avail uint64
		total uint64
	}{
		"Normal":    {32 * 1024 * 1024 * 1024, 64 * 1024 * 1024 * 1024},
		"ZeroTotal": {0, 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatMemoryRatio(tc.avail, tc.total))
		})
	}
}

func TestFormatProviderURL(t *testing.T) {
	tests := map[string]struct {
		hostURI string
		maxLen  int
	}{
		"Short":         {"https://short.com:8443", 30},
		"Long":          {"https://very-long-provider-name.example.com:8443", 20},
		"HTTPSStripped": {"https://provider.akash.network:8443", 34},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatProviderURL(tc.hostURI, tc.maxLen))
		})
	}
}

func TestTruncateAddress(t *testing.T) {
	tests := map[string]struct {
		addr   string
		maxLen int
	}{
		"Short": {"ABCDEF", 12},
		"Long":  {"ABCDEF1234567890ABCDEF", 12},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, truncateAddress(tc.addr, tc.maxLen))
		})
	}
}

func TestStripEmojis(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"NoEmojis":   {"Cosmostation"},
		"WithEmojis": {"Cosmo \U0001f680 Station \u2728"},
		"OnlyEmojis": {"\U0001f680\u2728\U0001f31f"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, stripEmojis(tc.input))
		})
	}
}

func TestCenterAlign(t *testing.T) {
	tests := map[string]struct {
		s     string
		width int
	}{
		"Narrow": {"x", 5},
		"Exact":  {"hello", 5},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, centerAlign(tc.s, tc.width))
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"NoANSI":   {"plain text"},
		"WithANSI": {"\x1b[31mred text\x1b[0m"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, stripANSI(tc.input))
		})
	}
}

func TestFormatNodeGPU(t *testing.T) {
	tests := map[string]struct {
		node rpc.ProviderNodeWithGPU
	}{
		"NoGPU": {rpc.ProviderNodeWithGPU{GPUAllocatable: 0}},
		"WithGPU": {rpc.ProviderNodeWithGPU{
			GPUAllocatable: 4,
			GPUAvailable:   2,
			GPUs:           []rpc.GPUInfo{{Vendor: "nvidia", Name: "H100", MemorySize: "80Gi"}},
		}},
		"WithModel": {rpc.ProviderNodeWithGPU{
			GPUAllocatable: 8,
			GPUAvailable:   4,
			GPUs:           []rpc.GPUInfo{{Vendor: "amd", Name: "MI300X", MemorySize: "192Gi"}},
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatNodeGPU(tc.node))
		})
	}
}

func TestFormatGPUModel(t *testing.T) {
	tests := map[string]struct {
		gpu rpc.GPUInfo
	}{
		"NVIDIA":        {rpc.GPUInfo{Vendor: "nvidia", Name: "H100", MemorySize: "80Gi"}},
		"LongTruncated": {rpc.GPUInfo{Vendor: "nvidia", Name: "Very Long GPU Model Name That Exceeds Limits", MemorySize: "80Gi"}},
		"WithMemory":    {rpc.GPUInfo{Vendor: "amd", Name: "MI300X", MemorySize: "192Gi"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatGPUModel(tc.gpu))
		})
	}
}

func TestFormatProviderGPU(t *testing.T) {
	tests := map[string]struct {
		p rpc.Provider
	}{
		"NoGPU":      {rpc.Provider{GPUTotal: 0}},
		"WithModels": {rpc.Provider{GPUAvailable: 2, GPUTotal: 4, GPUModels: []string{"H100"}}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, formatProviderGPU(tc.p))
		})
	}
}

// Row-level renderer tests

func TestRenderBlockRow(t *testing.T) {
	tests := map[string]struct {
		data blockRowData
	}{
		"Current": {blockRowData{
			height: 18234567, prevotePct: 0.95, precommPct: 0.92,
			elapsed: 3500 * time.Millisecond, round: 0, step: 1,
			isCurrent: true, isSelected: false, heightW: 14,
		}},
		"Historical": {blockRowData{
			height: 18234566, prevotePct: 0.98, precommPct: 0.95,
			elapsed: 5 * time.Second, round: 0, step: 1,
			isCurrent: false, isSelected: false, heightW: 14,
		}},
		"Selected": {blockRowData{
			height: 18234566, prevotePct: 0.98, precommPct: 0.95,
			elapsed: 5 * time.Second, round: 0, step: 1,
			isCurrent: false, isSelected: true, heightW: 14,
		}},
		"HighRound": {blockRowData{
			height: 18234560, prevotePct: 0.4, precommPct: 0.1,
			elapsed: 30 * time.Second, round: 3, step: 2,
			isCurrent: true, isSelected: false, heightW: 14,
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderBlockRow(tc.data))
		})
	}
}

func TestRenderSigningBar(t *testing.T) {
	tests := map[string]struct {
		history         []bool
		validatorIdx    int
		proposerHistory []int
		currentProposer int
		width           int
	}{
		"AllSigned":    {[]bool{true, true, true, true, true}, 0, []int{1, 2, 3, 4, 5}, -1, 10},
		"AllMissed":    {[]bool{false, false, false, false, false}, 0, []int{1, 2, 3, 4, 5}, -1, 10},
		"Mixed":        {[]bool{true, false, true, true, false}, 0, []int{1, 2, 3, 4, 5}, -1, 10},
		"WithProposer": {[]bool{true, true, true, true, true}, 0, []int{0, 1, 0, 2, 3}, -1, 10},
		"Empty":        {[]bool{}, 0, nil, -1, 10},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderSigningBar(tc.history, tc.validatorIdx, tc.proposerHistory, tc.currentProposer, tc.width))
		})
	}
}

func TestRenderVersionLine(t *testing.T) {
	tests := map[string]struct {
		version  string
		count    int
		total    int
		selected string
	}{
		"Selected":   {"0.6.4", 10, 20, "0.6.4"},
		"Unselected": {"0.6.3", 8, 20, "0.6.4"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderVersionLine(tc.version, tc.count, tc.total, tc.selected))
		})
	}
}

func TestVoteIndicator(t *testing.T) {
	tests := map[string]struct {
		voted bool
	}{
		"Voted":    {true},
		"NotVoted": {false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, voteIndicator(tc.voted))
		})
	}
}
