// Package bootstrap handles first-run initialization of the akt config.
// It fetches network definitions from the akash-network/net GitHub repo
// and creates a config.yaml with user-selected networks.
package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/glyphs"
)

const (
	netRepoAPI = "https://api.github.com/repos/akash-network/net/contents"
	netRepoRaw = "https://raw.githubusercontent.com/akash-network/net/main"
)

type githubEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type metaJSON struct {
	ChainName   string `json:"chain_name"`
	ChainID     string `json:"chain_id"`
	PrettyName  string `json:"pretty_name"`
	NetworkType string `json:"network_type"`
	Status      string `json:"status"`
	Fees        struct {
		FeeTokens []struct {
			Denom           string  `json:"denom"`
			AverageGasPrice float64 `json:"average_gas_price"`
		} `json:"fee_tokens"`
	} `json:"fees"`
	APIs struct {
		RPC []struct {
			Address string `json:"address"`
		} `json:"rpc"`
		REST []struct {
			Address string `json:"address"`
		} `json:"rest"`
		GRPC []struct {
			Address string `json:"address"`
		} `json:"grpc"`
	} `json:"apis"`
}

// Run performs first-run initialization.
func Run(cfgRoot string) error {
	// The wizard is interactive by design. Without a terminal it must not
	// silently fetch networks and write a config with fallback answers
	// (SPEC §2.0: no TTY -> print and exit); headless environments create
	// their config explicitly via akt context/network commands.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "no akt configuration found and no terminal is available for the first-run wizard;")
		fmt.Fprintln(os.Stderr, "run akt from a terminal to bootstrap, or create a config with \"akt context network create\" and \"akt context create\"")
		return nil
	}

	fmt.Println("Welcome to akt! No configuration found.")
	fmt.Println()
	fmt.Println("Fetching available networks from github.com/akash-network/net ...")
	fmt.Println()

	networks, err := fetchAllNetworks()
	if err != nil {
		return fmt.Errorf("fetch networks: %w", err)
	}

	if len(networks) == 0 {
		return fmt.Errorf("no networks found in akash-network/net repo")
	}

	// Interactive multi-select.
	selected := multiSelect(networks)
	if len(selected) == 0 {
		fmt.Println("No networks selected. Aborted.")
		os.Exit(0)
	}

	// Keyring backend selection.
	backend := selectKeyringBackend()

	// Pick current-context: prefer mainnet, else first selected.
	currentCtx := selected[0].Name
	for _, n := range selected {
		if n.Name == "mainnet" || strings.Contains(n.Name, "mainnet") {
			currentCtx = n.Name
			break
		}
	}

	cfg := &aktctx.Config{
		Version:        aktctx.ConfigVersion,
		CurrentContext: currentCtx,
		Networks:       selected,
		Keyrings: []aktctx.Keyring{
			{Name: "default", Backend: backend},
		},
		Defaults: aktctx.Defaults{
			Output:        "pretty",
			BroadcastMode: "sync",
		},
	}

	for _, n := range selected {
		cfg.Contexts = append(cfg.Contexts, aktctx.Context{
			Name:    n.Name,
			Network: aktctx.Network{Name: n.Name},
			Keyring: aktctx.Keyring{Name: "default"},
			Gas:     "auto",
			ProviderDefaults: aktctx.ProviderDefaults{
				AuthType: "jwt",
			},
		})
	}

	// Optional Console API key onboarding for the initial context.
	consoleKey, routeConsole := consoleOnboarding(currentCtx)
	if routeConsole {
		for i := range cfg.Contexts {
			if cfg.Contexts[i].Name == currentCtx {
				cfg.Contexts[i].AuthMethod = aktctx.AuthMethodConsoleAPI
			}
		}
	}

	if err := aktctx.SaveConfig(cfgRoot, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// The credential lives outside config.yaml (SPEC §7.1).
	if consoleKey != "" {
		if err := aktctx.SetConsoleAPIKey(cfgRoot, currentCtx, consoleKey); err != nil {
			return fmt.Errorf("store console api key: %w", err)
		}

		fmt.Printf("Console API key stored for context %q.\n", currentCtx)
	}

	fmt.Printf("\nConfig written to %s\n", aktctx.ConfigPath(cfgRoot))
	fmt.Printf("Active context: %s\n\n", currentCtx)

	return nil
}

// ANSI escape helpers.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiBgSel  = "\033[48;5;236m" // subtle dark bg for selected row
)

