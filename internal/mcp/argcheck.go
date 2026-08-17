package mcp

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
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

		arguments := req.GetArguments()
		if err := checkArgs(schema, arguments); err != nil {
			return marshal.ErrResult(err.Error()), nil
		}
		req.Params.Arguments = normalizedOptionalStrings(schema, arguments)

		return next(ctx, req)
	}
}

func checkArgs(schema mcp.ToolInputSchema, args map[string]any) error {
	for _, name := range schema.Required {
		if _, ok := args[name]; !ok {
			return fmt.Errorf("missing required parameter: %s", name)
		}
	}

	unknown := make([]string, 0)
	for name := range args {
		if _, ok := schema.Properties[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("unknown parameter: %s", unknown[0])
	}

	for name, raw := range args {
		prop, ok := schema.Properties[name]
		if !ok {
			continue
		}

		spec, ok := prop.(map[string]any)
		if !ok {
			continue
		}

		if want, _ := spec["type"].(string); want != "" && !jsonTypeMatches(want, raw) {
			return fmt.Errorf("parameter %s must be a %s", name, want)
		}
		if want, _ := spec["type"].(string); want == "string" {
			if value, ok := raw.(string); ok && strings.TrimSpace(value) == "" {
				if !requiresNonBlankString(schema, name, spec) {
					continue
				}
				return fmt.Errorf("parameter %s must not be empty", name)
			}
		}
		if err := checkNumber(name, spec, raw); err != nil {
			return err
		}

		if err := checkEnum(name, spec, raw); err != nil {
			return err
		}
	}

	return nil
}

func normalizedOptionalStrings(schema mcp.ToolInputSchema, args map[string]any) map[string]any {
	normalized := make(map[string]any, len(args))
	for name, value := range args {
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" && !slices.Contains(schema.Required, name) {
			continue
		}
		normalized[name] = value
	}
	return normalized
}

func requiresNonBlankString(schema mcp.ToolInputSchema, name string, spec map[string]any) bool {
	if minimum, ok := spec["minLength"].(float64); ok && minimum > 0 {
		return true
	}
	return slices.Contains(schema.Required, name)
}

func checkNumber(name string, spec map[string]any, raw any) error {
	want, _ := spec["type"].(string)
	if want != "number" && want != "integer" {
		return nil
	}

	value, ok := raw.(float64)
	if !ok {
		return nil
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("parameter %s must be a finite number", name)
	}
	if want == "integer" && value != math.Trunc(value) {
		return fmt.Errorf("parameter %s must be a whole number", name)
	}

	if minimum, ok := spec["minimum"].(float64); ok && value < minimum {
		return fmt.Errorf("parameter %s must be greater than or equal to %g", name, minimum)
	}
	if maximum, ok := spec["maximum"].(float64); ok && value > maximum {
		return fmt.Errorf("parameter %s must be less than or equal to %s", name, strconv.FormatFloat(maximum, 'f', -1, 64))
	}
	if multiple, ok := spec["multipleOf"].(float64); ok && multiple > 0 {
		if multiple == 1 {
			if value != math.Trunc(value) {
				return fmt.Errorf("parameter %s must be a whole number", name)
			}
			return nil
		}
		quotient := value / multiple
		tolerance := 1e-9 * math.Max(1, math.Abs(quotient))
		if math.Abs(quotient-math.Round(quotient)) > tolerance {
			return fmt.Errorf("parameter %s must be a multiple of %g", name, multiple)
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
		// JSON has one decoded numeric type. Range and integral constraints
		// are checked separately against the schema.
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
