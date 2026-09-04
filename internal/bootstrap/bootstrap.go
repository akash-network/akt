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
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

const (
	netRepoAPI = "https://api.github.com/repos/akash-network/net/contents"
	netRepoRaw = "https://raw.githubusercontent.com/akash-network/net/main"

	// defaultKeyringName is the name of the single keyring the wizard
	// creates. Every generated context references it.
	defaultKeyringName = "default"

	// pendingSuffix marks a path the wizard reports but does not create.
	// SaveConfig creates only the config root, so the context, store, and
	// action log directories do not exist when the summary prints; naming
	// them without this would be a claim about the filesystem that is not
	// yet true.
	pendingSuffix = "  (created on first use)"
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
			Denom        string  `json:"denom"`
			HighGasPrice float64 `json:"high_gas_price"`
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
	Faucets []struct {
		URL string `json:"url"`
	} `json:"faucets"`
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

	// Every byte the wizard emits is a prompt, progress, or a report about
	// them -- never data (SPEC §3.9.2, §10.1.1). stdout stays empty.
	out := os.Stderr

	fmt.Fprintln(out, "Welcome to akt! No configuration found.")
	fmt.Fprintln(out)

	// Say where this is going before asking anything. Someone who abandons
	// the wizard, or who has AKT_HOME/XDG_CONFIG_HOME set by another tool,
	// must not have to finish setup to learn which path won (SPEC §2.0a).
	writeDestination(out, cfgRoot)

	fmt.Fprintln(out, "Fetching available networks from github.com/akash-network/net ...")
	fmt.Fprintln(out)

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
		fmt.Fprintln(out, "No networks selected. Aborted.")
		os.Exit(0)
	}

	// Keyring backend selection.
	backend := selectKeyringBackend()

	// Which context is active is asked, never assumed: the answer decides
	// whether the user's first transaction spends real AKT (SPEC §2.0d).
	currentCtx := selectActiveContext(selected)

	cfg := &aktctx.Config{
		Version:        aktctx.ConfigVersion,
		CurrentContext: currentCtx,
		Networks:       selected,
		Keyrings: []aktctx.Keyring{
			{Name: defaultKeyringName, Backend: backend},
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
			Keyring: aktctx.Keyring{Name: defaultKeyringName},
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

		fmt.Fprintf(out, "Console API key stored for context %q.\n", currentCtx)
	}

	writeSummary(out, cfgRoot, currentCtx, chainIDOf(selected, currentCtx), backend)

	// Last, so it is still on screen when the user goes looking for the
	// keys the legacy CLI left behind (SPEC §1.12, §2.0g). A failure to
	// resolve the home directory only means no notice -- never an error.
	if home, err := os.UserHomeDir(); err == nil {
		writeLegacyNotice(out, detectLegacyHome(home), cfgRoot, backend)
	}

	return nil
}

// writeDestination announces the resolved config root and the overrides that
// relocate it, before the first prompt (SPEC §2.0a).
func writeDestination(w io.Writer, cfgRoot string) {
	for _, line := range destinationLines(cfgRoot) {
		fmt.Fprintln(w, line)
	}

	fmt.Fprintln(w)
}

// destinationLines is the announcement body. It is separated from rendering
// so its content -- the part that has to name the right path and the right
// overrides -- is assertable without a terminal.
func destinationLines(cfgRoot string) []string {
	return []string{
		"Configuration directory:  " + cfgRoot,
		"Config file to write:     " + aktctx.ConfigPath(cfgRoot),
		"",
		"Override the location with --home <dir> or AKT_HOME=<dir>",
		"(resolution order: --home, AKT_HOME, $XDG_CONFIG_HOME/akt, ~/.config/akt).",
		"Nothing is written until the prompts below are complete.",
	}
}

// writeSummary reports what was written and where the rest of this context's
// state will live (SPEC §2.0f).
func writeSummary(w io.Writer, cfgRoot, currentCtx, chainID, backend string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, ansiBold+"Setup complete"+ansiReset)
	fmt.Fprintln(w)

	for _, line := range summaryLines(cfgRoot, currentCtx, chainID, backend) {
		fmt.Fprintln(w, line)
	}

	fmt.Fprintln(w)
}

