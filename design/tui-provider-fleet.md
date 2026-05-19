# Provider Fleet Dashboard — UX Design

## Overview

The Provider dashboard is the second hub tab in `akt monitor`, accessible via the `Tab` key or `akt monitor provider`. It provides real-time monitoring of all Akash network providers: a scan progress bar during initial discovery, a version distribution chart with dot visualization, a scrollable provider table filtered by selected version, and a drill-down detail view showing per-node resource allocation. Data is sourced from on-chain provider queries and per-provider health checks (gRPC preferred, REST fallback), with a smart cache that schedules checks based on provider state.

## Wireframe — Provider List View

```
                        akt monitor - Akash Network
 Network   [Provider]   Oracle/BME

████████████████████████████████░░░░░░░░ Scanning providers... 142/247 checked, 98 online

Provider Version Distribution
▸ 0.6.4        ●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●●  62 ( 63.3%)
  0.6.3        ○○○○○○○○○○○○○○○○○○  18 ( 18.4%)
  0.6.2        ○○○○○○○○○○  10 ( 10.2%)
  0.6.1-rc2   ○○○○○○○○   8 (  8.2%)

← / → or h/l: select version

Providers (98 total, 62 on 0.6.4)
  #   Provider                             Version        CPU          Memory       GPU                Loc
  1   provider1.akash.network              0.6.4          42/64        128/256Gi    4/8 H100           US
▸ 2   provider2.akash.network              0.6.4          18/32        64/128Gi     -                  DE
  3   provider3.example.com                0.6.4          120/256      512/1024Gi   8/16 A100          US
  ...

q: quit | Tab: dashboard | j/k: scroll | enter: detail | r: refresh
RPC: https://rpc.akash.network [WS]
```

## Wireframe — Provider Detail View

```
                        akt monitor - Akash Network
 Network   [Provider]   Oracle/BME

Provider Details

Name:    Equinix Dallas
URL:     provider1.akash.network
Version: 0.6.4
Location: US
Total:   CPU 42/64 | Memory 128/256 Gi

Nodes (4 total, 12/16 GPUs avail)
  Node                 CPU            Memory           GPU
  node-gpu-01          8/16           32/64 Gi         2/4 NVIDIA H100 (80Gi)
  node-gpu-02          8/16           32/64 Gi         2/4 NVIDIA H100 (80Gi)
  node-cpu-01          12/16          32/64 Gi         -
  node-cpu-02          14/16          32/64 Gi         -

q: quit | Tab: dashboard | j/k: scroll | esc: back | r: refresh
RPC: https://rpc.akash.network [WS]
```

## Component Specifications

### Scan Progress Bar

| Element | Specification |
|---|---|
| **Visibility** | Shown only when `pv.Loading && pv.Total > 0` |
| **Bar** | `ProgressBar(loaded/total, 40)` — 40 chars wide, `bubbles/progress` with Slate200 fill, Slate700 empty |
| **Label** | `Scanning providers... N/T checked, M online` in `mutedStyle` (Slate500) |
| **Position** | Top of provider dashboard, above version distribution |

### Version Distribution Chart

| Element | Specification |
|---|---|
| **Header** | "Provider Version Distribution" in `headerStyle` (SectionTitle: Slate100 bold) |
| **Version order** | Semver-sorted newest-first via `rpc.GetProviderVersions()` (handles `-rc` suffixes) |
| **Selected version line** | `▸` cursor (proposerStyle, YellowColor bold) + version name (12w) + filled dots `●` (GreenColor) + count + percentage |
| **Unselected version line** | `  ` indent + version name (12w) + open dots `○` (Slate500) + count + percentage |
| **Dot count** | `min(count, 50)` — capped at 50 dots per line |
| **Dot characters** | `glyphs.G().DotFilled` (●) for selected, `glyphs.G().DotOpen` (○) for unselected |
| **Version marker** | Selected: `● ` (GreenColor). Unselected: `○ ` (Slate500). Prepended before index column. |
| **Help text** | `← / → or h/l: select version` in `mutedStyle` |
| **Filtering** | Localhost providers (`127.0.0.1`, `localhost`) excluded from counts |

### Provider Table

