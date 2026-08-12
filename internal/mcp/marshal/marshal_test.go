package marshal

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestTextAndErrorResultsPreserveStructuredMeaning(t *testing.T) {
	result, err := ToTextResult(map[string]any{"height": 42, "ready": true})
	if err != nil {
		t.Fatalf("ToTextResult: %v", err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %#v", result)
	}
	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T", result.Content[0])
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(content.Text), &decoded); err != nil {
		t.Fatalf("decode text result: %v", err)
	}
	ready, ok := decoded["ready"].(bool)
	if decoded["height"] != float64(42) || !ok || !ready {
		t.Fatalf("decoded result = %#v", decoded)
	}

	if _, err := ToTextResult(make(chan int)); err == nil || !strings.Contains(err.Error(), "marshal response") {
		t.Fatalf("unsupported response error = %v", err)
	}

	errorResult := ErrResultf("invalid %s", "request")
	if !errorResult.IsError || len(errorResult.Content) != 1 {
		t.Fatalf("error result = %#v", errorResult)
	}
	if text := errorResult.Content[0].(mcp.TextContent).Text; text != "invalid request" {
		t.Fatalf("error text = %q", text)
	}
}

func TestStringAndFloatArgumentsDistinguishMissingWrongAndValid(t *testing.T) {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"name":   "deployment",
		"amount": 1.25,
		"wrong":  true,
	}

	if got, err := RequireString(request, "name"); err != nil || got != "deployment" {
		t.Fatalf("required string = %q, err = %v", got, err)
	}
	if _, err := RequireString(request, "missing"); err == nil || !strings.Contains(err.Error(), "missing required parameter") {
		t.Fatalf("missing string error = %v", err)
	}
	if _, err := RequireString(request, "wrong"); err == nil || !strings.Contains(err.Error(), "must be a string") {
		t.Fatalf("wrong string error = %v", err)
	}
	for _, blank := range []string{"", "  \t\n"} {
		request.Params.Arguments = map[string]any{"name": blank}
		if _, err := RequireString(request, "name"); err == nil || !strings.Contains(err.Error(), "must not be empty") {
			t.Fatalf("blank required string %q error = %v", blank, err)
		}
	}
	request.Params.Arguments = map[string]any{
		"name":   "deployment",
		"amount": 1.25,
		"wrong":  true,
	}
	if got := OptionalString(request, "name"); got != "deployment" {
		t.Fatalf("optional string = %q", got)
	}
	if got := OptionalString(request, "wrong"); got != "" {
		t.Fatalf("wrong optional string = %q", got)
	}
	if got, ok := OptionalFloat(request, "amount"); !ok || got != 1.25 {
		t.Fatalf("optional float = %v, present = %t", got, ok)
	}
	if got, ok := OptionalFloat(request, "wrong"); ok || got != 0 {
		t.Fatalf("wrong optional float = %v, present = %t", got, ok)
	}
}

func numberRequest(key string, value float64) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{key: value}
	return req
}

func TestRequireUint64RejectsInvalidNumbers(t *testing.T) {
	cases := []struct {
		name  string
		value float64
	}{
		{"zero", 0},
		{"negative", -1},
		{"fractional", 15.75},
		{"not a number", math.NaN()},
		{"infinite", math.Inf(1)},
		{"overflow", math.Exp2(64)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RequireUint64(numberRequest("dseq", tc.value), "dseq")
			if err == nil {
				t.Fatalf("value %v was accepted", tc.value)
			}
			if !strings.Contains(err.Error(), "parameter dseq") {
				t.Fatalf("error does not name dseq: %v", err)
			}
		})
	}
}

func TestRequireUint32RejectsOverflow(t *testing.T) {
	_, err := RequireUint32(numberRequest("gseq", math.Exp2(32)), "gseq")
	if err == nil || !strings.Contains(err.Error(), "parameter gseq") {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestOptionalUint64DistinguishesMissingAndInvalid(t *testing.T) {
	if value, present, err := OptionalUint64(mcp.CallToolRequest{}, "limit"); err != nil || present || value != 0 {
		t.Fatalf("missing value = %d, present=%t, err=%v", value, present, err)
	}

	value, present, err := OptionalUint64(numberRequest("limit", -1), "limit")
	if err == nil || !present || value != 0 {
		t.Fatalf("invalid value = %d, present=%t, err=%v", value, present, err)
	}
}

func TestOptionalUint32DistinguishesMissingInvalidAndValid(t *testing.T) {
	if value, present, err := OptionalUint32(mcp.CallToolRequest{}, "gseq"); err != nil || present || value != 0 {
		t.Fatalf("missing value = %d, present=%t, err=%v", value, present, err)
	}

	value, present, err := OptionalUint32(numberRequest("gseq", math.Exp2(32)), "gseq")
	if err == nil || !present || value != 0 {
		t.Fatalf("overflow value = %d, present=%t, err=%v", value, present, err)
	}

	value, present, err = OptionalUint32(numberRequest("gseq", 7), "gseq")
	if err != nil || !present || value != 7 {
		t.Fatalf("valid value = %d, present=%t, err=%v", value, present, err)
	}
}