// summaryLines renders the closing summary as plain text. Labels match
// `akt context show` so the two describe the same thing the same way.
//
// No generated context gets a default-account, so the summary ends with the
// step that makes the configuration usable rather than leaving the user to
// discover the gap on their first command.
func summaryLines(cfgRoot, currentCtx, chainID, backend string) []string {
	active := currentCtx
	if chainID != "" {
		active = fmt.Sprintf("%s (%s)", currentCtx, chainID)
	}

	kv := func(label, value string) string {
		return fmt.Sprintf("  %-16s %s", label, value)
	}

	return []string{
		kv("Config", aktctx.ConfigPath(cfgRoot)),
		kv("Active Context", active),
		kv("Context Dir", aktctx.ContextDir(cfgRoot, currentCtx)+pendingSuffix),
		kv("Store", aktctx.StoreDir(cfgRoot, currentCtx)+pendingSuffix),
		kv("Action Log", aktctx.ActionLogPath(cfgRoot, currentCtx)+pendingSuffix),
		kv("Keyring", keyringLocation(cfgRoot, backend)),
		"",
		"No account is configured yet. Next:",
		"  akt context keys add <name>              create a new account",
		"  akt context keys add <name> --recover    import an existing mnemonic",
		"  akt context show                         review this context",
	}
}

// keyringLocation describes where the chosen backend keeps its keys. The os
// backend uses no directory at all, so naming one would send the user looking
// in the wrong place.
func keyringLocation(cfgRoot, backend string) string {
	if backend == "os" {
		return fmt.Sprintf("system keyring, service %q", aktctx.KeyringServiceName)
	}

	return aktctx.KeyringDir(cfgRoot, aktctx.Keyring{Name: defaultKeyringName}) + pendingSuffix
}