| Element | Specification |
|---|---|
| **Component** | `bubbles/table.Model` with custom styles |
| **Columns** | # (4w), Provider (36w), Version (14w), CPU (12w), Memory (12w), GPU (18w), Loc (4w) |
| **Header** | "Providers (N total, M on vX.Y.Z)" in `headerStyle` |
| **Provider URL** | `https://` and `http://` stripped, port stripped, truncated with `...` at 34 chars |
| **Version display** | Selected version highlighted in GreenColor; others in mutedStyle |
| **CPU format** | `available/total` in cores (millicores / 1000) via `formatResourceRatio()` |
| **Memory format** | `available/total` in human units (Mi/Gi/Ti) via `formatMemoryRatio()` |
| **GPU format** | `available/total model` — model name from first GPU, truncated to fit column. `-` if no GPUs. |
| **Location** | 2-letter country code from provider attributes, `--` if unknown |
| **Row selection** | `▸` cursor in proposerStyle (YellowColor bold). Selected row: `highlightStyle` (Slate200 bold). Normal: moniker/muted styles. |
| **Sorting** | Selected version first, then by version (newest first via semver compare), then by URL alphabetically |
| **Visible rows** | `max(height - providerListOverhead(25), minVisibleProviders(5))` |
| **Filtering** | Localhost providers excluded. Only online providers with known version shown. |

### Provider Detail View

| Element | Specification |
|---|---|
| **Header** | "Provider Details" in `detailHeaderStyle` (SectionTitle) |
| **Info panel** | KV pairs: Name, URL (truncated at 50), Version (GreenColor), Location, Total (CPU ratio + Memory ratio) |
| **Label style** | `detailLabelStyle` (KVLabel: Slate500) |
| **Value style** | `detailValueStyle` (KVValue: Slate200) |
| **Loading state** | "Fetching node details via gRPC..." in mutedStyle |
| **Error state** | Error message in errorStyle (AccentRed bold) |
| **Node table header** | "Nodes (N total)" or "Nodes (N total, A/T GPUs avail)" if GPUs present |
| **Node table** | `bubbles/table.Model` with columns: Node (20w), CPU (14w), Memory (16w), GPU (30w) |
| **Node name** | Truncated with `...` at 20 chars. Falls back to `node-N` if empty. |
| **Node GPU format** | `available/allocatable vendor model (memorySize)` — vendor normalized (nvidia→NVIDIA, amd→AMD). Truncated at 28 chars. `-` if no GPUs. |
| **Visible node rows** | `max(height - nodeListOverhead(14), minVisibleNodes(3))` |

### Table Styles (shared across provider/node/validator/block tables)

| Style | Specification |
|---|---|
| **Header** | `mutedStyle` (Slate500) |
| **Cell** | Transparent (`lipgloss.NewStyle()`) — allows pre-styled strings to pass through |
| **Selected** | `highlightStyle` (Slate200 bold) |

## Color Tokens Used

| Token | Usage |
|---|---|
| `theme.AccentRed` | Hub tab active background, error text |
| `theme.Slate950` | Hub tab active foreground |
| `theme.Slate500` | Muted text, labels, column headers, unselected version dots, grid not-voted |
| `theme.Slate400` | Inactive hub tab text |
| `theme.Slate300` | Body text, moniker style, detail values |
| `theme.Slate200` | Highlight style, selected rows, progress bar fill, detail values |
| `theme.Slate100` | Section titles, headings |
| `theme.Slate700` | Progress bar empty segments |
| `theme.GreenColor` | Selected version dots, version display highlight |
| `theme.YellowColor` | Cursor `▸`, proposer style |

## Interaction

