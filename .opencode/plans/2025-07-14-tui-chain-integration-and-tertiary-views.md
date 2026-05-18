# TUI Chain Integration & Tertiary Views (Part 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire chain query client into the TUI for live data in Providers/Governance/Staking views, implement detail views for those resources, add deploy workflow view, shell view, live sync integration, and toast wiring.

**Architecture:** The root command already creates a fully resolved `sdkclient.Context` during `PersistentPreRunE` but only passes the RPC endpoint string to the TUI. We expand `tui.Config` to accept a `v1beta3.LightClient` (from `pkg.akt.dev/go/node/client/v1beta3`), pass it from the root command, and use it to query chain state in TUI views via async `tea.Cmd` functions. Provider/Governance/Staking views are upgraded from placeholders to live data. Remaining detail views, deploy workflow UI, shell view, and live sync integration are built using existing components from Parts 1 and 2.

**Tech Stack:** Go, Bubbletea v2, Lipgloss v2, chain-sdk v1beta3 LightClient, provider REST client

**Design Reference:** `design/prototype/tui-views.jsx` (provider/governance/staking detail), `design/prototype/tui-workflow.jsx` (deploy workflow), `design/prototype/tui-monitor.jsx` (shell view), SPEC.md §2.3 (workflow engine), §8.3 (views), §6.6 (sync-to-store integration)

---

## File Structure

### Modified Files
| File | Changes |
|------|---------|
| `internal/tui/app.go` | Add LightClient to Config/App, wire chain queries, integrate new views |
| `internal/tui/messages/messages.go` | Add chain query message types (ProvidersLoadedMsg, ProposalsLoadedMsg, ValidatorsLoadedMsg, BalancesLoadedMsg) |
| `internal/cli/root.go` | Pass LightClient, Store, and ResolvedCtx to tui.Config when launching TUI |
| `internal/tui/views/providers.go` | Upgrade from placeholder to live data binding |
| `internal/tui/views/governance.go` | Upgrade from placeholder to live data binding |
| `internal/tui/views/staking.go` | Upgrade from placeholder to live data binding |

### New Files
| File | Responsibility |
|------|---------------|
| `internal/tui/views/provider_detail.go` | Provider detail view (capacity, attributes, audit history) |
| `internal/tui/views/proposal_detail.go` | Governance proposal detail (tally bar, timeline, description) |
| `internal/tui/views/validator_detail.go` | Validator detail (power, performance, delegation) |
| `internal/tui/views/workflow.go` | Deploy workflow view (step strip, log stream, bid panel) |
| `internal/tui/views/shell.go` | Interactive terminal in deployment via provider lease-shell |

---

## Task 1: Wire LightClient into TUI via Root Command

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/messages/messages.go`
- Modify: `internal/cli/root.go`

This is the foundational plumbing task. The root command (`internal/cli/root.go`) already creates a `sdkclient.Context` during `PersistentPreRunE`. We need to:
1. Create a `v1beta3.LightClient` from that context
2. Pass it through `tui.Config`
3. Add chain query message types
4. Add chain query loading commands in app.go

- [ ] **Step 1: Add chain query message types to messages.go**

Add to `internal/tui/messages/messages.go`:

```go
import (
    banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
    govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
    staketypes "github.com/cosmos/cosmos-sdk/x/staking/types"
    ptypes "pkg.akt.dev/go/node/types/provider/v1beta4"
)

// BalancesLoadedMsg carries account balances from chain.
type BalancesLoadedMsg struct {
    Balances []banktypes.Balance
    Err      error
}

// ProvidersChainLoadedMsg carries on-chain provider list.
type ProvidersChainLoadedMsg struct {
    Providers []ptypes.Provider
    Err       error
}

// ProposalsLoadedMsg carries governance proposals from chain.
type ProposalsLoadedMsg struct {
    Proposals []*govv1.Proposal
    Err       error
}

