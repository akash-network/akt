package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pkg.akt.dev/akt/internal/tui/views"
)

// activeView tracks which panel is displayed in the main area.
type activeView int

const (
	viewDashboard activeView = iota
	viewQuery
	viewTx
	viewTop
)

// App is the root bubbletea model for the akt TUI.
type App struct {
	keys    KeyMap
	view    activeView
	query   views.QueryView
	tx      views.TxView
	top     views.TopView
	search  views.SearchDialog
	command views.CommandInput
	width   int
	height  int
}

// New returns a new App model.
func New() App {
	return App{
		keys:    DefaultKeyMap(),
		view:    viewDashboard,
		query:   views.NewQueryView(),
		tx:      views.NewTxView(),
		top:     views.NewTopView(),
		search:  views.NewSearchDialog(),
		command: views.NewCommandInput(),
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle command submission (arrives after command input closes).
	if msg, ok := msg.(views.CommandSubmitMsg); ok {
		return a.handleCommand(msg.Value)
	}

	// Command input takes priority when active.
	if a.command.Active() {
		cmd := a.command.Update(msg)
		return a, cmd
	}

	// Search dialog takes priority when active.
	if a.search.Active() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if key.Matches(msg, a.keys.Back) {
				a.search.Close()
				return a, nil
			}
		}

		cmd := a.search.Update(msg)
		return a, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resize()
		return a, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, a.keys.Quit):
			return a, tea.Quit

		case key.Matches(msg, a.keys.Command):
			a.command.Open()
			return a, nil

		case key.Matches(msg, a.keys.Query):
			a.view = viewQuery
			return a, nil

		case key.Matches(msg, a.keys.Tx):
			a.view = viewTx
			return a, nil

		case key.Matches(msg, a.keys.Top):
			a.view = viewTop
			return a, nil

		case key.Matches(msg, a.keys.CommandSearch):
			a.search.Open()
			return a, nil

		case key.Matches(msg, a.keys.Back):
			a.view = viewDashboard
			return a, nil
		}
	}

	return a, nil
}

// View implements tea.Model.
func (a App) View() string {
	header := a.renderHeader()

	// When the command input is active it replaces the status bar.
	var status string
	if a.command.Active() {
		status = a.command.View()
	} else {
		status = a.renderStatus()
	}

	// Main area height = total - header (1) - status (1) - borders (2).
	mainH := a.height - 4
	if mainH < 1 {
		mainH = 1
	}

	var main string
	switch a.view {
	case viewQuery:
		main = a.query.View()
	case viewTx:
		main = a.tx.View()
	case viewTop:
		main = a.top.View()
	default:
		main = a.renderDashboard(mainH)
	}

	// If search dialog is active, overlay it on the main area.
	if a.search.Active() {
		main = a.search.View()
	}

	return header + "\n" + main + "\n" + status
}

// handleCommand dispatches a vim-style command entered via :.
func (a App) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "q", "quit":
		return a, tea.Quit
	case "top":
		a.view = viewTop
		return a, nil
	}

	return a, nil
}

func (a *App) resize() {
	mainH := a.height - 4
	if mainH < 1 {
		mainH = 1
	}

	a.query.SetSize(a.width, mainH)
	a.tx.SetSize(a.width, mainH)
	a.top.SetSize(a.width, mainH)
	a.search.SetSize(a.width, mainH)
	a.command.SetWidth(a.width)
}

func (a App) renderHeader() string {
	style := lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Width(a.width).
		Padding(0, 1)

	return style.Render("akt")
}

func (a App) renderStatus() string {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Width(a.width).
		Padding(0, 1)

	return style.Render("q: query  t: tx  :: command  ctrl+p: search  1: top  esc: back  ctrl+c: quit")
}

func (a App) renderDashboard(h int) string {
	style := lipgloss.NewStyle().
		Width(a.width).
		Height(h).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render("akt - Akash Network")
}

// Run starts the TUI application on the dashboard view.
func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// RunTop starts the TUI application directly in the consensus monitor view.
func RunTop() error {
	app := New()
	app.view = viewTop

	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
