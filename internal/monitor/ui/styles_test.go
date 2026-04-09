package ui

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestProgressBar(t *testing.T) {
	tests := map[string]struct {
		percent float64
		width   int
	}{
		"Zero":     {0.0, 40},
		"Half":     {0.5, 40},
		"Full":     {1.0, 40},
		"Overflow": {1.5, 40},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, ProgressBar(tc.percent, tc.width))
		})
	}
}

func TestFormatPercentStyles(t *testing.T) {
	tests := map[string]struct {
		percent float64
	}{
		"BelowThreshold": {0.50},
		"AtThreshold":    {0.667},
		"AboveThreshold": {0.95},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatPercent(tc.percent))
		})
	}
}

func TestDoubleProgressBar(t *testing.T) {
	tests := map[string]struct {
		pvPct float64
		pcPct float64
		width int
	}{
		"BothZero":    {0.0, 0.0, 40},
		"BothFull":    {1.0, 1.0, 40},
		"PVHighPCLow": {0.95, 0.3, 40},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, DoubleProgressBar(tc.pvPct, tc.pcPct, tc.width))
		})
	}
}

func TestFormatVoteGrid(t *testing.T) {
	tests := map[string]struct {
		pattern string
		width   int
	}{
		"Empty":       {"", 20},
		"AllVoted":    {"xxxxxxxxxx", 20},
		"Mixed":       {"xxx___x__x", 20},
		"WrapAtWidth": {"xxxx____xxxx____xxxx", 10},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, FormatVoteGrid(tc.pattern, tc.width))
		})
	}
}

func TestColorVotePercent(t *testing.T) {
	tests := map[string]struct {
		pct float64
	}{
		"Low":  {0.50},
		"High": {0.95},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, colorVotePercent(tc.pct))
		})
	}
}
