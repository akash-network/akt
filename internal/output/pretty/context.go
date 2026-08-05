package pretty

import (
	"fmt"
	"io"
	"strings"

	"pkg.akt.dev/akt/internal/capability"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// RenderContextShow renders a full context detail view as a string.
// Used by both CLI "akt context show" and TUI context detail views.
//
// effectiveKeyringBackend is the concrete credential store that serves the
// context's configured backend on this host, or "" when the host cannot
// provide it (SPEC §1.5). It is passed in rather than resolved here so the
// renderer stays a pure formatter -- and so this view can never be the reason
// a key store is opened.
func RenderContextShow(rc aktctx.Context, effectiveKeyringBackend string) string {
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

	// An omitted backend means "os", the same normalization the keyring
	// manager applies before opening.
	configuredBackend := rc.Keyring.Backend
	if configuredBackend == "" {
		configuredBackend = aktctx.DefaultKeyring().Backend
	}

	KV(w, "Keyring", fmt.Sprintf("%s (backend: %s)", rc.Keyring.Name, configuredBackend))

	// "os" is an alias for a platform store, so the configured value alone is
	// not an answer to "where are my keys?". Report the concrete store, and
	// say so plainly when this host has none.
	switch {
	case effectiveKeyringBackend == "":
		SubKV(w, "Effective", Dim("unavailable — this host has no "+configuredBackend+" credential store"))
	case effectiveKeyringBackend != configuredBackend:
		SubKV(w, "Effective", effectiveKeyringBackend)
	}

	if rc.DefaultAccount != "" {
		KV(w, "Default Account", rc.DefaultAccount)
	} else {
		KV(w, "Default Account", Dim("(not set)"))
	}

	// Which accounts `akt store sync` reconciles (SPEC §6.7). Empty means the
	// default account alone, which is what the dimmed hint says rather than
	// leaving the reader to guess.
	if len(rc.TrackedAccounts) > 0 {
		KV(w, "Tracked Accounts", strings.Join(rc.TrackedAccounts, ", "))
	} else {
		KV(w, "Tracked Accounts", Dim("(default account)"))
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

	// Feature set: what this configuration can actually do (SPEC §2.10).
	// Missing capabilities name their remedy so the user knows how to
	// enable the corresponding commands.
	set := capability.Resolve(&rc)

	Newline(w)
	KVHeader(w, "  Capabilities")
	renderCapability(w, "Chain queries", set.ChainQuery, "add an RPC endpoint to the network")
	renderCapability(w, "Chain transactions", set.ChainTx, "add an RPC endpoint to the network")
	renderCapability(w, "Provider gateway", set.Provider, "add an RPC endpoint to the network")
	renderCapability(w, "Console API", set.Console, "run akt console login")

	return buf.String()
}

func renderCapability(w io.Writer, label string, available bool, remedy string) {
	if available {
		SubKV(w, label, "available")
		return
	}

	SubKV(w, label, Dim("unavailable — "+remedy))
}
