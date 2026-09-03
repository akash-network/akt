package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"pkg.akt.dev/akt/internal/console"
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
	root         string
	ctxName      string
}

// NewConsoleChainClient wraps a Console API client into the workflow
// steps.ChainClient interface. chainQueries, when non-nil, handles query
// steps directly against the chain; root/ctxName locate the per-context
// manifest cache used to pass the deployment manifest from create to lease.
func NewConsoleChainClient(cc *console.Client, chainQueries steps.ChainClient, root, ctxName string) steps.ChainClient {
	return &consoleChainClient{
		cc:           cc,
		chainQueries: chainQueries,
		root:         root,
		ctxName:      ctxName,
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
// POST /v1/deployments and caches the returned manifest for the subsequent
// lease creation.
func (c *consoleChainClient) createDeployment(ctx context.Context, params map[string]string) (*steps.TxResult, error) {
	sdlStr, err := sdlContent(params["sdl"])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgCreateDeployment, err)
	}

	res, err := c.cc.CreateDeployment(ctx, sdlStr)
	if err != nil {
		return nil, err
	}

	dseq := res.DSeq.String()

	// Cache the manifest only when the API actually returned one (mirroring
	// the CLI twin): caching an empty file would make the later lease
	// creation send an empty manifest instead of failing with the clear
	// no-cached-manifest error.
	if res.Manifest != "" {
		if err := console.SaveManifest(c.root, c.ctxName, dseq, res.Manifest); err != nil {
			return nil, fmt.Errorf("deployment %s was created via Console, but caching its manifest failed (lease creation needs it): %w", dseq, err)
		}
	}

	return consoleTxResult(res.SignTx, map[string]string{
		"dseq":        dseq,
		"rail":        "console",
		"auto_top_up": "daily",
	})
}

// updateDeployment maps deployment.MsgUpdateDeployment to
// PUT /v1/deployments/{dseq}.
func (c *consoleChainClient) updateDeployment(ctx context.Context, params map[string]string) (*steps.TxResult, error) {
	dseq, err := requiredDSeqParam(params, msgUpdateDeployment)
	if err != nil {
		return nil, err
	}

	sdlStr, err := sdlContent(params["sdl"])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgUpdateDeployment, err)
	}

	if _, err := c.cc.UpdateDeployment(ctx, dseq, sdlStr); err != nil {
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
// manifest cached at deployment creation.
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

	manifest, err := console.LoadManifest(c.root, c.ctxName, dseq)
	if err != nil {
		return nil, fmt.Errorf("no cached manifest for deployment %s: the manifest is stored when the deployment is created with this context (e.g. `akt deploy`), and the Console API needs it to create the lease: %w", dseq, err)
	}

	_, err = c.cc.CreateLease(ctx, manifest, []console.LeaseRequest{{
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
