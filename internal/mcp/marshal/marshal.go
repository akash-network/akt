package marshal

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToTextResult marshals any value to JSON and wraps it in an MCP CallToolResult.
func ToTextResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(data),
			},
		},
	}, nil
}

// ErrResult returns an MCP error result with the given message.
func ErrResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: msg,
			},
		},
	}
}

// ErrResultf returns an MCP error result with a formatted message.
func ErrResultf(format string, args ...interface{}) *mcp.CallToolResult {
	return ErrResult(fmt.Sprintf(format, args...))
}

// RequireString extracts a required string parameter from the request.
func RequireString(req mcp.CallToolRequest, key string) (string, error) {
	v, ok := req.GetArguments()[key]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string", key)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("parameter %s must not be empty", key)
	}
	return s, nil
}

// OptionalString extracts an optional string parameter from the request.
func OptionalString(req mcp.CallToolRequest, key string) string {
	v, ok := req.GetArguments()[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// OptionalFloat extracts an optional float64 parameter from the request.
func OptionalFloat(req mcp.CallToolRequest, key string) (float64, bool) {
	v, ok := req.GetArguments()[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// PositiveInteger declares a numeric MCP argument as an integer greater than
// zero while retaining compatibility with clients that represent JSON numbers
// as float64.
func PositiveInteger() mcp.PropertyOption {
	return func(schema map[string]any) {
		schema["minimum"] = float64(1)
		schema["maximum"] = maxSafeJSONInteger
		schema["multipleOf"] = float64(1)
	}
}

// NonNegativeInteger declares a numeric MCP argument as an integer that may be
// zero, as required by pagination offsets and default-preserving limits.
func NonNegativeInteger() mcp.PropertyOption {
	return func(schema map[string]any) {
		schema["minimum"] = float64(0)
		schema["maximum"] = maxSafeJSONInteger
		schema["multipleOf"] = float64(1)
	}
}

const maxSafeJSONInteger = float64(1<<53 - 1)

// RequireUint64 extracts a required uint64 parameter (passed as float64 in JSON).
func RequireUint64(req mcp.CallToolRequest, key string) (uint64, error) {
	v, ok := req.GetArguments()[key]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: %s", key)
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("parameter %s must be a number", key)
	}
	return positiveUint(key, f, 64)
}

// OptionalUint64 extracts an optional uint64 parameter.
func OptionalUint64(req mcp.CallToolRequest, key string) (uint64, bool, error) {
	v, ok := req.GetArguments()[key]
	if !ok {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("parameter %s must be a number", key)
	}
	value, err := nonNegativeUint(key, f, 64)
	return value, true, err
}

// RequireUint32 extracts a required uint32 parameter.
func RequireUint32(req mcp.CallToolRequest, key string) (uint32, error) {
	v, ok := req.GetArguments()[key]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: %s", key)
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("parameter %s must be a number", key)
	}
	value, err := positiveUint(key, f, 32)
	return uint32(value), err
}

// OptionalUint32 extracts an optional uint32 parameter.
func OptionalUint32(req mcp.CallToolRequest, key string) (uint32, bool, error) {
	v, ok := req.GetArguments()[key]
	if !ok {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("parameter %s must be a number", key)
	}
	value, err := nonNegativeUint(key, f, 32)
	return uint32(value), true, err
}

func positiveUint(key string, value float64, bits int) (uint64, error) {
	if value <= 0 {
		return 0, fmt.Errorf("parameter %s must be greater than zero", key)
	}
	return nonNegativeUint(key, value, bits)
}

func nonNegativeUint(key string, value float64, bits int) (uint64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("parameter %s must be a finite number", key)
	}
	if value < 0 {
		return 0, fmt.Errorf("parameter %s must be greater than or equal to zero", key)
	}
	if value != math.Trunc(value) {
		return 0, fmt.Errorf("parameter %s must be a whole number", key)
	}
	if value > maxSafeJSONInteger {
		return 0, fmt.Errorf("parameter %s must be less than or equal to %.0f", key, maxSafeJSONInteger)
	}
	if value >= math.Exp2(float64(bits)) {
		return 0, fmt.Errorf("parameter %s is out of range for uint%d", key, bits)
	}
	return uint64(value), nil
}
