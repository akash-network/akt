package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/akt/internal/deploymentconfig"
	"pkg.akt.dev/akt/internal/workflow/steps"
)

// consoleChainClient adapts the Console API client to the workflow
// steps.ChainClient interface, routing tx steps through the Console API per
// SPEC §7.4/§7.5. Queries are delegated to a real chain client when one is
// available (console deployments live on chain, so chain queries work
// unchanged); without one, market.bids falls back to the Console bids
// endpoint.
type consoleChainClient struct {
	cc           *console.Client
	chainQueries steps.ChainClient
	inputs       *DeploymentInputs
}

// NewConsoleChainClient wraps a Console API client into the workflow
// steps.ChainClient interface. chainQueries, when non-nil, handles query
// steps directly against the chain. Inputs keep secret values outside workflow
// state; Console owns saved definitions and provider manifests.
func NewConsoleChainClient(cc *console.Client, chainQueries steps.ChainClient, root, ctxName string, inputs ...*DeploymentInputs) steps.ChainClient {
	return &consoleChainClient{
		cc:           cc,
		chainQueries: chainQueries,
		inputs:       selectDeploymentInputs(inputs, root, ctxName),
	}
}

// BroadcastTx routes the workflow tx message to the matching Console API
// endpoint (SPEC §7.5). Message types without a Console mapping produce an
// unsupported-command error directing the user to the chain workflow rail.
func (c *consoleChainClient) BroadcastTx(ctx context.Context, msgType string, params map[string]string) (*steps.TxResult, error) {
	switch msgType {
	case msgCreateDeployment:
		return c.createDeployment(ctx, params)
	case msgUpdateDeployment:
		return c.updateDeployment(ctx, params)
	case msgCloseDeployment:
		return c.closeDeployment(ctx, params)
	case msgCreateLease:
		return c.createLease(ctx, params)
	default:
		return nil, fmt.Errorf("command %q is not supported on the Console workflow rail; set the context's preferred workflow rail with --deploy-via chain", msgType)
	}
}

// Query delegates to the chain query client when available. Without chain
// access, market.bids is served from the Console bids endpoint (shaped like
// the chain response, i.e. a top-level "bids" array); other query paths
// require chain access.
func (c *consoleChainClient) Query(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	if c.chainQueries != nil {
		return c.chainQueries.Query(ctx, path, params)
	}

	switch path {
	case queryMarketBids:
		dseq := params["dseq"]
		if dseq == "" {
			return nil, fmt.Errorf("query %s via Console requires a %q filter", path, "dseq")
		}

		bids, err := c.cc.FetchBids(ctx, dseq)
		if err != nil {
			return nil, fmt.Errorf("fetch bids for dseq %s: %w", dseq, err)
		}

		// Mirror the chain query shape {"bids":[{"bid":{...}}]} so wait
		// conditions like {{ ge (len .Result.bids) 1 }} and the prompt
		// step's bid parsing work identically for both auth methods.
		wrapped := make([]map[string]console.Bid, 0, len(bids))
		for _, b := range bids {
			wrapped = append(wrapped, map[string]console.Bid{"bid": b})
		}

		return json.Marshal(map[string]any{
			"bids":              wrapped,
			"provider_metadata": fetchConsoleProviderMetadata(ctx, c.cc, bids),
		})

	default:
		return nil, fmt.Errorf("query %q is not supported on the Console workflow rail without chain access; add a network with an RPC endpoint to the context", path)
	}
}

// createDeployment maps deployment.MsgCreateDeployment to
// POST /v1/deployments, inheriting the source definition and secrets for redeploy.
func (c *consoleChainClient) createDeployment(ctx context.Context, params map[string]string) (*steps.TxResult, error) {
	sdlInput := params["sdl"]
	source := params["source-dseq"]
	if source != "" && sdlInput == "" {
		definition, err := c.cc.GetDeploymentDefinition(ctx, source)
		if err != nil {
			return nil, fmt.Errorf("read source deployment definition: %w", err)
		}
		sdlInput = definition.SDL
	}
	sdlStr, err := sdlContent(sdlInput)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgCreateDeployment, err)
	}

	secrets, err := console.ReadSecretValuesFile(params["secrets-file"], c.inputs.input)
	if err != nil {
		return nil, err
	}
	res, err := c.cc.CreateDeployment(ctx, sdlStr, console.CreateDeploymentOptions{
		Secrets: secrets, InheritSecretsFrom: source,
	})
	if err != nil {
		return nil, err
	}

	dseq := res.DSeq.String()

	return consoleTxResult(res.SignTx, map[string]string{
		"dseq":        dseq,
		"rail":        "console",
		"auto_top_up": "daily",
	})
}

