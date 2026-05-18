package components_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/tui/components"
)

func TestConfirmDialogClose(t *testing.T) {
	d := components.NewConfirmDialog(components.ConfirmClose, components.ConfirmData{
		Title:   "Close Deployment",
		Body:    "This will permanently close the deployment.",
		Danger:  true,
		Fee:     "0.0142 AKT",
		Account: "akash1abc...def",
	})
	d.Open()
	d.SetSize(80, 24)

	out := d.View()
	if out == "" {
		t.Fatal("View() returned empty string for active dialog")
	}

	plain := ansi.Strip(out)

	if !strings.Contains(plain, "Close Deployment") {
		t.Error("View() missing title")
	}
	if !strings.Contains(plain, "cancel") {
		t.Error("View() missing cancel hint")
	}
	if !strings.Contains(plain, "confirm") {
		t.Error("View() missing confirm hint")
	}
	if !strings.Contains(plain, "0.0142 AKT") {
		t.Error("View() missing fee preview")
	}
	if !strings.Contains(plain, "akash1abc...def") {
		t.Error("View() missing account")
	}
	if !strings.Contains(plain, "destructive") {
		t.Error("View() missing destructive badge for danger dialog")
	}
}

func TestConfirmDialogVote(t *testing.T) {
	d := components.NewConfirmDialog(components.ConfirmVote, components.ConfirmData{
		Title: "Vote on Proposal",
		Body:  "Cast your vote on proposal #42.",
	})
	d.Open()
	d.SetSize(80, 24)

	out := d.View()
	if out == "" {
		t.Fatal("View() returned empty string for vote dialog")
	}

	plain := ansi.Strip(out)

	for _, opt := range []string{"yes", "no", "abstain", "veto"} {
		if !strings.Contains(plain, opt) {
			t.Errorf("View() missing vote option %q", opt)
		}
	}
}

func TestConfirmDialogToggle(t *testing.T) {
	d := components.NewConfirmDialog(components.ConfirmClose, components.ConfirmData{
		Title: "Close",
	})

	// Initially inactive.
	if d.Active() {
		t.Error("new dialog should not be active")
	}
	if out := d.View(); out != "" {
		t.Error("inactive dialog should render empty string")
	}

	// Open.
	d.Open()
	if !d.Active() {
		t.Error("dialog should be active after Open()")
	}

	// Close.
	d.Close()
	if d.Active() {
		t.Error("dialog should not be active after Close()")
	}
	if out := d.View(); out != "" {
		t.Error("closed dialog should render empty string")
	}
}

func TestConfirmDialogEscSendsCancelMsg(t *testing.T) {
	d := components.NewConfirmDialog(components.ConfirmClose, components.ConfirmData{
		Title: "Close",
	})
	d.Open()
	d.SetSize(80, 24)

	cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if d.Active() {
		t.Error("dialog should be closed after esc")
	}
	if cmd == nil {
		t.Fatal("esc should produce a command")
	}
	msg := cmd()
	if _, ok := msg.(components.CancelMsg); !ok {
		t.Errorf("esc should produce CancelMsg, got %T", msg)
	}
}

func TestConfirmDialogEnterSendsConfirmMsg(t *testing.T) {
	d := components.NewConfirmDialog(components.ConfirmClose, components.ConfirmData{
		Title: "Close",
	})
	d.Open()
	d.SetSize(80, 24)

	cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if d.Active() {
		t.Error("dialog should be closed after enter")
	}
	if cmd == nil {
		t.Fatal("enter should produce a command")
	}
	msg := cmd()
	cm, ok := msg.(components.ConfirmMsg)
	if !ok {
		t.Fatalf("enter should produce ConfirmMsg, got %T", msg)
	}
	if cm.Kind != components.ConfirmClose {
		t.Errorf("ConfirmMsg.Kind = %d, want ConfirmClose", cm.Kind)
	}
}

func TestConfirmDialogVoteKeys(t *testing.T) {
	tests := []struct {
		key  string
		code rune
		want string
	}{
		{"y", 'y', "yes"},
		{"n", 'n', "no"},
		{"a", 'a', "abstain"},
		{"v", 'v', "veto"},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			d := components.NewConfirmDialog(components.ConfirmVote, components.ConfirmData{
				Title: "Vote",
			})
			d.Open()
			d.SetSize(80, 24)

			// Press the vote key to select the option.
			d.Update(tea.KeyPressMsg{Code: tc.code})

			// Press enter to confirm.
			cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("enter should produce a command")
			}
			msg := cmd()
			cm, ok := msg.(components.ConfirmMsg)
			if !ok {
				t.Fatalf("expected ConfirmMsg, got %T", msg)
			}
			if cm.VoteOption != tc.want {
				t.Errorf("VoteOption = %q, want %q", cm.VoteOption, tc.want)
			}
		})
	}
}

func TestConfirmDialogTabCyclesFocus(t *testing.T) {
	d := components.NewConfirmDialog(components.ConfirmClose, components.ConfirmData{
		Title: "Close",
	})
	d.Open()
	d.SetSize(80, 24)

	// Tab should not produce a command (just cycles focus).
	cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if cmd != nil {
		t.Error("tab should not produce a command")
	}
	if !d.Active() {
		t.Error("tab should not close the dialog")
	}
}

func TestConfirmDialogInactiveUpdateNoop(t *testing.T) {
	d := components.NewConfirmDialog(components.ConfirmClose, components.ConfirmData{
		Title: "Close",
	})
	// Don't open — should be a no-op.
	cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("update on inactive dialog should return nil")
	}
}
