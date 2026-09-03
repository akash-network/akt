package mcp

import (
	"testing"

	protocol "github.com/mark3labs/mcp-go/mcp"

	certtools "pkg.akt.dev/akt/internal/mcp/tools/cert"
	consoletools "pkg.akt.dev/akt/internal/mcp/tools/console"
	"pkg.akt.dev/akt/internal/mcp/tools/deployment"
	"pkg.akt.dev/akt/internal/mcp/tools/market"
	"pkg.akt.dev/akt/internal/mcp/tools/provider"
)

func TestSequenceSchemasRequirePositiveIntegers(t *testing.T) {
	tools := []protocol.Tool{
		deployment.ToolGetDeployment(),
		deployment.ToolGetGroup(),
		deployment.ToolCloseDeployment(),
		market.ToolGetOrder(),
		market.ToolListBids(),
		market.ToolGetBid(),
		market.ToolListLeases(),
		market.ToolGetLease(),
		market.ToolCreateLease(),
		market.ToolCloseLease(),
		provider.ToolLeaseStatus(),
		provider.ToolServiceStatus(),
		provider.ToolSubmitManifest(),
	}

	checked := 0
	for _, tool := range tools {
		for _, name := range []string{"dseq", "gseq", "oseq"} {
			property, ok := tool.InputSchema.Properties[name]
			if !ok {
				continue
			}
			checked++
			assertIntegerBounds(t, tool.Name, name, property, 1)
		}
	}

	if checked != 28 {
		t.Fatalf("checked %d sequence properties, want 28", checked)
	}
}

func TestStateSchemasDeclareAcceptedEnums(t *testing.T) {
	tests := []struct {
		tool    protocol.Tool
		allowed []string
	}{
		{tool: deployment.ToolListDeployments(), allowed: []string{"active", "closed"}},
		{tool: market.ToolListOrders(), allowed: []string{"open", "active", "closed"}},
		{tool: market.ToolListBids(), allowed: []string{"open", "active", "lost", "closed"}},
		{tool: market.ToolListLeases(), allowed: []string{"active", "closed", "insufficient_funds"}},
		{tool: certtools.ToolListCertificates(), allowed: []string{"valid", "revoked"}},
	}

	for _, tc := range tests {
		t.Run(tc.tool.Name, func(t *testing.T) {
			property, ok := tc.tool.InputSchema.Properties["state"].(map[string]any)
			if !ok {
				t.Fatalf("state schema = %#v", tc.tool.InputSchema.Properties["state"])
			}
			allowed, ok := property["enum"].([]string)
			if !ok {
				t.Fatalf("state enum = %#v, want %v", property["enum"], tc.allowed)
			}
			if len(allowed) != len(tc.allowed) {
				t.Fatalf("state enum = %v, want %v", allowed, tc.allowed)
			}
			for i := range tc.allowed {
				if allowed[i] != tc.allowed[i] {
					t.Fatalf("state enum = %v, want %v", allowed, tc.allowed)
				}
			}
		})
	}
}

func TestPaginationSchemasRequireNonNegativeIntegers(t *testing.T) {
	tools := []protocol.Tool{
		consoletools.ToolListDeployments(),
		consoletools.ToolListProviders(),
	}

	checked := 0
	for _, tool := range tools {
		for _, name := range []string{"skip", "limit"} {
			property, ok := tool.InputSchema.Properties[name]
			if !ok {
				continue
			}
			checked++
			assertIntegerBounds(t, tool.Name, name, property, 0)
		}
	}

	if checked != 3 {
		t.Fatalf("checked %d pagination properties, want 3", checked)
	}
}

func assertIntegerBounds(t *testing.T, toolName, propertyName string, property any, minimum float64) {
	t.Helper()
	schema, ok := property.(map[string]any)
	if !ok {
		t.Fatalf("%s.%s schema = %#v", toolName, propertyName, property)
	}
	if got, ok := schema["minimum"].(float64); !ok || got != minimum {
		t.Errorf("%s.%s minimum = %#v, want %v", toolName, propertyName, schema["minimum"], minimum)
	}
	if got, ok := schema["multipleOf"].(float64); !ok || got != 1 {
		t.Errorf("%s.%s multipleOf = %#v, want 1", toolName, propertyName, schema["multipleOf"])
	}
	if got, ok := schema["maximum"].(float64); !ok || got != 9007199254740991 {
		t.Errorf("%s.%s maximum = %#v, want JSON safe-integer ceiling", toolName, propertyName, schema["maximum"])
	}
}
