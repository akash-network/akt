package views

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/akt/internal/output/pretty"
	"pkg.akt.dev/akt/internal/tui/components"
)

// ViewComponent is the contract every navigable view must satisfy.
// It extends tea.Model with methods the App shell uses for chrome
// rendering (breadcrumb, footer hints) and layout (resize).
type ViewComponent interface {
	tea.Model

	// SetSize is called when the terminal resizes. w and h are the
	// available content area dimensions (header/footer already subtracted).
	SetSize(w, h int)

	// Breadcrumb returns the navigation label for this view.
	// Examples: "Deployments", "Deployment #12345", "Lease Detail"
	Breadcrumb() string

	// ShortHelp returns the footer hint pairs for this view.
	ShortHelp() []components.HintPair

	// Refresh returns a tea.Cmd that re-fires data loads for this view.
	// Called by the App when ViewDataRefreshMsg is received (sync bridge
	// detected a store change). Views that load data in Init() should
	// re-fire those same loads here. Views with no data loading return nil.
	Refresh() tea.Cmd
}

// Truncate shortens s to maxLen characters, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// CommaGroup formats an integer with comma separators.
func CommaGroup(n int64) string {
	if n < 0 {
		return "-" + CommaGroup(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	return CommaGroup(n/1000) + "," + fmt.Sprintf("%03d", n%1000)
}

// CmdFunc is a convenience for creating a tea.Cmd that returns a message.
func CmdFunc(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}

// formatStoredCoins converts the Cosmos coin strings persisted in store
// records into the canonical human-readable representation. Invalid legacy
// values are returned unchanged so the UI never hides stored data.
func formatStoredCoins(raw string) string {
	if raw == "" {
		return ""
	}
	coins, err := sdk.ParseDecCoins(raw)
	if err != nil {
		return raw
	}
	return pretty.FormatDecCoins(coins)
}
