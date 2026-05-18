package components

import (
	"image/color"
	"time"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ToastDuration is how long a toast notification is displayed before expiring.
const ToastDuration = 2500 * time.Millisecond

// ToastTone controls the icon and border color of a toast notification.
type ToastTone int

const (
	ToastOK   ToastTone = iota
	ToastInfo
	ToastError
)

// Toast is a transient notification that auto-dismisses after toastDuration.
type Toast struct {
	Message   string
	Tone      ToastTone
	CreatedAt time.Time
}

// ToastExpiredMsg is sent when a toast should be removed.
type ToastExpiredMsg struct{}

// NewToast creates a toast stamped with the current time.
func NewToast(message string, tone ToastTone) Toast {
	return Toast{
		Message:   message,
		Tone:      tone,
		CreatedAt: time.Now(),
	}
}

// Expired reports whether the toast has exceeded its display duration.
func (t Toast) Expired() bool {
	return time.Since(t.CreatedAt) >= ToastDuration
}

// toneStyle returns the icon string and border color for the toast's tone.
func toneStyle(tone ToastTone) (icon string, borderColor color.Color, iconColor color.Color) {
	switch tone {
	case ToastOK:
		return "✓", theme.GreenDim, theme.GreenColor
	case ToastInfo:
		return "ℹ", theme.BlueColor, theme.BlueColor
	case ToastError:
		return "✗", theme.RedDim, theme.AccentRed
	default:
		return "ℹ", theme.BlueColor, theme.BlueColor
	}
}

// View renders the toast as a rounded box with an icon and message.
func (t Toast) View() string {
	icon, borderColor, iconColor := toneStyle(t.Tone)

	iconStyle := lipgloss.NewStyle().Foreground(iconColor)
	msgStyle := lipgloss.NewStyle().Foreground(theme.Slate300)

	content := iconStyle.Render(icon) + " " + msgStyle.Render(t.Message)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(theme.Slate900).
		Padding(0, 1).
		MaxWidth(50)

	return box.Render(content)
}
