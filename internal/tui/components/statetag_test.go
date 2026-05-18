package components_test

import (
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/tui/components"
)

func TestStateTag(t *testing.T) {
	got := components.StateTag("active")
	if got == "" {
		t.Fatal("StateTag(\"active\") returned empty string")
	}
	if !strings.Contains(got, "active") {
		t.Errorf("StateTag(\"active\") = %q, want it to contain \"active\"", got)
	}
}

func TestStateTagWidth(t *testing.T) {
	tests := []struct {
		state string
		want  int
	}{
		{"active", len("active") + 2},
		{"closed", len("closed") + 2},
		{"paused", len("paused") + 2},
		{"open", len("open") + 2},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			got := components.StateTagWidth(tc.state)
			if got != tc.want {
				t.Errorf("StateTagWidth(%q) = %d, want %d", tc.state, got, tc.want)
			}
		})
	}
}

func TestStateTagAbbreviation(t *testing.T) {
	tests := []struct {
		state string
		abbr  string
	}{
		{"insufficient_funds", "low funds"},
		{"voting_period", "voting"},
		{"deposit_period", "deposit"},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			got := components.StateTag(tc.state)
			if !strings.Contains(got, tc.abbr) {
				t.Errorf("StateTag(%q) = %q, want it to contain %q", tc.state, got, tc.abbr)
			}
		})
	}
}

func TestStateTagAbbreviationWidth(t *testing.T) {
	// Width should use the abbreviated label, not the original state name.
	tests := []struct {
		state string
		want  int
	}{
		{"insufficient_funds", len("low funds") + 2},
		{"voting_period", len("voting") + 2},
		{"deposit_period", len("deposit") + 2},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			got := components.StateTagWidth(tc.state)
			if got != tc.want {
				t.Errorf("StateTagWidth(%q) = %d, want %d", tc.state, got, tc.want)
			}
		})
	}
}

func TestStateTagColorMapping(t *testing.T) {
	greenStates := []string{"active", "open", "bonded", "passed", "valid", "matched"}
	yellowStates := []string{"paused", "insufficient_funds", "overdrawn", "unbonding", "voting_period", "deposit_period", "pending"}
	closedStates := []string{"closed", "lost", "unbonded", "rejected", "failed", "jailed", "revoked", "invalid"}

	for _, state := range greenStates {
		t.Run("green/"+state, func(t *testing.T) {
			got := components.StateTag(state)
			if got == "" {
				t.Errorf("StateTag(%q) returned empty string", state)
			}
		})
	}
	for _, state := range yellowStates {
		t.Run("yellow/"+state, func(t *testing.T) {
			got := components.StateTag(state)
			if got == "" {
				t.Errorf("StateTag(%q) returned empty string", state)
			}
		})
	}
	for _, state := range closedStates {
		t.Run("closed/"+state, func(t *testing.T) {
			got := components.StateTag(state)
			if got == "" {
				t.Errorf("StateTag(%q) returned empty string", state)
			}
		})
	}
}

func TestStateTagUnknownState(t *testing.T) {
	// Unknown states should still produce a non-empty tag.
	got := components.StateTag("unknown_state")
	if got == "" {
		t.Fatal("StateTag(\"unknown_state\") returned empty string")
	}
	if !strings.Contains(got, "unknown_state") {
		t.Errorf("StateTag(\"unknown_state\") = %q, want it to contain \"unknown_state\"", got)
	}
}
