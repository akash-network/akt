package marshal

import (
	"encoding/json"
	"fmt"

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
	return uint64(f), nil
}

// OptionalUint64 extracts an optional uint64 parameter.
func OptionalUint64(req mcp.CallToolRequest, key string) (uint64, bool) {
	v, ok := req.GetArguments()[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return uint64(f), true
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
	return uint32(f), nil
}

// OptionalUint32 extracts an optional uint32 parameter.
func OptionalUint32(req mcp.CallToolRequest, key string) (uint32, bool) {
	v, ok := req.GetArguments()[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return uint32(f), true
}