// ValidatorsLoadedMsg carries validator set from chain.
type ValidatorsLoadedMsg struct {
    Validators []staketypes.Validator
    Err        error
}
```

- [ ] **Step 2: Expand tui.Config with LightClient**

In `internal/tui/app.go`, add to Config:

```go
import (
    v1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type Config struct {
    // ... existing fields ...
    
    // Chain client (optional — nil = no chain queries, views show placeholders)
    ChainClient v1beta3.LightClient
}
```

Add to App struct:
```go
type App struct {
    // ... existing fields ...
    chainClient v1beta3.LightClient
}
```

In `newApp()`, store `cfg.ChainClient`.

- [ ] **Step 3: Add chain query loading commands**

In `internal/tui/app.go`, add async loaders:

```go
func loadProvidersFromChain(cl v1beta3.LightClient) tea.Cmd {
    return func() tea.Msg {
        if cl == nil {
            return messages.ProvidersChainLoadedMsg{Err: fmt.Errorf("no chain client")}
        }
        resp, err := cl.Query().Provider().Providers(context.Background(),
            &ptypes.QueryProvidersRequest{})
        if err != nil {
            return messages.ProvidersChainLoadedMsg{Err: err}
        }
        return messages.ProvidersChainLoadedMsg{Providers: resp.Providers}
    }
}

func loadProposals(cl v1beta3.LightClient) tea.Cmd {
    return func() tea.Msg {
        if cl == nil {
            return messages.ProposalsLoadedMsg{Err: fmt.Errorf("no chain client")}
        }
        resp, err := cl.Query().Gov().Proposals(context.Background(),
            &govv1.QueryProposalsRequest{})
        if err != nil {
            return messages.ProposalsLoadedMsg{Err: err}
        }
        return messages.ProposalsLoadedMsg{Proposals: resp.Proposals}
    }
}

func loadValidators(cl v1beta3.LightClient) tea.Cmd {
    return func() tea.Msg {
        if cl == nil {
            return messages.ValidatorsLoadedMsg{Err: fmt.Errorf("no chain client")}
        }
        resp, err := cl.Query().Staking().Validators(context.Background(),
            &staketypes.QueryValidatorsRequest{Status: "BOND_STATUS_BONDED"})
        if err != nil {
            return messages.ValidatorsLoadedMsg{Err: err}
        }
        return messages.ValidatorsLoadedMsg{Validators: resp.Validators}
    }
}

func loadBalances(cl v1beta3.LightClient, addr string) tea.Cmd {
    return func() tea.Msg {
        if cl == nil || addr == "" {
            return messages.BalancesLoadedMsg{Err: fmt.Errorf("no chain client or address")}
        }
        resp, err := cl.Query().Bank().AllBalances(context.Background(),
            &banktypes.QueryAllBalancesRequest{Address: addr})
        if err != nil {
            return messages.BalancesLoadedMsg{Err: err}
        }
        // Convert sdk.Coins to []banktypes.Balance
        var balances []banktypes.Balance
        for _, c := range resp.Balances {
            balances = append(balances, banktypes.Balance{Address: addr, Coins: sdk.NewCoins(c)})
        }
        return messages.BalancesLoadedMsg{Balances: balances}
    }
}
```

- [ ] **Step 4: Handle chain query messages in Update()**

Add message handlers that pass data to views:

```go
case messages.ProvidersChainLoadedMsg:
    if msg.Err == nil {
        a.providers.SetChainData(msg.Providers)
    }
case messages.ProposalsLoadedMsg:
    if msg.Err == nil {
        a.governance.SetProposals(msg.Proposals)
    }
case messages.ValidatorsLoadedMsg:
    if msg.Err == nil {
        a.staking.SetValidators(msg.Validators)
    }
case messages.BalancesLoadedMsg:
    if msg.Err == nil {
        a.dashboard.SetBalances(msg.Balances)
    }
```

- [ ] **Step 5: Trigger chain queries on view switch**

When switching to providers view → fire `loadProvidersFromChain`. When switching to governance → fire `loadProposals`. When switching to staking → fire `loadValidators`. Also fire `loadBalances` on init for dashboard.

- [ ] **Step 6: Update root.go to pass data to TUI**

In `internal/cli/root.go`, find the `RunE` that launches the TUI (the no-subcommand case). Update it to:
1. Get the resolved `*aktctx.Context` from the manager
2. Open the bbolt store for the current context
3. Create a `v1beta3.LightClient` from the `sdkclient.Context`
4. Pass all three through `tui.Config`

```go
RunE: func(cmd *cobra.Command, args []string) error {
    cctx := sdkclient.GetClientContextFromCmd(cmd)
    cfgRoot, _ := aktctx.ConfigHome(v.GetString("home"))
    
    // Get resolved context
    var resolvedCtx *aktctx.Context
    ctxName := v.GetString("context")
    if mgr != nil {
        resolvedCtx, _ = mgr.Resolve(ctxName)
    }
    
    // Open store for current context
    var dataStore store.Store
    if resolvedCtx != nil {
        storeDir := aktctx.StoreDir(cfgRoot, resolvedCtx.Name)
        dbPath := filepath.Join(storeDir, "deployments.db")
        if bs, err := bbolt.Open(dbPath); err == nil {
            dataStore = bs
            defer bs.Close()
        }
    }
    
    // Create chain query client
    var chainClient v1beta3.LightClient
    if cctx.NodeURI != "" {
        chainClient, _ = v1beta3client.NewLightClient(cctx)
    }
    
    return akttui.Run(akttui.Config{
        Viper:        v,
        RPCEndpoint:  cctx.NodeURI,
        RESTEndpoint: "",
        CacheDir:     filepath.Join(cfgRoot, "cache"),
        Insecure:     true,
        Store:        dataStore,
        ResolvedCtx:  resolvedCtx,
        ChainClient:  chainClient,
    })
}
```

- [ ] **Step 7: Verify build**

Run: `make akt`

- [ ] **Step 8: Commit**

```bash
git add internal/tui/ internal/cli/root.go
git commit -m "feat(tui): wire chain query client into TUI for live data

Expand Config with v1beta3.LightClient for chain queries.
Add message types for providers, proposals, validators, balances.
Add async loading commands for all chain query types.
Root command now passes LightClient, Store, and ResolvedCtx to TUI."
```

---

## Task 2: Upgrade Providers View with Chain Data

**Files:**
- Modify: `internal/tui/views/providers.go`
- Create: `internal/tui/views/provider_detail.go`
- Modify: `internal/tui/app.go`

Upgrade the placeholder providers view to show live on-chain provider data.

- [ ] **Step 1: Add SetChainData to ProvidersView**

In `providers.go`, add a method that accepts `[]ptypes.Provider` (from chain query) and converts them to `TableRow` format:

```go
func (v *ProvidersView) SetChainData(providers []ptypes.Provider) {
    // Convert each provider to a TableRow
    // Extract: host URI, region (from attributes), GPU (from attributes), 
    // CPU/memory (from attributes), active leases count, audit status, version
}
func (v *ProvidersView) SelectedProvider() *ptypes.Provider
```

- [ ] **Step 2: Create provider_detail.go**

Following the JSX prototype `ViewProviderDetail`:
- Header: host URL, audit badge, version badge, region
- Capacity panel (KV): CPU total, memory, GPU, active leases + utilization progress bars
- Attributes panel (KV): all provider attributes as key-value pairs
- Audit history panel: list of audit entries (date, height, auditor, result)

```go
type ProviderDetailView struct {
    provider   *ptypes.Provider
    width      int
    height     int
    scroll     int
}

func NewProviderDetailView() ProviderDetailView
func (v *ProviderDetailView) SetProvider(p *ptypes.Provider)
func (v *ProviderDetailView) SetSize(w, h int)
func (v *ProviderDetailView) ScrollUp()
func (v *ProviderDetailView) ScrollDown()
func (v ProviderDetailView) HasData() bool
func (v ProviderDetailView) View() string
```

- [ ] **Step 3: Add viewProviderDetail to app.go**

Wire Enter on providers list → provider detail. Add `providerDetail ProviderDetailView` to App. Handle Esc to go back.

- [ ] **Step 4: Verify and commit**

```bash
git add internal/tui/
git commit -m "feat(tui): upgrade providers view with chain data and add detail view"
```

---

## Task 3: Upgrade Governance View with Chain Data

**Files:**
- Modify: `internal/tui/views/governance.go`
- Create: `internal/tui/views/proposal_detail.go`
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Add SetProposals to GovernanceView**

In `governance.go`, add a method that accepts `[]*govv1.Proposal` and converts to `TableRow`:

```go
func (v *GovernanceView) SetProposals(proposals []*govv1.Proposal) {
    // Convert each proposal to TableRow with: ID, Title, Status (StateTag), 
    // Yes%, No%, Abstain%, Veto%, EndTime
}
func (v *GovernanceView) SelectedProposal() *govv1.Proposal
```

- [ ] **Step 2: Create proposal_detail.go**

Following JSX `ViewProposalDetail`:
- Header: proposal ID, title, status badge
- Tally panel: horizontal color bar (green/red/gray/yellow segments) + per-option progress bars
- Timeline panel: 5 steps (submitted, deposit, voting, ends, executed) with ●/○ indicators
- Description panel: proposal summary/description text

```go
type ProposalDetailView struct {
    proposal *govv1.Proposal
    width    int
    height   int
    scroll   int
}

func NewProposalDetailView() ProposalDetailView
func (v *ProposalDetailView) SetProposal(p *govv1.Proposal)
func (v *ProposalDetailView) SetSize(w, h int)
func (v *ProposalDetailView) ScrollUp()
func (v *ProposalDetailView) ScrollDown()
func (v ProposalDetailView) HasData() bool
func (v ProposalDetailView) View() string
```

- [ ] **Step 3: Wire into app.go**

Enter on governance list → proposal detail. Add `proposalDetail ProposalDetailView` to App. `v` key → open vote confirm dialog.

- [ ] **Step 4: Verify and commit**

```bash
git add internal/tui/
git commit -m "feat(tui): upgrade governance view with chain data and add proposal detail"
```

---

## Task 4: Upgrade Staking View with Chain Data

**Files:**
- Modify: `internal/tui/views/staking.go`
- Create: `internal/tui/views/validator_detail.go`
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Add SetValidators to StakingView**

In `staking.go`, add:

```go
func (v *StakingView) SetValidators(validators []staketypes.Validator) {
    // Sort by tokens descending, convert to TableRow with:
    // rank, moniker, power (formatted with K/M/B), VP%, commission%, uptime, signed blocks
}
func (v *StakingView) SelectedValidator() *staketypes.Validator
```

- [ ] **Step 2: Create validator_detail.go**

Following JSX `ViewValidatorDetail`:
- Header: moniker, rank badge, VP% badge
- Power panel (KV): voting power, rank, commission, max change, self bond
- Performance panel (KV): uptime, signed, missed, jailed status, slashes
- Your delegation panel: delegated amount, rewards, APR, unbonding (requires chain query for user-specific data — show "connect wallet" placeholder if no default account)

```go
type ValidatorDetailView struct {
    validator *staketypes.Validator
    width     int
    height    int
    scroll    int
}

func NewValidatorDetailView() ValidatorDetailView
func (v *ValidatorDetailView) SetValidator(val *staketypes.Validator)
func (v *ValidatorDetailView) SetSize(w, h int)
func (v *ValidatorDetailView) ScrollUp()
func (v *ValidatorDetailView) ScrollDown()
func (v ValidatorDetailView) HasData() bool
func (v ValidatorDetailView) View() string
```

- [ ] **Step 3: Wire into app.go**

Enter on staking list → validator detail. `d` → delegate confirm dialog. `u` → undelegate confirm dialog. `r` → redelegate confirm dialog.

- [ ] **Step 4: Verify and commit**

```bash
git add internal/tui/
git commit -m "feat(tui): upgrade staking view with chain data and add validator detail"
```

---

## Task 5: Deploy Workflow View

**Files:**
- Create: `internal/tui/views/workflow.go`
- Modify: `internal/tui/app.go`

This implements the animated deploy workflow UI from the JSX prototype (`tui-workflow.jsx`). It's a **display-only** view that visualizes workflow step progression. The actual workflow execution engine (`internal/workflow/`) is not wired in this task — this just creates the UI that can be driven by step-complete messages.

- [ ] **Step 1: Create workflow.go**

```go
type WorkflowStep struct {
    Name        string
    Description string
    Status      string // "pending", "running", "done", "error"
    Output      string // step result text
}

type WorkflowView struct {
    name     string // "deploy", "update", "close"
    steps    []WorkflowStep
    current  int
    logLines []string
    bids     []WorkflowBid // for deploy workflow bid panel
    width    int
    height   int
    paused   bool
    done     bool
}

type WorkflowBid struct {
    Provider string
    Price    string
    Audit    string
    Selected bool
}

func NewWorkflowView(name string, steps []WorkflowStep) WorkflowView
func (v *WorkflowView) SetSize(w, h int)
func (v *WorkflowView) AdvanceStep(output string)
func (v *WorkflowView) FailStep(err string)
func (v *WorkflowView) AppendLog(line string)
func (v *WorkflowView) SetBids(bids []WorkflowBid)
func (v *WorkflowView) SelectBid(idx int)
func (v *WorkflowView) TogglePause()
func (v WorkflowView) View() string
```

The `View()` renders (from JSX `ViewWorkflow`):
1. **Header**: `akt deploy` badge + SDL filename + deployment name + status indicator
2. **Step strip**: Horizontal numbered circles — ✓ (done, green), spinner (running, red), number (pending, dim). Connected by lines (green if done, dim if pending).
3. **Split panel**: Left = log stream (scrollable), Right = bid panel (for deploy workflow)
4. **Footer**: When done, show endpoint summary. When running, show pause/cancel hints.

The 8 default deploy steps (from JSX):
```go
var deploySteps = []WorkflowStep{
    {Name: "Parse SDL", Description: "validate manifest"},
    {Name: "Create", Description: "MsgCreateDeployment"},
    {Name: "Bids", Description: "60-block auction"},
    {Name: "Select", Description: "rank by price + audit"},
    {Name: "Lease", Description: "MsgCreateLease"},
    {Name: "Manifest", Description: "mTLS upload"},
    {Name: "Active", Description: "pull image · run"},
    {Name: "Endpoints", Description: "forwarded URIs"},
}
```

- [ ] **Step 2: Wire into app.go (optional activation)**

Add `workflowView WorkflowView` to App. The workflow view can be activated from the command palette ("Deploy" command) or from a future `D` keybinding. For now, just add the view constant and rendering — actual workflow execution integration is deferred.

- [ ] **Step 3: Verify and commit**

```bash
git add internal/tui/views/workflow.go internal/tui/app.go
git commit -m "feat(tui): implement deploy workflow progress view

8-step animated progress: step strip with ✓/spinner/number circles,
split panel with log stream and bid panel. Display-only — workflow
engine integration deferred to future task."
```

---

## Task 6: Shell View

**Files:**
- Create: `internal/tui/views/shell.go`
- Modify: `internal/tui/app.go`

Interactive terminal in a deployment container. This creates the UI structure; actual provider gateway `LeaseShell` connection is deferred.

- [ ] **Step 1: Create shell.go**

Following JSX `ViewShell`:

```go
type ShellView struct {
    title    string
    dseq     string
    service  string
    history  []ShellLine
    input    string
    width    int
    height   int
    active   bool
    scroll   int
}

type ShellLine struct {
    Kind string // "cmd" or "out"
    Text string
}

func NewShellView() ShellView
func (v *ShellView) Open(title, dseq, service string)
func (v *ShellView) Close()
func (v ShellView) Active() bool
func (v *ShellView) AppendOutput(text string)
func (v *ShellView) SetSize(w, h int)
func (v *ShellView) HandleKey(msg tea.KeyPressMsg) tea.Cmd
func (v ShellView) View() string
```

The `View()` renders:
1. Header: `shell` badge + deployment name + service + connection status
2. Terminal area: scrollable history with PS1 prompt (`akash@dseq:~$`) + command history + output
3. Input line: current input with cursor
4. Footer: Enter=run, Esc=back

`HandleKey` processes character input, Backspace, Enter (submit command as ShellLine with Kind="cmd").

- [ ] **Step 2: Wire into app.go**

`s` key on deployments → open shell view. Shell intercepts keys when active.

- [ ] **Step 3: Verify and commit**

```bash
git add internal/tui/views/shell.go internal/tui/app.go
git commit -m "feat(tui): implement shell view for interactive terminal in deployments

Terminal-style view with PS1 prompt, command history, scrollable output.
UI structure ready for provider gateway LeaseShell integration."
```

---

## Task 7: Live Sync Integration

**Files:**
- Modify: `internal/tui/app.go`

Wire store change notifications to trigger TUI re-renders. When the sync engine updates the store (via WebSocket events), the TUI should refresh the current view's data.

- [ ] **Step 1: Add periodic refresh ticker**

Use `tea.Tick` to periodically check for store updates. A simple approach: every 5 seconds, if the sync state's `LastBlockHeight` has changed, refresh the current view's data.

```go
type syncCheckMsg struct{}

func syncCheckTick() tea.Cmd {
    return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
        return syncCheckMsg{}
    })
}
```

In `Init()`, start the tick. In `Update()`, on `syncCheckMsg`:
1. Fire `loadSyncState`
2. Compare new sync state with cached — if block height changed, fire appropriate load commands for the current view

- [ ] **Step 2: Fire the tick chain**

In the `SyncStateMsg` handler, compare heights and trigger refreshes:

```go
case messages.SyncStateMsg:
    if msg.Err == nil {
        oldHeight := int64(0)
        if a.syncState != nil {
            oldHeight = a.syncState.LastBlockHeight
        }
        a.syncState = msg.State
        a.dashboard.SetSyncState(msg.State)
        
        // If block height advanced, refresh current view
        if msg.State.LastBlockHeight > oldHeight {
            return a, a.refreshCurrentView()
        }
    }
