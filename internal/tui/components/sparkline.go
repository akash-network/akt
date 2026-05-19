package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// sparkBlocks are the Unicode block elements from shortest to tallest.
var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline renders a mini bar chart from a slice of float64 values.
// Each value maps to one character column. The color parameter controls
// the foreground color of the bars. Width limits the number of data points
// shown (uses the rightmost values if data exceeds width).
func Sparkline(data []float64, width int, color color.Color) string {
	if len(data) == 0 || width <= 0 {
		return ""
	}

	// Use the rightmost `width` data points.
	start := 0
	if len(data) > width {
		start = len(data) - width
	}
	visible := data[start:]

	// Find min/max for normalization.
	minVal, maxVal := visible[0], visible[0]
	for _, v := range visible {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	span := maxVal - minVal
	if span == 0 {
		span = 1 // avoid division by zero; all values are equal
	}

	style := lipgloss.NewStyle().Foreground(color)
	var b strings.Builder
	for _, v := range visible {
		normalized := (v - minVal) / span
		idx := int(normalized * float64(len(sparkBlocks)-1))
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		if idx < 0 {
			idx = 0
		}
		b.WriteRune(sparkBlocks[idx])
	}

	return style.Render(b.String())
}
