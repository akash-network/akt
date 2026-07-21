package pretty

import (
	"fmt"
	"strings"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// RenderContextShow renders a full context detail view as a string.
// Used by both CLI "akt context show" and TUI context detail views.
func RenderContextShow(rc aktctx.Context) string {
	var buf strings.Builder
	w := &buf

	fmt.Fprintln(w, Section("Context"))
	KV(w, "Name", Bold(rc.Name))
	KV(w, "Network", rc.Network.Name)

	KVHeader(w, "  Network")
	SubKV(w, "Chain ID", rc.Network.ChainID)

	rpcStr := "(none)"
	if len(rc.Network.Endpoints.RPC) > 0 {
		rpcStr = rc.Network.Endpoints.RPC[0]
		if len(rc.Network.Endpoints.RPC) > 1 {
			rpcStr += fmt.Sprintf(" (+%d backup)", len(rc.Network.Endpoints.RPC)-1)
		}
	}
	SubKV(w, "RPC", rpcStr)

	if len(rc.Network.Endpoints.API) > 0 {
		apiStr := rc.Network.Endpoints.API[0]
		if len(rc.Network.Endpoints.API) > 1 {
			apiStr += fmt.Sprintf(" (+%d backup)", len(rc.Network.Endpoints.API)-1)
		}
		SubKV(w, "API", apiStr)
	}

	if len(rc.Network.Endpoints.GRPC) > 0 {
		SubKV(w, "gRPC", rc.Network.Endpoints.GRPC[0])
	}

	SubKV(w, "Gas Prices", rc.GasPrices)
	SubKV(w, "Gas Adj", rc.GasAdjustment)

	Newline(w)

	authMethod := rc.AuthMethod
	if authMethod == "" {
		authMethod = aktctx.AuthMethodKeyring
	}
	KV(w, "Auth Method", authMethod)

	if authMethod == aktctx.AuthMethodConsoleAPI {
		KVHeader(w, "  Console API")
		SubKV(w, "URL", rc.ConsoleAPIURL)

		// The key itself is never printed (SPEC §7.1).
		if rc.ConsoleAPIKey != "" {
			SubKV(w, "API Key", "configured")
		} else {
			SubKV(w, "API Key", Dim("(not set)"))
		}
	}

	KV(w, "Keyring", fmt.Sprintf("%s (backend: %s)", rc.Keyring.Name, rc.Keyring.Backend))

	if rc.DefaultAccount != "" {
		KV(w, "Default Account", rc.DefaultAccount)
	} else {
		KV(w, "Default Account", Dim("(not set)"))
	}

	KV(w, "Gas", rc.Gas)

	if rc.Fees != "" {
		KV(w, "Fees", rc.Fees)
	} else {
		KV(w, "Fees", Dim("(none)"))
	}

	KV(w, "Provider Auth", rc.AuthType)
	KV(w, "Store", aktctx.StoreDir(rc.Root, rc.Name))
	KV(w, "Action Log", aktctx.ActionLogPath(rc.Root, rc.Name))

	return buf.String()
}
