package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/actionlog"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	"pkg.akt.dev/akt/internal/output"
	"pkg.akt.dev/akt/internal/output/pretty"
)

// faucetState is the ordered, first-match-wins outcome of `akt faucet`: what
// it does depends entirely on the active context's auth method and its
// network's faucet field, not on inline checks scattered through the handler.
type faucetState int

const (
	faucetStateConsoleManaged faucetState = iota
	faucetStateNoNetwork
	faucetStateMainnetNoFaucet
	faucetStateNoFaucetConfigured
	faucetStateReady
)

func classifyFaucetState(rc *aktctx.Context, isMainnet bool) faucetState {
	switch {
	case rc.AuthMethod == aktctx.AuthMethodConsoleAPI:
		return faucetStateConsoleManaged
	case rc.Network.Name == "":
		return faucetStateNoNetwork
	case rc.Network.Faucet == "" && isMainnet:
		return faucetStateMainnetNoFaucet
	case rc.Network.Faucet == "":
		return faucetStateNoFaucetConfigured
	default:
		return faucetStateReady
	}
}

// faucetStateError returns the error for every non-ready state, or nil for
// faucetStateReady.
func faucetStateError(rc *aktctx.Context, state faucetState) error {
	switch state {
	case faucetStateConsoleManaged:
		return fmt.Errorf("the active context %q uses a Console-managed wallet, which has no chain faucet; check the balance with 'akt console wallet balance' or add funds at https://console.akash.network", rc.Name)
	case faucetStateNoNetwork:
		return fmt.Errorf("the active context %q has no network attached, so there is no faucet; attach a test network and try again", rc.Name)
	case faucetStateMainnetNoFaucet:
		return fmt.Errorf("%s (%s) is a live network and has no faucet; acquire real AKT by transfer, or switch to a test network with 'akt context use sandbox'", rc.Network.Name, rc.Network.ChainID)
	case faucetStateNoFaucetConfigured:
		return fmt.Errorf("network %q (%s) has no faucet configured; set one with 'akt context network edit %s --faucet <url>'", rc.Network.Name, rc.Network.ChainID, rc.Network.Name)
	default:
		return nil
	}
}

// isMainnetNetwork reports whether net is the live Akash mainnet: by name
// convention, by its well-known chain-id, or by matching the chain-id the
// active config resolves for its own "mainnet" network entry.
func isMainnetNetwork(net aktctx.Network, mainnetChainID string) bool {
	if strings.Contains(strings.ToLower(net.Name), "mainnet") {
		return true
	}
	if net.ChainID == "akashnet-2" {
		return true
	}
	return mainnetChainID != "" && net.ChainID == mainnetChainID
}

const faucetAddressPlaceholder = "(no default account resolved - paste your own address)"

// faucetOutcome is the result of a faucet interaction, rendered either as a
// JSON/YAML payload or as a table; Status distinguishes a display-only run
// ("") from a submitted request ("requested").
type faucetOutcome struct {
	Network         string `json:"network"                    yaml:"network"`
	ChainID         string `json:"chain_id"                   yaml:"chain_id"`
	Address         string `json:"address"                    yaml:"address"`
	FaucetURL       string `json:"faucet_url"                 yaml:"faucet_url"`
	TransactionHash string `json:"transaction_hash,omitempty" yaml:"transaction_hash,omitempty"`
	Status          string `json:"status,omitempty"           yaml:"status,omitempty"`
}

// recordFaucetAction writes a type=faucet entry for a completed --send
// request. Logging is best-effort and a nil logger is a no-op, exactly like
// internal/provider/actionlog.go RecordAction: the display path is read-only
// and never calls this.
func recordFaucetAction(ctx context.Context, account, txHash string, sendErr error) {
	l := cliutil.ActionLogFromContext(ctx)
	if l == nil {
		return
	}

	entry := actionlog.Entry{
		Type:    actionlog.TypeFaucet,
		Action:  "request",
		Account: account,
		TxHash:  txHash,
		Status:  "success",
	}

	if sendErr != nil {
		entry.Status = "failed"
		entry.Error = sendErr.Error()
	}

	_ = l.Log(entry)
}

