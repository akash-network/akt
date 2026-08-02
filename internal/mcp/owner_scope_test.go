package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	ctypes "pkg.akt.dev/go/node/cert/v1"
	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"

	certtools "pkg.akt.dev/akt/internal/mcp/tools/cert"
	deploymenttools "pkg.akt.dev/akt/internal/mcp/tools/deployment"
	markettools "pkg.akt.dev/akt/internal/mcp/tools/market"
)

// refusingQueryClient fails the test if any module is reached. The whole point
// of the owner guard is that the query never leaves, so a query client that is
// touched at all is the bug.
type refusingQueryClient struct {
	cv1beta3.QueryClient

	t *testing.T
}

func (q *refusingQueryClient) reject(module string) {
	q.t.Helper()
	q.t.Fatalf("%s query was sent with no owner to filter on; an empty filter lists the whole network", module)
}

// Reached only when a guard is missing; t.Fatalf unwinds before the nil
// return is ever used.
func (q *refusingQueryClient) Deployment() dtypes.QueryClient { q.reject("deployment"); return nil }
func (q *refusingQueryClient) Market() mtypes.QueryClient     { q.reject("market"); return nil }
func (q *refusingQueryClient) Certs() ctypes.QueryClient      { q.reject("cert"); return nil }

func (q *refusingQueryClient) ClientContext() sdkclient.Context { return sdkclient.Context{} }

// accountlessClient is a LightClient whose context has no default account --
// the permanent state of a console-api context, and (before the default account
// is resolved to an address) the state of every context on a query path.
type accountlessClient struct {
	q cv1beta3.QueryClient
}

func (c *accountlessClient) Query() cv1beta3.QueryClient      { return c.q }
func (c *accountlessClient) Node() cv1beta3.NodeClient        { return nil }
func (c *accountlessClient) ClientContext() sdkclient.Context { return sdkclient.Context{} }
func (c *accountlessClient) PrintMessage(interface{}) error   { return nil }
func (c *accountlessClient) PrintJSON(interface{}) error      { return nil }

// TestListToolsRefuseWithoutOwner pins the MCP half of the unscoped-list bug.
// These handlers defaulted the owner from the client context and, finding
// nothing there, queried with Owner: "" -- which the query layer reads as "no
// filter", so the tool answered with every matching record on the network as a
// success. An assistant has no help text to catch that on.
func TestListToolsRefuseWithoutOwner(t *testing.T) {
	q := &refusingQueryClient{t: t}
	cl := &accountlessClient{q: q}

	handlers := map[string]func(cv1beta3.LightClient) mcpserver.ToolHandlerFunc{
		"akash_list_deployments":  deploymenttools.HandleListDeployments,
		"akash_list_certificates": certtools.HandleListCertificates,
		"akash_list_orders":       markettools.HandleListOrders,
		"akash_list_bids":         markettools.HandleListBids,
		"akash_list_leases":       markettools.HandleListLeases,
	}

	for name, newHandler := range handlers {
		t.Run(name, func(t *testing.T) {
			res, err := newHandler(cl)(context.Background(), mcp.CallToolRequest{})
			if err != nil {
				t.Fatalf("handler returned a transport error: %v", err)
			}
			if res == nil || !res.IsError {
				t.Fatalf("%s answered without an owner; it must refuse instead", name)
			}

			var text string
			for _, c := range res.Content {
				if tc, ok := c.(mcp.TextContent); ok {
					text += tc.Text
				}
			}

			if !strings.Contains(text, "owner address is required") {
				t.Errorf("unhelpful refusal: %q", text)
			}
		})
	}
}