// updateDeployment maps deployment.MsgUpdateDeployment to
// a service PATCH, or the legacy full-SDL path when no saved definition exists.
func (c *consoleChainClient) updateDeployment(ctx context.Context, params map[string]string) (*steps.TxResult, error) {
	dseq, err := requiredDSeqParam(params, msgUpdateDeployment)
	if err != nil {
		return nil, err
	}

	sdlStr, err := sdlContent(params["sdl"])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgUpdateDeployment, err)
	}

	secrets, err := console.ReadSecretValuesFile(params["secrets-file"], c.inputs.input)
	if err != nil {
		return nil, err
	}
	if params["patch"] == "true" {
		patch, err := deploymentconfig.ParsePatch([]byte(sdlStr))
		if err != nil {
			return nil, err
		}
		_, err = c.cc.PatchDeployment(ctx, dseq, patch, secrets)
		if err != nil {
			return nil, err
		}
	} else if _, err := c.cc.UpdateDeployment(ctx, dseq, sdlStr, console.UpdateDeploymentOptions{Secrets: secrets}); err != nil {
		return nil, err
	}

	return consoleTxResult(nil, map[string]string{"dseq": dseq})
}

// closeDeployment maps deployment.MsgCloseDeployment to
// DELETE /v1/deployments/{dseq}.
func (c *consoleChainClient) closeDeployment(ctx context.Context, params map[string]string) (*steps.TxResult, error) {
	dseq, err := requiredDSeqParam(params, msgCloseDeployment)
	if err != nil {
		return nil, err
	}

	if err := c.cc.CloseDeployment(ctx, dseq); err != nil {
		return nil, err
	}

	return consoleTxResult(nil, map[string]string{"dseq": dseq})
}

// createLease maps market.MsgCreateLease to POST /v1/leases, sending the
// server-derived manifest from the saved deployment definition.
func (c *consoleChainClient) createLease(ctx context.Context, params map[string]string) (*steps.TxResult, error) {
	dseq, err := requiredDSeqParam(params, msgCreateLease)
	if err != nil {
		return nil, err
	}

	provider := params["provider"]
	if provider == "" {
		return nil, fmt.Errorf("%s: required param %q missing", msgCreateLease, "provider")
	}

	gseq, err := uint32ParamWithDefault(params, "gseq", 1)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgCreateLease, err)
	}

	oseq, err := uint32ParamWithDefault(params, "oseq", 1)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgCreateLease, err)
	}

	_, err = c.cc.CreateLease(ctx, "", []console.LeaseRequest{{
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider,
	}})
	if err != nil {
		return nil, err
	}

	return consoleTxResult(nil, map[string]string{
		"dseq":     dseq,
		"gseq":     strconv.FormatUint(uint64(gseq), 10),
		"oseq":     strconv.FormatUint(uint64(oseq), 10),
		"provider": provider,
	})
}

// consoleTxResult builds a steps.TxResult from an optional Console SignTx
// broadcast report and a flat data payload (always carrying "dseq" as a JSON
// string, matching the keyring chain adapter). A non-zero SignTx code means
// the managed-wallet broadcast failed on chain even though the Console API
// answered 200; that is a step failure (mirroring the keyring chain
// adapter's TxResponse.Code check), not a success to wait on.
func consoleTxResult(signTx *console.SignTx, data map[string]string) (*steps.TxResult, error) {
	if signTx != nil && signTx.Code != 0 {
		return nil, fmt.Errorf("tx %s failed with code %d: %s", signTx.TransactionHash, signTx.Code, signTx.RawLog)
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal tx result data: %w", err)
	}

	res := &steps.TxResult{Data: raw}

	if signTx != nil {
		res.TxHash = signTx.TransactionHash
		res.Code = uint32(signTx.Code) // nolint: gosec
	}

	return res, nil
}

// requiredDSeqParam returns the "dseq" param, validated as an unsigned
// integer but kept as a string (the Console API uses string dseqs).
func requiredDSeqParam(params map[string]string, msgType string) (string, error) {
	if _, err := requiredUint64Param(params, "dseq"); err != nil {
		return "", fmt.Errorf("%s: %w", msgType, err)
	}

	return params["dseq"], nil
}

// sdlContent interprets the workflow "sdl" param as either a path to an SDL
// file or raw SDL content (mirroring readSDL), returning the SDL text the
// Console API expects. A value that looks like a path but does not exist is
// an error — the provider twin fails on such input too (its SDL parse
// rejects a bare path), and silently POSTing a typo'd filename as "SDL
// content" would create a garbage deployment on the managed wallet.
func sdlContent(param string) (string, error) {
	s := strings.TrimSpace(param)
	if s == "" {
		return "", fmt.Errorf("required param %q missing", "sdl")
	}

	if info, err := os.Stat(s); err == nil && info.Mode().IsRegular() {
		data, err := os.ReadFile(s)
		if err != nil {
			return "", fmt.Errorf("read SDL file %q: %w", s, err)
		}

		return string(data), nil
	}

	if looksLikeSDLPath(s) {
		return "", fmt.Errorf("SDL file %q does not exist", s)
	}

	return param, nil
}

// looksLikeSDLPath reports whether s is plausibly a file path rather than
// raw SDL content: a single line ending in .yaml/.yml or containing a path
// separator. Raw SDL is multi-line YAML, so it never matches.
func looksLikeSDLPath(s string) bool {
	if strings.ContainsRune(s, '\n') {
		return false
	}

	if strings.HasSuffix(s, ".yaml") || strings.HasSuffix(s, ".yml") {
		return true
	}

	return strings.ContainsRune(s, '/') || strings.ContainsRune(s, os.PathSeparator)
}
