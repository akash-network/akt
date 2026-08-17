package mcp

import (
	"context"
	"math"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
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

func TestCheckArgsRejectsOnlySchemaRequiredBlankStrings(t *testing.T) {
	s := mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"required":  map[string]any{"type": "string"},
			"minLength": map[string]any{"type": "string", "minLength": float64(1)},
			"filter":    map[string]any{"type": "string"},
		},
		Required: []string{"required"},
	}
	for _, name := range []string{"required", "minLength"} {
		args := map[string]any{"required": "value"}
		args[name] = " \t"
		if err := checkArgs(s, args); err == nil || err.Error() != "parameter "+name+" must not be empty" {
			t.Fatalf("blank %s error = %v", name, err)
		}
	}
	if err := checkArgs(s, map[string]any{"required": "value", "filter": "  "}); err != nil {
		t.Fatalf("blank optional filter was not treated as omitted: %v", err)
	}
	normalized := normalizedOptionalStrings(s, map[string]any{"filter": "  ", "keep": "value"})
	if _, exists := normalized["filter"]; exists || normalized["keep"] != "value" {
		t.Fatalf("normalized optional strings = %#v", normalized)
	}
	if err := checkArgs(s, map[string]any{}); err == nil || err.Error() != "missing required parameter: required" {
		t.Fatalf("missing required key error = %v", err)
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

// An argument the schema does not declare is usually a typo. Reject it before
// an optional value silently falls back to the handler's default.
func TestCheckArgsRejectsUndeclaredArgumentDeterministically(t *testing.T) {
	s := schema(map[string]any{"limit": map[string]any{"type": "number"}})

	err := checkArgs(s, map[string]any{"z_typo": true, "a_typo": "anything"})
	if err == nil || err.Error() != "unknown parameter: a_typo" {
		t.Fatalf("undeclared argument error = %v", err)
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
	if err := checkArgs(s, map[string]any{"scope": "trial"}); err != nil {
		t.Fatalf("valid JSON-round-tripped enum value rejected: %v", err)
	}
}

func TestCheckArgsEnforcesEveryJSONContainerAndIntegerType(t *testing.T) {
	cases := []struct {
		name    string
		spec    map[string]any
		value   any
		wantErr string
	}{
		{name: "array", spec: map[string]any{"type": "array"}, value: []any{"gpu", float64(1)}},
		{name: "array rejects scalar", spec: map[string]any{"type": "array"}, value: "gpu", wantErr: "parameter value must be a array"},
		{name: "object", spec: map[string]any{"type": "object"}, value: map[string]any{"cpu": float64(1)}},
		{name: "object rejects array", spec: map[string]any{"type": "object"}, value: []any{}, wantErr: "parameter value must be a object"},
		{name: "integer", spec: map[string]any{"type": "integer"}, value: float64(12)},
		{name: "integer rejects fraction", spec: map[string]any{"type": "integer"}, value: 12.5, wantErr: "parameter value must be a whole number"},
		{name: "future schema type fails open", spec: map[string]any{"type": "null"}, value: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkArgs(schema(map[string]any{"value": tc.spec}), map[string]any{"value": tc.value})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid value rejected: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestCheckArgsEnforcesFractionalMultiplesWithoutRejectingJSONRounding(t *testing.T) {
	s := schema(map[string]any{
		"amount": map[string]any{"type": "number", "multipleOf": 0.25},
	})
	for _, value := range []float64{0, 0.25, 1.25, 2.5} {
		if err := checkArgs(s, map[string]any{"amount": value}); err != nil {
			t.Errorf("valid multiple %v rejected: %v", value, err)
		}
	}
	if err := checkArgs(s, map[string]any{"amount": 1.2}); err == nil || err.Error() != "parameter amount must be a multiple of 0.25" {
		t.Fatalf("non-multiple error = %v", err)
	}

	// Schemas assembled programmatically can contain non-property metadata.
	// The validator must leave that metadata to the MCP library rather than
	// panicking on a failed type assertion.
	if err := checkArgs(schema(map[string]any{"metadata": "external"}), map[string]any{"metadata": true}); err != nil {
		t.Fatalf("non-map schema property was not ignored safely: %v", err)
	}
}

func TestArgumentMiddlewareStopsInvalidCallsAndNormalizesOptionalBlanks(t *testing.T) {
	server := &Server{schemas: map[string]mcp.ToolInputSchema{
		"list": {
			Type: "object",
			Properties: map[string]any{
				"owner": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "number"},
			},
		},
	}}

	calls := 0
	var forwarded map[string]any
	next := mcpserver.ToolHandlerFunc(func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls++
		forwarded = req.GetArguments()
		return &mcp.CallToolResult{}, nil
	})
	handler := server.validateArguments(next)

	bad := mcp.CallToolRequest{}
	bad.Params.Name = "list"
	bad.Params.Arguments = map[string]any{"limit": "ten"}
	result, err := handler(context.Background(), bad)
	if err != nil || result == nil || !result.IsError || calls != 0 {
		t.Fatalf("invalid call result=%#v err=%v downstream calls=%d", result, err, calls)
	}

	valid := mcp.CallToolRequest{}
	valid.Params.Name = "list"
	valid.Params.Arguments = map[string]any{"owner": " \t", "limit": float64(10)}
	result, err = handler(context.Background(), valid)
	if err != nil || result == nil || result.IsError || calls != 1 {
		t.Fatalf("valid call result=%#v err=%v downstream calls=%d", result, err, calls)
	}
	if _, exists := forwarded["owner"]; exists || forwarded["limit"] != float64(10) {
		t.Fatalf("forwarded arguments = %#v", forwarded)
	}

	// An unknown tool is the protocol server's responsibility. The schema
	// middleware must not claim or rewrite it before normal dispatch.
	unknown := mcp.CallToolRequest{}
	unknown.Params.Name = "future_tool"
	unknown.Params.Arguments = map[string]any{"opaque": true}
	if _, err := handler(context.Background(), unknown); err != nil || calls != 2 {
		t.Fatalf("unknown tool did not pass through: err=%v downstream calls=%d", err, calls)
	}
}
