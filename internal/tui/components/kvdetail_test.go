package components_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/tui/components"
)

func TestSection(t *testing.T) {
	result := components.Section("Overview", 40)
	if result == "" {
		t.Fatal("Section returned empty string")
	}

	plain := ansi.Strip(result)
	if !strings.Contains(plain, "Overview") {
		t.Errorf("Section plain text should contain title; got %q", plain)
	}
	if !strings.Contains(plain, "─") {
		t.Error("Section should contain a horizontal rule character")
	}
}

func TestKV(t *testing.T) {
	result := components.KV("Status", "active")
	if result == "" {
		t.Fatal("KV returned empty string")
	}

	plain := ansi.Strip(result)
	if !strings.Contains(plain, "Status") {
		t.Errorf("KV plain text should contain label; got %q", plain)
	}
	if !strings.Contains(plain, "active") {
		t.Errorf("KV plain text should contain value; got %q", plain)
	}
}

func TestKVMuted(t *testing.T) {
	result := components.KVMuted("Note", "n/a")
	if result == "" {
		t.Fatal("KVMuted returned empty string")
	}

	plain := ansi.Strip(result)
	if !strings.Contains(plain, "Note") {
		t.Errorf("KVMuted plain text should contain label; got %q", plain)
	}
	if !strings.Contains(plain, "n/a") {
		t.Errorf("KVMuted plain text should contain value; got %q", plain)
	}
}

func TestKVBold(t *testing.T) {
	result := components.KVBold("Name", "my-deploy")
	if result == "" {
		t.Fatal("KVBold returned empty string")
	}

	plain := ansi.Strip(result)
	if !strings.Contains(plain, "Name") {
		t.Errorf("KVBold plain text should contain label; got %q", plain)
	}
	if !strings.Contains(plain, "my-deploy") {
		t.Errorf("KVBold plain text should contain value; got %q", plain)
	}
}

func TestKVBlock(t *testing.T) {
	pairs := []components.KVPair{
		{Label: "Status", Value: "active"},
		{Label: "Owner", Value: "akash1abc"},
		{Label: "Balance", Value: "100 AKT"},
	}

	result := components.KVBlock(pairs)
	if result == "" {
		t.Fatal("KVBlock returned empty string")
	}

	plain := ansi.Strip(result)
	for _, p := range pairs {
		if !strings.Contains(plain, p.Label) {
			t.Errorf("KVBlock should contain label %q; got %q", p.Label, plain)
		}
		if !strings.Contains(plain, p.Value) {
			t.Errorf("KVBlock should contain value %q; got %q", p.Value, plain)
		}
	}

	// Should have one line per pair.
	lines := strings.Split(result, "\n")
	if len(lines) != len(pairs) {
		t.Errorf("KVBlock should have %d lines; got %d", len(pairs), len(lines))
	}
}

func TestSectionWithKV(t *testing.T) {
	pairs := []components.KVPair{
		{Label: "Height", Value: "12345"},
		{Label: "Time", Value: "2024-01-01"},
	}

	result := components.SectionWithKV("Block Info", 30, pairs)
	if result == "" {
		t.Fatal("SectionWithKV returned empty string")
	}

	plain := ansi.Strip(result)
	if !strings.Contains(plain, "Block Info") {
		t.Errorf("SectionWithKV should contain section title; got %q", plain)
	}
	for _, p := range pairs {
		if !strings.Contains(plain, p.Label) {
			t.Errorf("SectionWithKV should contain label %q", p.Label)
		}
		if !strings.Contains(plain, p.Value) {
			t.Errorf("SectionWithKV should contain value %q", p.Value)
		}
	}
}
