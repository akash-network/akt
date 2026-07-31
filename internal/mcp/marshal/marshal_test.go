package marshal

import (
	"math"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

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
