package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"pkg.akt.dev/akt/internal/mcp/marshal"
)

// validateArguments rejects a call whose arguments contradict the tool's own
// input schema.
//
// Required parameters were type-checked by hand in each handler, but optional
// ones were read with helpers that discard anything of the wrong type -- so
// `{"limit": "fifty"}` or `{"limit": -5}` silently fell back to the default and
// the caller got a different page size than it asked for, reported as success.
// Checking against the declared schema covers every tool at once, including
// the enum values, instead of relying on each handler to remember.
func (s *Server) validateArguments(next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		schema, ok := s.schemas[req.Params.Name]
		if !ok {
			return next(ctx, req)
		}

		if err := checkArgs(schema, req.GetArguments()); err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		return next(ctx, req)
	}
}

func checkArgs(schema mcp.ToolInputSchema, args map[string]any) error {
	for name, raw := range args {
		prop, ok := schema.Properties[name]
		if !ok {
			// Unknown arguments stay ignored: a client that sends an extra
			// field should not have the call fail.
			continue
		}

		spec, ok := prop.(map[string]any)
		if !ok {
			continue
		}

		if want, _ := spec["type"].(string); want != "" && !jsonTypeMatches(want, raw) {
			return fmt.Errorf("parameter %s must be a %s", name, want)
		}

		if err := checkEnum(name, spec, raw); err != nil {
			return err
		}
	}

	return nil
}

// checkEnum rejects a value outside the declared set, so a bad one fails here
// instead of as a 400 from the far end.
func checkEnum(name string, spec map[string]any, raw any) error {
	// The schema is read as a Go value rather than as decoded JSON, so the
	// enum arrives as []string when the tool was built with mcp.Enum and as
	// []any only if it came back through a JSON round trip.
	var names []string

	switch allowed := spec["enum"].(type) {
	case []string:
		names = allowed
	case []any:
		for _, a := range allowed {
			names = append(names, fmt.Sprintf("%v", a))
		}
	default:
		return nil
	}

	if len(names) == 0 {
		return nil
	}

	if slices.ContainsFunc(names, func(n string) bool { return n == fmt.Sprintf("%v", raw) }) {
		return nil
	}

	return fmt.Errorf("parameter %s must be one of %s", name, strings.Join(names, ", "))
}

func jsonTypeMatches(want string, v any) bool {
	switch want {
	case "string":
		_, ok := v.(string)

		return ok
	case "number", "integer":
		// JSON has one numeric type; negatives are caught by each handler's
		// own range check, not here.
		_, ok := v.(float64)

		return ok
	case "boolean":
		_, ok := v.(bool)

		return ok
	case "array":
		_, ok := v.([]any)

		return ok
	case "object":
		_, ok := v.(map[string]any)

		return ok
	default:
		return true
	}
}
