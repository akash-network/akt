package bootstrap

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"pkg.akt.dev/akt/internal/console"
)

// consoleOnboarding optionally collects an Akash Console API key during the
// first-run wizard (SPEC §7.1). Returns the key ("" = skipped) and whether
// deployments for the initial context should be routed through Console
// (auth-method: console-api). Non-interactive runs skip the step entirely.
func consoleOnboarding(ctxName string) (key string, routeDeployments bool) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", false
	}

	// Prompt and progress rendering is stderr-only (SPEC §3.9.2, §10.1.1).
	out := os.Stderr

	fmt.Fprintln(out, ansiBold+"Akash Console (optional)"+ansiReset)
	fmt.Fprintln(out, ansiDim+"A Console API key enables managed deployments, USD billing, and the"+ansiReset)
	fmt.Fprintln(out, ansiDim+"akt console commands. Keys: console.akash.network > Settings > API Keys."+ansiReset)
	fmt.Fprintln(out)

	if !promptYesNo(fmt.Sprintf("Configure a Console API key for context %q?", ctxName), false) {
		fmt.Fprintln(out, ansiDim+"Skipped. You can add one later with: akt console login"+ansiReset)
		fmt.Fprintln(out)

		return "", false
	}

	key = promptSecret("Console API key: ")
	if key == "" {
		fmt.Fprintln(out, ansiYellow+"No key entered; skipping Console setup."+ansiReset)
		fmt.Fprintln(out)

		return "", false
	}

	validateConsoleKey(key)

	routeDeployments = promptYesNo("Route deployments through Console for this context (managed wallet, USD)?", false)
	fmt.Fprintln(out)

	return key, routeDeployments
}

// validateConsoleKey checks the key against the Console API. Validation is
// best-effort: first-run may be offline, so a failure only warns.
func validateConsoleKey(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := console.New("", key).GetUser(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, ansiYellow+"Could not validate the key (%v); storing anyway. Verify later with: akt console whoami"+ansiReset+"\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, ansiGreen+"Authenticated as %s"+ansiReset+"\n", user.Username)
}

// promptYesNo asks a yes/no question on stdin. Empty input selects def.
func promptYesNo(question string, def bool) bool {
	suffix := " [y/N]: "
	if def {
		suffix = " [Y/n]: "
	}

	fmt.Fprint(os.Stderr, question+suffix)

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return def
	}

	return parseYesNo(line, def)
}

// parseYesNo interprets a yes/no answer, falling back to def on empty or
// unrecognized input.
func parseYesNo(input string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

// promptSecret reads a line without echoing it to the terminal.
func promptSecret(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)

	data, err := term.ReadPassword(int(os.Stdin.Fd()))

	fmt.Fprintln(os.Stderr)

	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}