| Key | Context | Action |
|---|---|---|
| `Tab` | Any | Cycle hub: Network → Provider → Oracle/BME |
| `Shift+Tab` | Any | Cycle hub backward |
| `h` / `←` | Provider list | Select previous version (wraps around) |
| `l` / `→` | Provider list | Select next version (wraps around) |
| `j` / `↓` | Provider list | Move provider table cursor down |
| `k` / `↑` | Provider list | Move provider table cursor up |
| `g` / `Home` | Provider list | Jump to first provider |
| `G` / `End` | Provider list | Jump to last provider |
| `Enter` | Provider list | Open provider detail view (fetch nodes via gRPC) |
| `Esc` / `Backspace` | Provider detail | Return to provider list |
| `j` / `↓` | Provider detail | Scroll node table down |
| `k` / `↑` | Provider detail | Scroll node table up |
| `1` / `2` / `3` / `Tab` | Provider detail | Exit detail and switch tabs |
| `r` | Provider list | Trigger re-scan (chain sync) |
| `q` | Any | Quit (or BackMsg if embedded) |

## Data Sources

| Data | Source | Method |
|---|---|---|
| On-chain provider list | ABCI query `akash.provider.v1beta4.Query/Providers` | `rpc.RPCProviderClient.GetProvidersOnChain()` or seed endpoint |
| Active lease providers | REST `/akash/market/v1beta5/leases/list?filters.state=active` | `rpc.RPCProviderClient.GetActiveLeaseProviders()` |
| Provider health check | gRPC port 8444 (preferred) or REST `/status` + `/version` (fallback) | `rpc.QueryProviderStatusGRPC()` / `rpc.QueryProviderStatus()` + `rpc.QueryProviderVersion()` |
| Provider node details | gRPC port 8444 | `rpc.QueryProviderStatusGRPC()` — returns `[]ProviderNodeWithGPU` |

### Smart Cache Scheduling

The provider cache (`internal/monitor/cache/cache.go`) uses adaptive check intervals based on provider state:

| Provider State | Check Interval | Constant |
|---|---|---|
| Online | 1 minute | `OnlineCheckInterval` |
| Recently offline (< 5 consecutive failures) | 5 minutes | `RecentOfflineInterval` |
| Long-term offline (>= 5 consecutive failures) | 6 hours | `LongTermOfflineInterval` |

**Priority queue** (for initial scan ordering):
1. Unchecked (never checked, priority 0)
2. Online (priority 1)
3. Recently offline (priority 2)
4. Long-term offline (priority 3)

Within same priority, sorted by `LastChecked` ascending (oldest first).

**Concurrency**: Maximum 10 concurrent provider checks (`MaxConcurrentChecks`). Dispatched via `dispatchProviderChecks()` which fills available slots from the queue.

**Persistence**: bbolt database at `~/.config/akt/cache/providers.json`. Data persisted on each write (bbolt is transactional). Cache save tick every 30s (`CacheSaveInterval`).

**Chain re-sync**: Full provider list refresh every 10 minutes (`ChainSyncInterval`). On first run with empty cache, attempts seed endpoint before falling back to on-chain query.

**Deduplication**: Providers deduplicated by `HostURI`, keeping the most recently seen entry.

## Implementation Reference

| Component | File |
|---|---|
| Provider list rendering (`renderProvidersTab`) | `internal/monitor/ui/view.go` (lines 963-991) |
| Version distribution (`renderVersionDistribution`) | `internal/monitor/ui/view.go` (lines 993-1013) |
| Provider detail rendering (`renderProviderDetailView`) | `internal/monitor/ui/view.go` (lines 1177-1244) |
| Provider row formatting | `internal/monitor/ui/view.go` (lines 1065-1116) |
| Node GPU formatting | `internal/monitor/ui/view.go` (lines 1247-1305) |
| Model state & Update loop | `internal/monitor/ui/model.go` |
| Provider cache (bbolt) | `internal/monitor/cache/cache.go` |
| RPC provider client | `internal/monitor/rpc/` |
| Shared theme | `internal/ui/theme/theme.go` |
| Glyph definitions (●, ○, ▸) | `internal/glyphs/` |

## SPEC.md Cross-Reference

| Section | Coverage |
|---|---|
| **§8.3.10** Provider Fleet Monitor View | Scan progress bar with checked/total/online counts. Version distribution: semver-sorted newest-first, dot visualization (●/○), h/l to select version filter. Provider table: URL, version, CPU, memory, GPU, location. Provider detail: info panel + node-level table (CPU, memory, GPU model+count). Smart cache scheduling: online 1m, recently-offline 5m, long-term-offline 6h. Max 10 concurrent checks, chain re-sync every 10m. |
