package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestMonitorPreservesValidatorIdentifiers(t *testing.T) {
	address := strings.Repeat("ABCDEF0123", 4)
	key := strings.Repeat("abcdef012345", 6)
	v := consensus.ValidatorStatus{Address: address, PubKey: key, VotingPower: 100}
	state := &consensus.State{ProposerAddress: address, Validators: []consensus.ValidatorStatus{v}}
	m := Model{width: 120, height: 40, state: state, validatorTable: newTestValidatorTableModel(nil, nil, nil, nil)}
	m.rebuildValidatorTableRows()
	m.resizeComponents()
	for name, output := range map[string]string{
		"consensus":   renderConsensusSection(state),
		"list":        m.validatorTable.View(),
		"row":         renderValidatorRowWithBlocks(v, nil, nil, nil, 100, 16, 20, false),
		"block votes": renderExpandedValidators([]BlockValidatorVote{{Address: address}}, nil, 20, 0, 60),
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(ansi.Strip(output), address) {
				t.Errorf("full address missing: %s", output)
			}
		})
	}
	detail := ansi.Strip(renderValidatorDetailPanel(v, nil, nil, 100, 60))
	if !strings.Contains(detail, key) {
		t.Errorf("full public key missing: %s", detail)
	}
}

func TestMonitorPreservesProviderIdentifiers(t *testing.T) {
	for _, url := range []string{
		"https://very-long-provider-name.example.com:8443/path/to/provider",
		"http://[2001:db8:1234:5678::1]:8443/gateway",
	} {
		t.Run(url, func(t *testing.T) {
			p := rpc.Provider{HostURI: url, AkashVersion: "0.6.4"}
			node := rpc.ProviderNodeWithGPU{Name: "worker-node-with-a-long-unique-name-0123456789"}
			m := Model{
				width: 120, height: 40,
				providers:     ProviderList{Items: []rpc.Provider{p}, Version: p.AkashVersion},
				detail:        ProviderDetail{Provider: &p, Nodes: []rpc.ProviderNodeWithGPU{node}},
				providerTable: newTestProviderTableModel(nil),
				nodeTable:     newTestNodeTableModel(nil),
			}
			m.rebuildProviderTableRows()
			m.rebuildNodeTableRows()
			m.resizeComponents()
			ctx := ViewContext{Providers: ProviderViewState{Detail: ProviderDetailState{Provider: &p, Nodes: []rpc.ProviderNodeWithGPU{node}}}, NodeTable: m.nodeTable}
			for name, output := range map[string]string{
				"list":   m.providerTable.View(),
				"row":    renderProviderRow(p, 1, p.AkashVersion, false),
				"detail": renderProviderDetailView(ctx),
			} {
				t.Run(name, func(t *testing.T) {
					if !strings.Contains(ansi.Strip(output), url) {
						t.Errorf("full URL missing: %s", output)
					}
				})
			}
			if output := ansi.Strip(m.nodeTable.View()); !strings.Contains(output, node.Name) {
				t.Errorf("full node name missing: %s", output)
			}
		})
	}
}

func TestMonitorPreservesGPUModel(t *testing.T) {
	name := "NVIDIA H100 80GB HBM3 PCIe"
	if output := formatProviderGPU(rpc.Provider{GPUTotal: 1, GPUModels: []string{name}}); !strings.Contains(output, name) {
		t.Errorf("provider omitted full GPU model: %s", output)
	}
	want := name + " (80Gi)"
	if output := formatGPUModel(rpc.GPUInfo{Name: name, Vendor: "nvidia", MemorySize: "80Gi"}); output != want {
		t.Errorf("GPU description = %q, want %q", output, want)
	}
}
