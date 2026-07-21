package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	aktprovider "pkg.akt.dev/akt/internal/provider"
	"pkg.akt.dev/akt/internal/workflow/steps"
	mv1 "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	rest "pkg.akt.dev/go/provider/client"
	"pkg.akt.dev/go/sdl"
)

// Default group and order sequence used when the workflow only tracks a
// deployment sequence.
const (
	defaultGSeq uint32 = 1
	defaultOSeq uint32 = 1
)

// providerClient adapts the provider gateway REST client to the workflow
// steps.ProviderClient interface. The gateway client is constructed per call
// because the target provider is supplied per call; its gateway URL is
// resolved from the provider's on-chain record (HostURI).
type providerClient struct {
	cctx     sdkclient.Context
	authType string
}

// NewProviderClient creates a workflow provider client. authType is "jwt"
// (default when empty) or "mtls", matching internal/provider.NewGatewayClient.
func NewProviderClient(cctx sdkclient.Context, authType string) steps.ProviderClient {
	return &providerClient{
		cctx:     cctx,
		authType: authType,
	}
}

// SendManifest parses the SDL into a manifest and submits it to the provider
// for the given deployment, mirroring `akt provider send-manifest`. The sdl
// argument may be either raw SDL content or a path to an SDL file (the
// builtin workflows pass the sdl-file path through the provider step).
func (p *providerClient) SendManifest(ctx context.Context, provider string, dseq uint64, sdlData []byte) error {
	if dseq == 0 {
		return fmt.Errorf("send manifest: dseq is required")
	}

	sdlManifest, err := readSDL(sdlData)
	if err != nil {
		return err
	}

	mani, err := sdlManifest.Manifest()
	if err != nil {
		return fmt.Errorf("build manifest from SDL: %w", err)
	}

	cl, err := p.gatewayClient(ctx, provider)
	if err != nil {
		return err
	}

	if err := cl.SubmitManifest(ctx, dseq, mani); err != nil {
		return fmt.Errorf("submit manifest: %w", err)
	}

	return nil
}

// LeaseStatus queries the live status of a lease from the provider gateway
// and returns it as JSON. The lease is identified by the client's
// from-address as owner and defaults gseq/oseq to 1.
func (p *providerClient) LeaseStatus(ctx context.Context, provider string, dseq uint64) (json.RawMessage, error) {
	if dseq == 0 {
		return nil, fmt.Errorf("lease status: dseq is required")
	}

	cl, err := p.gatewayClient(ctx, provider)
	if err != nil {
		return nil, err
	}

	lid := mv1.LeaseID{
		Owner:    p.cctx.GetFromAddress().String(),
		DSeq:     dseq,
		GSeq:     defaultGSeq,
		OSeq:     defaultOSeq,
		Provider: provider,
	}

	status, err := cl.LeaseStatus(ctx, lid)
	if err != nil {
		return nil, fmt.Errorf("query lease status: %w", err)
	}

	return json.Marshal(status)
}

// gatewayClient constructs a provider gateway REST client for the given
// provider address, resolving the gateway URL from the provider's on-chain
// record. This mirrors gatewayClientFromCmd in internal/cli/provider, except
// the URL lookup is done on chain instead of via a --provider-url flag.
func (p *providerClient) gatewayClient(ctx context.Context, provider string) (rest.Client, error) {
	if _, err := sdk.AccAddressFromBech32(provider); err != nil {
		return nil, fmt.Errorf("invalid provider address %q: %w", provider, err)
	}

	hostURI, err := p.providerHostURI(ctx, provider)
	if err != nil {
		return nil, err
	}

	cl, err := aktprovider.NewGatewayClient(
		ctx,
		p.cctx,
		p.cctx.GetFromAddress(),
		hostURI,
		p.authType,
		p.cctx.Keyring,
	)
	if err != nil {
		return nil, fmt.Errorf("create provider gateway client: %w", err)
	}

	return cl, nil
}

// providerHostURI queries the provider's on-chain record for its gateway URL.
func (p *providerClient) providerHostURI(ctx context.Context, provider string) (string, error) {
	res, err := ptypes.NewQueryClient(p.cctx).Provider(ctx, &ptypes.QueryProviderRequest{Owner: provider})
	if err != nil {
		return "", fmt.Errorf("query provider %s: %w", provider, err)
	}

	if res.Provider.HostURI == "" {
		return "", fmt.Errorf("provider %s has no host URI on chain", provider)
	}

	return res.Provider.HostURI, nil
}

// readSDL interprets data as either a path to an SDL file or raw SDL content.
func readSDL(data []byte) (sdl.SDL, error) {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return nil, fmt.Errorf("empty SDL")
	}

	if info, err := os.Stat(s); err == nil && info.Mode().IsRegular() {
		obj, err := sdl.ReadFile(s)
		if err != nil {
			return nil, fmt.Errorf("read SDL file %q: %w", s, err)
		}

		return obj, nil
	}

	obj, err := sdl.Read(data)
	if err != nil {
		return nil, fmt.Errorf("parse SDL: %w", err)
	}

	return obj, nil
}
