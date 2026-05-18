package components_test

import (
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/tui/components"
)

func TestToastCreation(t *testing.T) {
	toast := components.NewToast("deployed", components.ToastOK)
	if toast.Message != "deployed" {
		t.Errorf("Message = %q, want %q", toast.Message, "deployed")
	}
	if toast.Tone != components.ToastOK {
		t.Errorf("Tone = %d, want %d", toast.Tone, components.ToastOK)
	}
	if toast.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestToastExpiry(t *testing.T) {
	fresh := components.NewToast("hello", components.ToastInfo)
	if fresh.Expired() {
		t.Error("freshly created toast should not be expired")
	}

	old := components.Toast{
		Message:   "old",
		Tone:      components.ToastError,
		CreatedAt: time.Now().Add(-3 * time.Second),
	}
	if !old.Expired() {
		t.Error("toast created 3s ago should be expired")
	}
}

func TestToastView(t *testing.T) {
	tones := []components.ToastTone{
		components.ToastOK,
		components.ToastInfo,
		components.ToastError,
	}
	for _, tone := range tones {
		toast := components.NewToast("test message", tone)
		out := toast.View()
		if out == "" {
			t.Errorf("View() for tone %d returned empty string", tone)
		}
	}
}