// multiSelect presents an interactive multi-select for networks.
// "Select all" is the first item. Everything is selected by default.
func multiSelect(networks []aktctx.Network) []aktctx.Network {
	n := len(networks)
	checked := make([]bool, n)
	for i := range checked {
		checked[i] = true // all selected by default
	}

	cursor := 0 // 0 = "Select all", 1..n = networks
	totalItems := n + 1

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return networks
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	render := func() {
		g := glyphs.G()
		var b strings.Builder

		b.WriteString(ansiBold + "Select networks" + ansiReset + ansiDim + "  ↑↓ move  space toggle  enter confirm" + ansiReset + "\r\n")
		b.WriteString("\r\n")

		// "Select all" row — index 0.
		{
			allOn := allSelected(checked)
			icon := ansiDim + g.CheckboxOff + ansiReset
			if allOn {
				icon = ansiGreen + g.CheckboxOn + ansiReset
			}
			prefix := "  "
			rowStart := ""
			rowEnd := ""
			if cursor == 0 {
				prefix = ansiYellow + g.Cursor + " " + ansiReset
				rowStart = ansiBgSel
				rowEnd = ansiReset
			}
			b.WriteString(fmt.Sprintf("  %s%s %s  %s%s%s\r\n",
				rowStart, prefix, icon, ansiBold+g.SelectAll+" Select all"+ansiReset, "", rowEnd))
		}

		b.WriteString("\r\n")

		// Network rows — indices 1..n.
		for i, net := range networks {
			icon := ansiDim + g.CheckboxOff + ansiReset
			nameStyle := ansiDim
			if checked[i] {
				icon = ansiGreen + g.CheckboxOn + ansiReset
				nameStyle = ansiReset
			}
			prefix := "  "
			rowStart := ""
			rowEnd := ""
			if cursor == i+1 {
				prefix = ansiYellow + g.Cursor + " " + ansiReset
				rowStart = ansiBgSel
				rowEnd = ansiReset
			}

			info := fmt.Sprintf("%s%-18s %s%s",
				ansiCyan, net.ChainID+ansiReset,
				ansiDim, fmt.Sprintf("[%d rpc, %d api, %d grpc]", len(net.Endpoints.RPC), len(net.Endpoints.API), len(net.Endpoints.GRPC))+ansiReset)

			b.WriteString(fmt.Sprintf("  %s%s %s  %s%-20s  %s%s\r\n",
				rowStart, prefix, icon, nameStyle, net.Name+ansiReset, info, rowEnd))
		}

		b.WriteString("\r\n")
		b.WriteString(ansiDim + "  q quit" + ansiReset + "\r\n")

		os.Stdout.WriteString(b.String())
	}

	// Total rendered lines: header(1) + blank(1) + select-all(1) + blank(1) + networks(n) + blank(1) + hint(1) = n+6
	renderLines := n + 6

	clear := func() {
		for i := 0; i < renderLines; i++ {
			os.Stdout.WriteString("\033[A\033[2K")
		}
	}

	render()

	buf := make([]byte, 3)
	for {
		nr, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		if nr == 1 {
			switch buf[0] {
			case ' ':
				clear()
				if cursor == 0 {
					allOn := allSelected(checked)
					for i := range checked {
						checked[i] = !allOn
					}
				} else {
					checked[cursor-1] = !checked[cursor-1]
				}
				render()
			case '\r', '\n':
				os.Stdout.WriteString("\r\n")
				term.Restore(int(os.Stdin.Fd()), oldState)

				var result []aktctx.Network
				for i, net := range networks {
					if checked[i] {
						result = append(result, net)
					}
				}
				return result
			case 'j':
				clear()
				cursor = (cursor + 1) % totalItems
				render()
			case 'k':
				clear()
				cursor = (cursor - 1 + totalItems) % totalItems
				render()
			case 'q', 3:
				os.Stdout.WriteString("\r\n")
				term.Restore(int(os.Stdin.Fd()), oldState)
				fmt.Println("Aborted.")
				os.Exit(0)
			}
		} else if nr == 3 && buf[0] == 27 && buf[1] == 91 {
			clear()
			switch buf[2] {
			case 65: // Up
				cursor = (cursor - 1 + totalItems) % totalItems
			case 66: // Down
				cursor = (cursor + 1) % totalItems
			}
			render()
		}
	}

	var result []aktctx.Network
	for i, net := range networks {
		if checked[i] {
			result = append(result, net)
		}
	}
	return result
}

func allSelected(checked []bool) bool {
	for _, c := range checked {
		if !c {
			return false
		}
	}
	return true
}

// keyringOption describes one selectable keyring backend.
type keyringOption struct {
	value string
	label string
	desc  string
}

