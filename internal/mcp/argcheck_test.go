package mcp

import (
	"math"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func schema(props map[string]any) mcp.ToolInputSchema {
	return mcp.ToolInputSchema{Type: "object", Properties: props}
}

// TestCheckArgsRejectsWrongTypes pins the fix for optional arguments being
// silently discarded when they had the wrong type: an assistant that asked for
// a specific page size got a different one, reported as success.
func TestCheckArgsRejectsWrongTypes(t *testing.T) {
	s := schema(map[string]any{
		"limit": map[string]any{"type": "number"},
		"dseq":  map[string]any{"type": "string"},
		"force": map[string]any{"type": "boolean"},
	})

	cases := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{"string for number", map[string]any{"limit": "fifty"}, "parameter limit must be a number"},
		{"number for string", map[string]any{"dseq": float64(12345)}, "parameter dseq must be a string"},
		{"string for boolean", map[string]any{"force": "yes"}, "parameter force must be a boolean"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkArgs(s, tc.args)
			if err == nil {
				t.Fatalf("%v was accepted; it must be refused", tc.args)
			}
			if err.Error() != tc.wantErr {
				t.Errorf("error = %q, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestCheckArgsAcceptsCorrectTypes(t *testing.T) {
	s := schema(map[string]any{
		"limit": map[string]any{"type": "number"},
		"dseq":  map[string]any{"type": "string"},
		"force": map[string]any{"type": "boolean"},
	})

	args := map[string]any{"limit": float64(5), "dseq": "12345", "force": true}
	if err := checkArgs(s, args); err != nil {
		t.Fatalf("valid arguments rejected: %v", err)
	}
}

func TestCheckArgsEnforcesNumericConstraints(t *testing.T) {
	s := schema(map[string]any{
		"limit": map[string]any{
			"type":       "number",
			"minimum":    float64(0),
			"maximum":    float64(200),
			"multipleOf": float64(1),
		},
	})

	cases := []struct {
		name    string
		value   float64
		wantErr string
	}{
		{"negative", -1, "parameter limit must be greater than or equal to 0"},
		{"fractional", 1.5, "parameter limit must be a whole number"},
		{"above maximum", 201, "parameter limit must be less than or equal to 200"},
		{"not a number", math.NaN(), "parameter limit must be a finite number"},
		{"infinite", math.Inf(1), "parameter limit must be a finite number"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkArgs(s, map[string]any{"limit": tc.value})
			if err == nil {
				t.Fatalf("value %v was accepted", tc.value)
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("error = %q, want %q", err, tc.wantErr)
			}
		})
	}

	for _, value := range []float64{0, 1, 200} {
		if err := checkArgs(s, map[string]any{"limit": value}); err != nil {
			t.Errorf("valid value %v rejected: %v", value, err)
		}
	}

	integerOnly := schema(map[string]any{
		"dseq": map[string]any{"type": "number", "multipleOf": float64(1)},
	})
	if err := checkArgs(integerOnly, map[string]any{"dseq": 1_000_000_000_000.5}); err == nil {
		t.Fatal("large fractional identifier was accepted")
	} else if err.Error() != "parameter dseq must be a whole number" {
		t.Fatalf("large fractional error = %q", err)
	}
}

// An argument the schema does not declare stays ignored: a client sending an
// extra field should not have the call fail.
func TestCheckArgsIgnoresUndeclaredArgument(t *testing.T) {
	s := schema(map[string]any{"limit": map[string]any{"type": "number"}})

	if err := checkArgs(s, map[string]any{"bogus": "anything"}); err != nil {
		t.Fatalf("undeclared argument rejected: %v", err)
	}
}

// TestCheckArgsEnforcesEnum: the enum arrives as []string from mcp.Enum, which
// an earlier version of this check missed, so a bad value still round-tripped
// into a 400 from the far end.
func TestCheckArgsEnforcesEnum(t *testing.T) {
	s := schema(map[string]any{
		"scope": map[string]any{"type": "string", "enum": []string{"all", "trial"}},
	})

	if err := checkArgs(s, map[string]any{"scope": "active"}); err == nil {
		t.Fatal("a value outside the enum was accepted")
	} else if err.Error() != "parameter scope must be one of all, trial" {
		t.Errorf("unexpected error: %v", err)
	}

	for _, ok := range []string{"all", "trial"} {
		if err := checkArgs(s, map[string]any{"scope": ok}); err != nil {
			t.Errorf("valid enum value %q rejected: %v", ok, err)
		}
	}
}

// The same check must work when the schema came back through a JSON round
// trip, where the enum is []any.
func TestCheckArgsEnforcesEnumFromJSON(t *testing.T) {
	s := schema(map[string]any{
		"scope": map[string]any{"type": "string", "enum": []any{"all", "trial"}},
	})

	if err := checkArgs(s, map[string]any{"scope": "active"}); err == nil {
		t.Fatal("a value outside the enum was accepted")
	}
}