// submitFaucetRequest posts the address to the faucet's /faucet endpoint and
// returns the transaction hash the faucet reports.
func submitFaucetRequest(ctx context.Context, client *http.Client, faucetURL, address string) (string, error) {
	endpoint, err := url.Parse(faucetURL)
	if err != nil {
		return "", fmt.Errorf("invalid faucet URL %q: %w", faucetURL, err)
	}
	endpoint.Path = "/faucet"
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	body := strings.NewReader(url.Values{"address": {address}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return "", fmt.Errorf("request to faucet %q failed: %w", faucetURL, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request to faucet %q failed: %w", faucetURL, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("faucet %q returned status %d: %s", faucetURL, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed struct {
		TransactionHash string `json:"transactionHash"`
	}
	_ = json.Unmarshal(respBody, &parsed)

	return parsed.TransactionHash, nil
}

func faucetCmd(mgr func() *aktctx.Manager, mainnetChainIDFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "faucet",
		Short: "Show, or with --send request, test funds from the active network's faucet",
		Args:  cobra.NoArgs,
		Example: `  akt faucet
  akt faucet --send`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()
			if m == nil {
				return fmt.Errorf("no akt configuration found; run akt to bootstrap or akt context create to set one up")
			}

			rc, err := m.Resolve(cliutil.SelectedContextName(cmd, m))
			if err != nil {
				return err
			}

			isMainnet := isMainnetNetwork(rc.Network, mainnetChainIDFn())

			state := classifyFaucetState(rc, isMainnet)
			if stateErr := faucetStateError(rc, state); stateErr != nil {
				return stateErr
			}

			address, _ := chaincli.ResolveDefaultAccountAddress(chaincli.GetClientContextFromCmd(cmd))
			address = strings.TrimSpace(address)

			send, _ := cmd.Flags().GetBool(flagdefs.FlagSend)
			if !send {
				return displayFaucet(cmd, rc, address)
			}

			if isMainnet {
				return fmt.Errorf("refusing to auto-request funds on mainnet %q; --send is for test networks only", rc.Network.Name)
			}

			if address == "" {
				return fmt.Errorf("no default account resolved for context %q; set one with 'akt context edit %s --default-account <name>' or add a key, then retry", rc.Name, rc.Name)
			}

			txHash, err := submitFaucetRequest(cmd.Context(), &http.Client{Timeout: 30 * time.Second}, rc.Network.Faucet, address)
			recordFaucetAction(cmd.Context(), address, txHash, err)
			if err != nil {
				return err
			}

			return displayFaucetRequested(cmd, rc, address, txHash)
		},
	}

	cmd.Flags().Bool(flagdefs.FlagSend, false, "Submit the funds request to the faucet automatically")

	return cmd
}

// displayFaucet renders the pre-submission payload: JSON/YAML fields as-is,
// or a table with the faucet URL and a hint to request funds automatically.
func displayFaucet(cmd *cobra.Command, rc *aktctx.Context, address string) error {
	outcome := faucetOutcome{
		Network:   rc.Network.Name,
		ChainID:   rc.Network.ChainID,
		Address:   address,
		FaucetURL: rc.Network.Faucet,
	}

	if f := output.FormatFromCmd(cmd); f != output.FormatTable {
		return output.Fprint(cmd.OutOrStdout(), f, outcome)
	}

	addressDisplay := address
	if addressDisplay == "" {
		addressDisplay = faucetAddressPlaceholder
	}

	var buf strings.Builder
	pretty.KV(&buf, "Network", fmt.Sprintf("%s (%s)", outcome.Network, outcome.ChainID))
	pretty.KV(&buf, "Address", addressDisplay)
	pretty.KV(&buf, "Faucet", outcome.FaucetURL)
	pretty.Newline(&buf)
	fmt.Fprintln(&buf, "Open the faucet page and paste your address to request test funds. These tokens are for testing only and have no monetary value.")
	fmt.Fprintln(&buf, "Run 'akt faucet --send' to request funds automatically.")

	checked := output.NewCheckedWriter(output.TerminalAwareWriter(cmd.OutOrStdout()))
	_, writeErr := fmt.Fprint(checked, buf.String())
	return checked.Complete(writeErr)
}

// displayFaucetRequested renders the outcome of a submitted faucet request.
func displayFaucetRequested(cmd *cobra.Command, rc *aktctx.Context, address, txHash string) error {
	outcome := faucetOutcome{
		Network:         rc.Network.Name,
		ChainID:         rc.Network.ChainID,
		Address:         address,
		FaucetURL:       rc.Network.Faucet,
		TransactionHash: txHash,
		Status:          "requested",
	}

	if f := output.FormatFromCmd(cmd); f != output.FormatTable {
		return output.Fprint(cmd.OutOrStdout(), f, outcome)
	}

	var buf strings.Builder
	pretty.KV(&buf, "Network", fmt.Sprintf("%s (%s)", outcome.Network, outcome.ChainID))
	pretty.KV(&buf, "Address", outcome.Address)
	pretty.KV(&buf, "Faucet", outcome.FaucetURL)
	if outcome.TransactionHash != "" {
		pretty.KV(&buf, "Tx Hash", outcome.TransactionHash)
	}
	pretty.Newline(&buf)
	fmt.Fprintf(&buf, "Requested test funds for %s. These tokens are for testing only and have no monetary value.\n", outcome.Address)

	checked := output.NewCheckedWriter(output.TerminalAwareWriter(cmd.OutOrStdout()))
	_, writeErr := fmt.Fprint(checked, buf.String())
	return checked.Complete(writeErr)
}
