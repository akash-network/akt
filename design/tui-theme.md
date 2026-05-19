# Theme System — UX Design

## Overview

The akt theme system provides a unified color palette and style library shared by both the CLI pretty-print output and the TUI views. It is implemented as a single Go package (`internal/ui/theme`) that exports lipgloss color variables and pre-built styles. The palette is based on the Shadcn Zinc/neutral scale with a single red accent color. State colors (green, yellow) are used exclusively for status indicators. The theme is currently dark-only; a light theme is planned but not yet implemented.

## Wireframe

The theme does not have its own visual layout. Instead, it provides the visual vocabulary used by every other component. The following diagram shows where each color tier appears in the shell:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Slate900 bg ─────────────────────────────────────────────────────────────── │
│ AccentRed "akt"  Slate200 "mainnet"  Slate500 ":"  Slate200 "18234567"     │
│                                                     GreenColor "● synced"  │
├──────────────────────────────────────────────────────────────────────────────┤
│ AccentRed bg + Slate950 fg [Active Tab]   Slate400 [Inactive Tab]          │
│ Slate700 ──────────────────────────────────────────────────────── (HRule)   │
│ Slate200 bold "Deployments"   Slate600 "/"   Slate200 bold "Detail"        │
│                                                                             │
│  Slate500 "DSEQ    STATE    PROVIDER    PRICE"          <- TableHeader     │
│  Slate300 "12345   active   prov1.net   5.2 uakt"       <- TableRow        │
│  Slate800 bg + Slate200 fg "12346   active   ..."       <- TableRowSelected│
│  AccentRed ">"                                           <- TableCursor     │
│                                                                             │
│  GreenColor "active"   YellowColor "paused"   Slate500 "closed"  <- Tags  │
│                                                                             │
│ Slate700 ──────────────────────────────────────────────────────── (HRule)   │
│ Slate400 bold "j/k"  Slate600 "move"   AccentRed bold "D"  Slate600 "new" │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Component Specifications

### Zinc/Neutral Scale

The palette uses the Shadcn Zinc scale mapped to true-color hex values. Brighter values indicate more visual emphasis.

| Variable | Hex | Role |
|----------|-----|------|
| `Slate950` | `#09090b` | App background, active tab text |
| `Slate900` | `#18181b` | Card/panel/overlay background, header bar bg |
| `Slate800` | `#27272a` | Elevated surface, selected row bg, secondary button bg |
| `Slate700` | `#3f3f46` | Borders, horizontal rules, panel borders, progress bar empty |
| `Slate600` | `#52525b` | Muted borders, subtle dividers, footer descriptions, breadcrumb separator |
| `Slate500` | `#71717a` | Column headers, muted labels, KV labels, metadata text |
| `Slate400` | `#a1a1aa` | Secondary text, inactive nav tabs, footer keys |
| `Slate300` | `#d4d4d8` | Body text, table row values, secondary button text |
| `Slate200` | `#e4e4e7` | Primary text, bold values, headings, context names, block height |
| `Slate100` | `#f4f4f5` | Headings, section titles |
| `Slate50`  | `#fafafa` | Maximum emphasis (reserved) |

### Accent Colors

| Variable | Hex | Usage |
|----------|-----|-------|
| `AccentRed` | `#ff4136` | Primary accent: app name, active tab bg, cursor, spinner, primary button bg, section rules, deploy button |
| `RedDim` | `#b91c1c` | Muted accent for destructive tag borders |
| `RedBg` | `#1c0a09` | Destructive pill background |

### State Colors

| Variable | Hex | Semantic Mapping |
|----------|-----|------------------|
| `GreenColor` | `#22c55e` | active, open, bonded, passed, synced, voted, healthy |
| `GreenDim` | `#166534` | Green state tag border |
| `YellowColor` | `#eab308` | paused, insufficient_funds, unbonding, warning, proposer |
| `YellowDim` | `#854d0e` | Yellow state tag border |
| `BlueColor` | `#65b3ff` | Informational, precommit progress, IBC-related |
| `PurpleColor` | `#c08cff` | Supplementary accent |