// selectKeyringBackend presents a single-select menu for keyring backend.
// Returns the selected backend string (e.g. "os", "file", "test").
func selectKeyringBackend() string {
	options := []keyringOption{
		{value: "os", label: "os", desc: "System keyring (recommended)"},
		{value: "file", label: "file", desc: "File-based encrypted keyring"},
		{value: "test", label: "test", desc: "Unencrypted test keyring (development only)"},
	}

	cursor := 0 // default to "os"

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "os" // fallback
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	render := func() {
		g := glyphs.G()
		var b strings.Builder

		b.WriteString(ansiBold + "Select keyring backend" + ansiReset + ansiDim + "  ↑↓ move  enter confirm" + ansiReset + "\r\n")
		b.WriteString("\r\n")

		for i, opt := range options {
			icon := ansiDim + g.CheckboxOff + ansiReset
			nameStyle := ansiDim
			prefix := "  "
			rowStart := ""
			rowEnd := ""
			if i == cursor {
				icon = ansiGreen + g.CheckboxOn + ansiReset
				nameStyle = ansiReset
				prefix = ansiYellow + g.Cursor + " " + ansiReset
				rowStart = ansiBgSel
				rowEnd = ansiReset
			}

			b.WriteString(fmt.Sprintf("  %s%s %s  %s%-10s %s%s%s\r\n",
				rowStart, prefix, icon, nameStyle, opt.label+ansiReset,
				ansiDim, opt.desc+ansiReset, rowEnd))
		}

		b.WriteString("\r\n")
		os.Stdout.WriteString(b.String())
	}

	// header(1) + blank(1) + options(3) + blank(1) = 6
	renderLines := len(options) + 3

	clear := func() {
		for i := 0; i < renderLines; i++ {
			os.Stdout.WriteString("\033[A\033[2K")
		}
	}

	render()

	buf := make([]byte, 3)
	totalItems := len(options)
	for {
		nr, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		if nr == 1 {
			switch buf[0] {
			case '\r', '\n':
				os.Stdout.WriteString("\r\n")
				term.Restore(int(os.Stdin.Fd()), oldState)
				return options[cursor].value
			case 'j':
				clear()
				cursor = (cursor + 1) % totalItems
				render()
			case 'k':
				clear()
				cursor = (cursor - 1 + totalItems) % totalItems
				render()
			case 'q', 3:
				os.Stdout.WriteString("\r\n")
				term.Restore(int(os.Stdin.Fd()), oldState)
				fmt.Println("Aborted.")
				os.Exit(0)
			}
		} else if nr == 3 && buf[0] == 27 && buf[1] == 91 {
			clear()
			switch buf[2] {
			case 65: // Up
				cursor = (cursor - 1 + totalItems) % totalItems
			case 66: // Down
				cursor = (cursor + 1) % totalItems
			}
			render()
		}
	}

	return options[cursor].value
}

// --- Network fetching ---

func fetchAllNetworks() ([]aktctx.Network, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	dirs, err := listRepoDirs(client)
	if err != nil {
		return nil, err
	}

	var networks []aktctx.Network
	for _, dir := range dirs {
		meta, err := fetchMeta(client, dir)
		if err != nil {
			continue
		}

		n := metaToNetwork(dir, meta)
		if n.ChainID == "" {
			continue
		}
		networks = append(networks, n)
	}

	return networks, nil
}

func listRepoDirs(client *http.Client) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, netRepoAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var entries []githubEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}

	var dirs []string
	for _, e := range entries {
		if e.Type == "dir" {
			dirs = append(dirs, e.Name)
		}
	}
	return dirs, nil
}

func fetchMeta(client *http.Client, dir string) (*metaJSON, error) {
	url := fmt.Sprintf("%s/%s/meta.json", netRepoRaw, dir)

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("meta.json returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var meta metaJSON
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func metaToNetwork(dirName string, meta *metaJSON) aktctx.Network {
	n := aktctx.Network{
		Name:    dirName,
		ChainID: meta.ChainID,
	}

	for _, rpc := range meta.APIs.RPC {
		if rpc.Address != "" {
			n.Endpoints.RPC = append(n.Endpoints.RPC, rpc.Address)
		}
	}

	for _, rest := range meta.APIs.REST {
		if rest.Address != "" {
			n.Endpoints.API = append(n.Endpoints.API, rest.Address)
		}
	}

	for _, grpc := range meta.APIs.GRPC {
		if grpc.Address != "" {
			n.Endpoints.GRPC = append(n.Endpoints.GRPC, grpc.Address)
		}
	}

	if len(meta.Fees.FeeTokens) > 0 {
		ft := meta.Fees.FeeTokens[0]
		if ft.AverageGasPrice > 0 && ft.Denom != "" {
			n.GasPrices = fmt.Sprintf("%g%s", ft.AverageGasPrice, ft.Denom)
		}
	}

	n.GasAdjustment = "1.5"

	return n
}
