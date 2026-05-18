package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ConfirmKind identifies the type of destructive action being confirmed.
type ConfirmKind int

const (
	ConfirmClose      ConfirmKind = iota // close deployment
	ConfirmVote                          // governance vote
	ConfirmDelegate                      // delegate tokens
	ConfirmUnbond                        // unbond tokens
	ConfirmRedelegate                    // redelegate tokens
)

// ConfirmData holds the content displayed inside the confirmation dialog.
type ConfirmData struct {
	Title   string
	Body    string
	Danger  bool   // if true, red border instead of neutral
	Fee     string // e.g., "0.0142 AKT"
	Account string // e.g., "akash1abc...def"
}

// ConfirmMsg is sent when the user confirms the action.
type ConfirmMsg struct {
	Kind       ConfirmKind
	VoteOption string // "yes","no","abstain","veto" — only for vote variant
	Amount     string // for delegate/unbond variants
}

// CancelMsg is sent when the user cancels the dialog.
type CancelMsg struct{}

// voteOptions lists the available vote choices in display order.
var voteOptions = []string{"yes", "no", "abstain", "veto"}

// voteKeys maps shortcut keys to vote option indices.
var voteKeys = map[string]int{
	"y": 0,
	"n": 1,
	"a": 2,
	"v": 3,
}

// ConfirmDialog is a modal overlay for destructive actions.
type ConfirmDialog struct {
	kind       ConfirmKind
	data       ConfirmData
	active     bool
	width      int
	height     int
	focusBtn   int // 0 = cancel, 1 = confirm
	voteChoice int // index into voteOptions (vote variant only)
}

// NewConfirmDialog creates a confirmation dialog for the given kind and data.
func NewConfirmDialog(kind ConfirmKind, data ConfirmData) ConfirmDialog {
	return ConfirmDialog{
		kind:     kind,
		data:     data,
		focusBtn: 1, // default focus on confirm
	}
}

// Active returns whether the dialog is currently visible.
func (d ConfirmDialog) Active() bool {
	return d.active
}

// Open makes the dialog visible and resets focus state.
func (d *ConfirmDialog) Open() {
	d.active = true
	d.focusBtn = 1
	d.voteChoice = 0
}

// Close hides the dialog.
func (d *ConfirmDialog) Close() {
	d.active = false
}

// SetSize updates the available terminal dimensions for centering.
func (d *ConfirmDialog) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// Update handles key events while the dialog is active.
func (d *ConfirmDialog) Update(msg tea.Msg) tea.Cmd {
	if !d.active {
		return nil
	}

	kmsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	k := kmsg.String()

	switch k {
	case "esc":
		d.Close()
		return func() tea.Msg { return CancelMsg{} }

	case "enter":
		d.Close()
		cm := ConfirmMsg{Kind: d.kind}
		if d.kind == ConfirmVote {
			cm.VoteOption = voteOptions[d.voteChoice]
		}
		return func() tea.Msg { return cm }

	case "tab":
		d.focusBtn = (d.focusBtn + 1) % 2
		return nil
	}

	// Vote-specific shortcut keys.
	if d.kind == ConfirmVote {
		if idx, found := voteKeys[k]; found {
			d.voteChoice = idx
			return nil
		}
	}

	return nil
}

// dialogWidth is the fixed outer width of the dialog box.
const dialogWidth = 60

// View renders the dialog as a centered overlay.
func (d ConfirmDialog) View() string {
	if !d.active {
		return ""
	}

	// Border color depends on danger flag.
	borderColor := theme.Slate600
	if d.data.Danger {
		borderColor = theme.AccentRed
	}

	// Inner content width: box width minus border (2) and padding (4).
	innerW := dialogWidth - 6
	if innerW < 20 {
		innerW = 20
	}

	var sections []string

	// 1. Title bar.
	titleLine := theme.Heading.Render(d.data.Title)
	if d.data.Danger {
		badge := lipgloss.NewStyle().Foreground(theme.AccentRed).Render("destructive")
		titleLine += "  " + badge
	}
	sections = append(sections, titleLine)

	// 2. Body text.
	if d.data.Body != "" {
		body := theme.Secondary.Width(innerW).Render(d.data.Body)
		sections = append(sections, body)
	}

	// 3. Variant-specific content.
	if d.kind == ConfirmVote {
		sections = append(sections, d.renderVoteOptions(innerW))
	}

	// 4. Fee preview.
	if d.data.Fee != "" || d.data.Account != "" {
		sections = append(sections, d.renderFeePreview())
	}

	// 5. Button row.
	sections = append(sections, d.renderButtons(innerW))

	content := strings.Join(sections, "\n\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(theme.Slate900).
		Padding(1, 2).
		Width(dialogWidth)

	dialog := box.Render(content)

	w := d.width
	if w < dialogWidth {
		w = dialogWidth
	}
	h := d.height
	if h < 10 {
		h = 10
	}

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, dialog)
}

// renderVoteOptions renders the row of vote option buttons.
func (d ConfirmDialog) renderVoteOptions(innerW int) string {
	var opts []string
	for i, opt := range voteOptions {
		style := lipgloss.NewStyle().Padding(0, 1)
		if i == d.voteChoice {
			style = style.
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.AccentRed).
				Foreground(theme.Slate100)
		} else {
			style = style.Foreground(theme.Slate400)
		}
		opts = append(opts, style.Render(opt))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, opts...)
}

// renderFeePreview renders the fee and account KV lines.
func (d ConfirmDialog) renderFeePreview() string {
	var lines []string
	if d.data.Fee != "" {
		lines = append(lines, fmt.Sprintf("%s %s",
			theme.Secondary.Render("fee preview:"),
			theme.KVValue.Render(d.data.Fee)))
	}
	if d.data.Account != "" {
		lines = append(lines, fmt.Sprintf("%s %s",
			theme.Secondary.Render("from:"),
			theme.KVValue.Render(d.data.Account)))
	}
	return strings.Join(lines, "\n")
}

// renderButtons renders the cancel/confirm button row.
func (d ConfirmDialog) renderButtons(innerW int) string {
	cancelStyle := theme.ButtonSecondary.Padding(0, 1)
	confirmStyle := theme.ButtonPrimary.Padding(0, 1)

	cancelText := "esc cancel"
	confirmText := "↵ confirm"

	cancel := cancelStyle.Render(cancelText)
	confirm := confirmStyle.Render(confirmText)

	gap := innerW - lipgloss.Width(cancel) - lipgloss.Width(confirm)
	if gap < 1 {
		gap = 1
	}

	return cancel + strings.Repeat(" ", gap) + confirm
}