### Typography Styles

| Style | Foreground | Weight | Usage |
|-------|------------|--------|-------|
| `Heading` | Slate100 | Bold | Section headings, view titles |
| `PrimaryValue` | Slate200 | Bold | Important numeric values, highlighted data |
| `Body` | Slate300 | Normal | Default body text, table cell values |
| `Secondary` | Slate400 | Normal | Secondary information, dimmed text |
| `Muted` | Slate500 | Normal | Least-emphasis text, column headers, labels |

### Component Styles

#### Header Bar

| Style | Properties |
|-------|------------|
| `HeaderStyle` | `Background(Slate900)`, `Padding(0, 1)` |
| `HeaderAppName` | `Foreground(AccentRed)`, `Bold(true)` |
| `HeaderContext` | `Foreground(Slate200)`, `Bold(true)` |
| `HeaderMeta` | `Foreground(Slate500)` |
| `HeaderValue` | `Foreground(Slate200)` |
| `SyncOK` | `Foreground(GreenColor)` |

#### Navigation Tabs

| Style | Properties |
|-------|------------|
| `NavTabActive` | `Background(AccentRed)`, `Foreground(Slate950)`, `Bold(true)`, `Padding(0, 1)` |
| `NavTabInactive` | `Foreground(Slate400)`, `Padding(0, 1)` |

#### Breadcrumb

| Style | Properties |
|-------|------------|
| `BreadcrumbSegment` | `Foreground(Slate500)` |
| `BreadcrumbActive` | `Foreground(Slate200)`, `Bold(true)` |
| `BreadcrumbSeparator` | `Foreground(Slate600)` |

#### Footer

| Style | Properties |
|-------|------------|
| `FooterKey` | `Foreground(Slate400)`, `Bold(true)` |
| `FooterDesc` | `Foreground(Slate600)` |
| Accent key (in `footer.go`) | `Foreground(AccentRed)`, `Bold(true)` |

#### Table

| Style | Properties |
|-------|------------|
| `TableHeader` | `Foreground(Slate500)` |
| `TableRow` | `Foreground(Slate300)` |
| `TableRowSelected` | `Background(Slate800)`, `Foreground(Slate200)`, `Bold(true)` |
| `TableCursor` | `Foreground(AccentRed)`, `Bold(true)` |

#### KV Detail

| Style | Properties |
|-------|------------|
| `SectionTitle` | `Foreground(Slate100)`, `Bold(true)` |
| `SectionRule` | `Foreground(AccentRed)` |
| `KVLabel` | `Foreground(Slate500)`, `Width(16)` |
| `KVValue` | `Foreground(Slate200)` |

#### Panel / Card

| Style | Properties |
|-------|------------|
| `PanelBorder` | `Foreground(Slate700)` |
| `PanelBg` | `Background(Slate900)` |

#### State Tags

| Style | Properties | Used For |
|-------|------------|----------|
| `StateGreen` | `Foreground(GreenColor)` | active, open, bonded, passed |
| `StateYellow` | `Foreground(YellowColor)` | paused, insufficient_funds, unbonding |
| `StateClosed` | `Foreground(Slate500)` | closed, inactive, rejected |

#### Overlay

| Style | Properties |
|-------|------------|
| `OverlayBg` | `Background(Slate900)` |
| `OverlayBorder` | `Foreground(Slate700)` |

#### Buttons

| Style | Properties |
|-------|------------|
| `ButtonPrimary` | `Background(AccentRed)`, `Foreground(Slate950)`, `Bold(true)` |
| `ButtonSecondary` | `Background(Slate800)`, `Foreground(Slate300)` |

#### Progress Bar

| Style | Properties |
|-------|------------|
| `BarFilled` | `Foreground(Slate200)` |
| `BarEmpty` | `Foreground(Slate700)` |

#### Spinner

| Style | Properties |
|-------|------------|
| `SpinnerStyle` | `Foreground(AccentRed)` |

### Layout Helpers

