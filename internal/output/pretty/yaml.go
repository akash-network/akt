package pretty

import (
	"fmt"
	"io"
	"strings"
)

// WriteHighlightedYAML writes syntax-highlighted YAML to w.
// Keys are cyan, strings green, numbers yellow, booleans magenta, null gray.
func WriteHighlightedYAML(w io.Writer, data []byte) error {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		colored := colorizeYAMLLine(line)
		fmt.Fprintln(w, colored)
	}
	return nil
}

// colorizeYAMLLine applies syntax highlighting to a single YAML line.
func colorizeYAMLLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return line
	}

	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]

	// Document markers.
	if trimmed == "---" || trimmed == "..." {
		return indent + StyleGray.Render(trimmed)
	}

	// Comments.
	if strings.HasPrefix(trimmed, "#") {
		return indent + StyleGray.Render(trimmed)
	}

	// List items: "- key: value" or "- value"
	listPrefix := ""
	content := trimmed
	if strings.HasPrefix(trimmed, "- ") {
		listPrefix = "- "
		content = trimmed[2:]
	}

	// Try to split on key: value.
	if idx := strings.Index(content, ": "); idx > 0 {
		key := content[:idx]
		val := content[idx+2:]

		coloredKey := StyleCyan.Render(key)
		coloredVal := colorizeYAMLValue(val)

		return indent + listPrefix + coloredKey + ": " + coloredVal
	}

	// Key with no value (e.g., "key:" at end of line, indicating a nested object).
	if strings.HasSuffix(content, ":") {
		key := strings.TrimSuffix(content, ":")
		return indent + listPrefix + StyleCyan.Render(key) + ":"
	}

	// Plain value (e.g., list items like "- value").
	if listPrefix != "" {
		return indent + listPrefix + colorizeYAMLValue(content)
	}

	return line
}

// colorizeYAMLValue applies color to a YAML value.
func colorizeYAMLValue(val string) string {
	clean := strings.TrimSpace(val)

	switch {
	case clean == "null" || clean == "~" || clean == "":
		return StyleGray.Render(clean)
	case clean == "true" || clean == "false":
		return StyleMagenta.Render(clean)
	case len(clean) > 0 && clean[0] == '"':
		return StyleGreen.Render(clean)
	case len(clean) > 0 && clean[0] == '\'':
		return StyleGreen.Render(clean)
	case isNumeric(clean):
		return StyleYellow.Render(clean)
	default:
		// Unquoted strings in YAML.
		return StyleGreen.Render(clean)
	}
}

// isNumeric checks if a string looks like a number.
func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	hasDot := false
	for i := start; i < len(s); i++ {
		if s[i] == '.' {
			if hasDot {
				return false
			}
			hasDot = true
		} else if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