// chainIDOf returns the chain ID of the named network, or "" if it is absent.
func chainIDOf(networks []aktctx.Network, name string) string {
	for _, n := range networks {
		if n.Name == name {
			return n.ChainID
		}
	}

	return ""
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
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

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

		os.Stderr.WriteString(b.String())
	}

	// Total rendered lines: header(1) + blank(1) + select-all(1) + blank(1) + networks(n) + blank(1) + hint(1) = n+6
	renderLines := n + 6

	clearLines := func() {
		for i := 0; i < renderLines; i++ {
			os.Stderr.WriteString("\033[A\033[2K")
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
				clearLines()
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
				os.Stderr.WriteString("\r\n")
				_ = term.Restore(int(os.Stdin.Fd()), oldState)

				var result []aktctx.Network
				for i, net := range networks {
					if checked[i] {
						result = append(result, net)
					}
				}
				return result
			case 'j':
				clearLines()
				cursor = (cursor + 1) % totalItems
				render()
			case 'k':
				clearLines()
				cursor = (cursor - 1 + totalItems) % totalItems
				render()
			case 'q', 3:
				os.Stderr.WriteString("\r\n")
				_ = term.Restore(int(os.Stdin.Fd()), oldState)
				fmt.Fprintln(os.Stderr, "Aborted.")
				os.Exit(0)
			}
		} else if nr == 3 && buf[0] == 27 && buf[1] == 91 {
			clearLines()
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

// selectOption describes one row of a single-select menu.
type selectOption struct {
	value string
	label string
	desc  string

	// unavailable marks a row this host cannot provide. It is still listed,
	// so the absence is visible rather than silent, but the cursor skips it
	// and it can never be chosen.
	unavailable bool
}

// selectKeyringBackend presents a single-select menu for keyring backend.
// Returns the selected backend string (e.g. "os", "file", "test").
//
// "os" is offered only where a platform credential store actually answers.
// Offering it on a host without one is how a headless server ended up with
// keys in an encrypted file while the context went on reporting "os"
// (SPEC §1.5, §3.9.1).
func selectKeyringBackend() string {
	systemKeyring := aktkeyring.SystemKeyringAvailable()

	osDesc := "System keyring (recommended)"
	if !systemKeyring {
		osDesc = "System keyring — unavailable on this host"
	}

	return singleSelect("Select keyring backend", []selectOption{
		{value: "os", label: "os", desc: osDesc, unavailable: !systemKeyring},
		{value: "file", label: "file", desc: "File-based encrypted keyring"},
		{value: "test", label: "test", desc: "Unencrypted test keyring (development only)"},
	}, 0)
}

// selectActiveContext asks which of the newly created contexts becomes
// current-context (SPEC §2.0d).
//
// The wizard used to prefer mainnet silently, so the shortest path through
// setup produced a configuration whose next transaction spent real AKT. The
// cursor now starts on a test network; mainnet is one keystroke away but is
// never what pressing enter selects.
func selectActiveContext(networks []aktctx.Network) string {
	if len(networks) == 0 {
		return ""
	}

	preferred := pickInitialContext(networks)

	options := make([]selectOption, 0, len(networks))
	initial := 0

	for i, n := range networks {
		if n.Name == preferred {
			initial = i
		}

		options = append(options, selectOption{
			value: n.Name,
			label: n.Name,
			desc:  strings.TrimSpace(fmt.Sprintf("%-14s %s", n.ChainID, networkRisk(n.Name))),
		})
	}

	return singleSelect("Select the active context", options, initial)
}

// pickInitialContext returns the network that the active-context prompt
// starts on: the safest of the selected networks.
//
// Preference runs safest-first -- sandbox, then testnet, then any other
// non-mainnet network -- and falls back to the first selection only when
// every option is mainnet. Selection lives here, rather than inside the
// raw-mode prompt, because this is the part that decides whether a mistake
// costs money and so is the part that has to be testable.
func pickInitialContext(networks []aktctx.Network) string {
	if len(networks) == 0 {
		return ""
	}

	for _, want := range []string{"sandbox", "testnet", "test"} {
		for _, n := range networks {
			if isMainnetName(n.Name) {
				continue
			}

			if strings.Contains(strings.ToLower(n.Name), want) {
				return n.Name
			}
		}
	}

	// No recognizable test network was selected. Anything that is not
	// mainnet is still the safer place to start.
	for _, n := range networks {
		if !isMainnetName(n.Name) {
			return n.Name
		}
	}

	return networks[0].Name
}

// isMainnetName reports whether a network name identifies the live network.
func isMainnetName(name string) bool {
	return strings.Contains(strings.ToLower(name), "mainnet")
}

// networkRisk states, in plain language, what transacting on a network costs.
// That distinction is the entire reason the prompt exists, so it belongs on
// the row rather than in a footnote.
func networkRisk(name string) string {
	lower := strings.ToLower(name)

	switch {
	case isMainnetName(lower):
		return "live network - transactions spend real AKT"
	case strings.Contains(lower, "sandbox"),
		strings.Contains(lower, "testnet"),
		strings.Contains(lower, "test"):
		return "test network - tokens have no value"
	default:
		return ""
	}
}

// renderSingleSelect draws a single-select menu with cursor on row `cursor`.
// It is a pure function so the frame the user actually sees can be asserted
// without a terminal; singleSelect only decides when to draw it.
func renderSingleSelect(title string, options []selectOption, cursor int) string {
	g := glyphs.G()

	labelWidth := 10
	for _, opt := range options {
		if n := len(opt.label); n > labelWidth {
			labelWidth = n
		}
	}

	var b strings.Builder

	b.WriteString(ansiBold + title + ansiReset + ansiDim + "  ↑↓ move  enter confirm" + ansiReset + "\r\n")
	b.WriteString("\r\n")

	for i, opt := range options {
		icon := ansiDim + g.CheckboxOff + ansiReset
		nameStyle := ansiDim
		prefix := "  "
		rowStart := ""
		rowEnd := ""

		if opt.unavailable {
			// Listed so the absence is visible, never highlighted as a
			// choice: the cursor cannot rest here.
			b.WriteString(fmt.Sprintf("  %s%s %s  %s%s %s%s\r\n",
				prefix, ansiDim+g.CheckboxOff, ansiReset+ansiDim,
				fmt.Sprintf("%-*s", labelWidth, opt.label),
				ansiDim, opt.desc, ansiReset))

			continue
		}

		if i == cursor {
			icon = ansiGreen + g.CheckboxOn + ansiReset
			nameStyle = ansiReset
			prefix = ansiYellow + g.Cursor + " " + ansiReset
			rowStart = ansiBgSel
			rowEnd = ansiReset
		}

		b.WriteString(fmt.Sprintf("  %s%s %s  %s%s%s %s%s%s\r\n",
			rowStart, prefix, icon,
			nameStyle, fmt.Sprintf("%-*s", labelWidth, opt.label), ansiReset,
			ansiDim, opt.desc+ansiReset, rowEnd))
	}

	b.WriteString("\r\n")

	return b.String()
}

// singleSelect presents a single-select menu and returns the chosen value.
// initial positions the cursor.
//
// When stdin cannot be put in raw mode (a pipe, a CI runner) the initial
// option is returned rather than blocking on a read that will never be
// answered -- so every caller's documented default is also its fallback.
func singleSelect(title string, options []selectOption, initial int) string {
	if len(options) == 0 {
		return ""
	}

	if initial < 0 || initial >= len(options) {
		initial = 0
	}

	cursor := initial

	// Never start on a row this host cannot provide.
	for i := 0; i < len(options) && options[cursor].unavailable; i++ {
		cursor = (cursor + 1) % len(options)
	}

	if options[cursor].unavailable {
		// Every option is unavailable; there is nothing to choose.
		return ""
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// Non-TTY fallback (SPEC §3.9.3): the default option, which is the
		// first selectable one — never a row this host cannot provide.
		return options[cursor].value
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	render := func() {
		os.Stderr.WriteString(renderSingleSelect(title, options, cursor))
	}

	// header(1) + blank(1) + options(n) + blank(1)
	renderLines := len(options) + 3

	clearLines := func() {
		for i := 0; i < renderLines; i++ {
			os.Stderr.WriteString("\033[A\033[2K")
		}
	}

	render()

	buf := make([]byte, 3)
	totalItems := len(options)

	// step advances the cursor past any row this host cannot provide, so an
	// unavailable option can never be selected.
	step := func(delta int) {
		for i := 0; i < totalItems; i++ {
			cursor = (cursor + delta + totalItems) % totalItems
			if !options[cursor].unavailable {
				return
			}
		}
	}

	for {
		nr, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		if nr == 1 {
			switch buf[0] {
			case '\r', '\n':
				os.Stderr.WriteString("\r\n")
				_ = term.Restore(int(os.Stdin.Fd()), oldState)
				return options[cursor].value
			case 'j':
				clearLines()
				step(1)
				render()
			case 'k':
				clearLines()
				step(-1)
				render()
			case 'q', 3:
				os.Stderr.WriteString("\r\n")
				_ = term.Restore(int(os.Stdin.Fd()), oldState)
				fmt.Fprintln(os.Stderr, "Aborted.")
				os.Exit(0)
			}
		} else if nr == 3 && buf[0] == 27 && buf[1] == 91 {
			clearLines()
			switch buf[2] {
			case 65: // Up
				step(-1)
			case 66: // Down
				step(1)
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
		if ft.HighGasPrice > 0 && ft.Denom != "" {
			n.GasPrices = fmt.Sprintf("%g%s", ft.HighGasPrice, ft.Denom)
		}
	}

	n.GasAdjustment = "1.5"

	if len(meta.Faucets) > 0 && meta.Faucets[0].URL != "" {
		n.Faucet = meta.Faucets[0].URL
	}

	return n
}