| Function | Behavior |
|----------|----------|
| `HRule(w int) string` | Returns a full-width horizontal rule of `"─"` characters in `Slate700` |

## Color Tokens Used

All color tokens are defined in `internal/ui/theme/theme.go`. The complete set:

**Zinc scale**: `Slate950`, `Slate900`, `Slate800`, `Slate700`, `Slate600`, `Slate500`, `Slate400`, `Slate300`, `Slate200`, `Slate100`, `Slate50`

**Accent**: `AccentRed`, `RedDim`, `RedBg`

**State**: `GreenColor`, `GreenDim`, `YellowColor`, `YellowDim`, `BlueColor`, `PurpleColor`

## Interaction

### NO_COLOR / Non-TTY Support

The theme relies on lipgloss v2 which automatically disables color output when the output is not a TTY (e.g., piped to a file). This provides implicit `NO_COLOR` support without any custom logic in the theme package. When colors are disabled, all styled text renders as plain text with no ANSI escape sequences.

### Dark Theme Only

The current implementation provides a single dark theme. All color values are hardcoded as `lipgloss.Color()` hex strings. The SPEC.md describes a configuration-driven theme system with `tui.theme` and `tui.custom-themes` YAML support, but this is not yet implemented. The backward-compatible aliases section in `theme.go` preserves the previous 256-color theme's exported names for compilation compatibility.

### Backward Compatibility

The theme file exports a large set of deprecated aliases (lines 179-333) that map old names to the new Zinc-scale equivalents. These exist so that existing packages (monitor, pretty output, TUI views) continue to compile without changes. Each alias is annotated with a `// Deprecated:` comment indicating the preferred replacement.

## Data Sources

The theme is a pure style library with no data dependencies. All values are compile-time constants.

## Implementation Reference

| Component | File |
|-----------|------|
| Complete theme definition | `internal/ui/theme/theme.go` |
| Zinc scale (lines 22-34) | `internal/ui/theme/theme.go:22-34` |
| Accent colors (lines 38-42) | `internal/ui/theme/theme.go:38-42` |
| State colors (lines 46-53) | `internal/ui/theme/theme.go:46-53` |
| Typography (lines 57-63) | `internal/ui/theme/theme.go:57-63` |
| Header bar styles (lines 67-74) | `internal/ui/theme/theme.go:67-74` |
| Nav tab styles (lines 78-88) | `internal/ui/theme/theme.go:78-88` |
| Breadcrumb styles (lines 92-96) | `internal/ui/theme/theme.go:92-96` |
| Footer styles (lines 100-103) | `internal/ui/theme/theme.go:100-103` |
| Table styles (lines 107-112) | `internal/ui/theme/theme.go:107-112` |
| KV detail styles (lines 116-121) | `internal/ui/theme/theme.go:116-121` |
| Panel styles (lines 125-128) | `internal/ui/theme/theme.go:125-128` |
| State tag styles (lines 132-136) | `internal/ui/theme/theme.go:132-136` |
| Overlay styles (lines 140-143) | `internal/ui/theme/theme.go:140-143` |
| Button styles (lines 147-150) | `internal/ui/theme/theme.go:147-150` |
| Progress bar styles (lines 154-157) | `internal/ui/theme/theme.go:154-157` |
| Spinner style (lines 161-163) | `internal/ui/theme/theme.go:161-163` |
| HRule helper (lines 168-170) | `internal/ui/theme/theme.go:168-170` |
| Backward-compat aliases (lines 179-333) | `internal/ui/theme/theme.go:179-333` |
| Footer accent key style | `internal/tui/components/footer.go:21` |

## SPEC.md Cross-Reference

- **Section 8.7 — Theme System**: Describes the planned theme configuration with `tui.theme`, `tui.custom-themes`, and per-theme color/style overrides. The current implementation provides only the `dark` theme as hardcoded values; the YAML-driven custom theme system is not yet built.
- **Section 8.1 — Application Shell Layout**: Header, status bar, and main area styling all reference tokens from this theme.