```

Where `refreshCurrentView()` returns the appropriate load command based on `a.view`.

- [ ] **Step 3: Verify and commit**

```bash
git add internal/tui/app.go
git commit -m "feat(tui): add live sync integration with periodic refresh

Poll sync state every 5 seconds. When block height advances,
refresh the current view's data from store. Updates propagate
to dashboard, deployments, and leases views automatically."
```

---

## Task 8: Toast Wiring

**Files:**
- Modify: `internal/tui/app.go`

Wire toast notifications to action completions. Show toasts when:
- Deployment closed successfully
- Vote submitted
- Delegation successful
- Data refresh completed

- [ ] **Step 1: Add toast lifecycle to App**

```go
// In Update(), when ConfirmMsg is received:
case components.ConfirmMsg:
    a.confirmDialog.Close()
    // Show toast
    toast := components.NewToast("Deployment close initiated", components.ToastOK)
    a.toast = &toast
    return a, tea.Tick(components.ToastDuration, func(t time.Time) tea.Msg {
        return components.ToastExpiredMsg{}
    })

case components.ToastExpiredMsg:
    a.toast = nil
```

- [ ] **Step 2: Render toast in View()**

If `a.toast != nil`, render it at the bottom-right of the content area.

- [ ] **Step 3: Verify and commit**

```bash
git add internal/tui/app.go
git commit -m "feat(tui): wire toast notifications to action completions"
```

---

## Task 9: Final Integration Test

- [ ] **Step 1: Run full test suite**

Run: `go test ./internal/... 2>&1 | tail -20`
Expected: All PASS

- [ ] **Step 2: Build**

Run: `make akt`

- [ ] **Step 3: Update golden files**

Run: `go test ./internal/tui/ -update && go test ./internal/monitor/ui/ -update`

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore(tui): part 3 integration test fixes and polish"
```

---

## Summary

| Task | What | Dependencies |
|------|------|-------------|
| 1 | Chain client plumbing (LightClient → TUI, root.go update) | None |
| 2 | Providers view upgrade + provider detail | Task 1 |
| 3 | Governance view upgrade + proposal detail | Task 1 |
| 4 | Staking view upgrade + validator detail | Task 1 |
| 5 | Deploy workflow view (display-only) | None (independent) |
| 6 | Shell view | None (independent) |
| 7 | Live sync integration | Task 1 |
| 8 | Toast wiring | None |
| 9 | Final integration test | All above |

**Parallelizable:** Tasks 2-4 after Task 1. Tasks 5-6 and 8 can run in parallel with everything. Task 9 runs last.

**Deferred to future work:**
- Actual workflow engine execution (needs ChainClient + ProviderClient wired into workflow engine)
- Actual provider LeaseShell connection (needs provider gateway client in TUI)
- Vote/Delegate transaction broadcast (needs TxClient, which requires full Client not just LightClient)
- E2E tests for TUI (TASKS.md T105)
