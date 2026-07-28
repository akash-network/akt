// Package capability derives a feature set from the active context's
// configuration: which transports are usable (chain RPC, Console API) and
// therefore which command groups can work. The CLI uses it to gate the
// command surface so users are never offered commands their configuration
// cannot execute — e.g. a context with only a Console API key and no RPC
// endpoint cannot run chain queries.
//
// Two presentation modes are supported while UX feedback is collected
// (config: defaults.command-gating):
//
//   - "dim" (default): unavailable commands stay listed, marked
//     "[unavailable: ...]" in help, and fail fast with an explanation.
//   - "hide": unavailable commands are removed from help listings entirely
//     (still fail fast with the explanation if invoked directly).
//   - "off": no gating; commands fail wherever the missing transport is
//     first touched (the pre-gating behavior).
package capability

import (
	"fmt"
	"strings"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// Capability names a single thing a context configuration can do.
type Capability string

const (
	// ChainQuery requires a network RPC endpoint.
	ChainQuery Capability = "chain-query"
	// ChainTx requires a network RPC endpoint (transactions additionally
	// need a funded key at execution time; key presence is deliberately
	// not probed here because opening OS keyrings can prompt).
	ChainTx Capability = "chain-tx"
	// Console requires a resolvable Console API key.
	Console Capability = "console"
	// Provider requires chain access (gateway discovery + wallet auth).
	Provider Capability = "provider"
)

// Set is the feature set resolved from a context.
type Set struct {
	ChainQuery bool
	ChainTx    bool
	Console    bool
	Provider   bool
}

// Resolve derives the feature set from a resolved context. A nil context
// (no active context) yields the empty set.
func Resolve(rc *aktctx.Context) Set {
	if rc == nil {
		return Set{}
	}

	hasRPC := len(rc.Network.Endpoints.RPC) > 0
	hasKey := rc.ConsoleAPIKey != ""

	return Set{
		ChainQuery: hasRPC,
		ChainTx:    hasRPC,
		Console:    hasKey,
		Provider:   hasRPC,
	}
}

// Has reports whether the set satisfies a single capability.
func (s Set) Has(c Capability) bool {
	switch c {
	case ChainQuery:
		return s.ChainQuery
	case ChainTx:
		return s.ChainTx
	case Console:
		return s.Console
	case Provider:
		return s.Provider
	default:
		// Unknown requirements never gate: fail open so a typo in an
		// annotation cannot brick a command.
		return true
	}
}

// AnnotationKey is the cobra annotation carrying a command's requirement.
// The value is a requirement expression: capabilities separated by "|" are
// alternatives (any one suffices), e.g. "chain-tx|console" for workflow
// commands that run on either rail.
const AnnotationKey = "akt.requires"

// Satisfies evaluates a requirement expression against the set.
func (s Set) Satisfies(requirement string) bool {
	requirement = strings.TrimSpace(requirement)
	if requirement == "" {
		return true
	}

	for _, alt := range strings.Split(requirement, "|") {
		if s.Has(Capability(strings.TrimSpace(alt))) {
			return true
		}
	}

	return false
}

// remedies maps a capability to how the user enables it.
var remedies = map[Capability]string{
	ChainQuery: "add an RPC endpoint to the context's network (akt context network edit <network> --rpc <url>)",
	ChainTx:    "add an RPC endpoint to the context's network (akt context network edit <network> --rpc <url>)",
	Console:    "configure a Console API key (akt console login, or akt context edit <context> --console-api-key <key>)",
	Provider:   "add an RPC endpoint to the context's network (akt context network edit <network> --rpc <url>)",
}

// Explain describes an unsatisfied requirement and how to fix it. The
// returned string is suitable both for help-line dimming and for the error
// returned when the command is invoked anyway.
func (s Set) Explain(requirement string) string {
	var wants []string
	for _, alt := range strings.Split(requirement, "|") {
		c := Capability(strings.TrimSpace(alt))
		remedy, known := remedies[c]
		if !known {
			continue
		}
		wants = append(wants, fmt.Sprintf("%s (%s)", c, remedy))
	}

	if len(wants) == 0 {
		return fmt.Sprintf("requires %q", requirement)
	}

	return "requires " + strings.Join(wants, ", or ")
}

// Mode is the gating presentation mode from config.
type Mode string

const (
	ModeDim  Mode = "dim"
	ModeHide Mode = "hide"
	ModeOff  Mode = "off"
)

// ParseMode normalizes a configured gating mode; unknown values fall back
// to the default (dim) so a config typo never disables the safety net.
func ParseMode(s string) Mode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(ModeHide):
		return ModeHide
	case string(ModeOff):
		return ModeOff
	default:
		return ModeDim
	}
}
