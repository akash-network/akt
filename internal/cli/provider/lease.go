package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/spf13/cobra"

	mtypes "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

// leasePageLimit bounds one page of the lease query used for provider
// resolution. A deployment has one lease per group per provider, so a single
// page covers every realistic deployment; pagination is still followed so a
// pathological one cannot silently lose a lease.
const leasePageLimit = 100

// leaseQuery returns the leases owned by owner for one deployment, in every
// state. It is a function type so provider resolution can be exercised without
// a chain.
type leaseQuery func(ctx context.Context, owner string, dseq uint64) ([]mtypes.Lease, error)

// hostURIQuery resolves a provider address to its on-chain gateway host URI.
type hostURIQuery func(ctx context.Context, provider string) (string, error)

// leaseScope is the part of a lease identity that resolves locally: the owner
// comes from the selected context's default account, and the sequence numbers
// from the positional [dseq] argument and the --gseq/--oseq flags. The provider
// is deliberately absent — it is resolved afterwards, either from --provider or
// from the deployment's active lease (SPEC §2.4).
type leaseScope struct {
	Owner string
	DSeq  uint64
	GSeq  uint32
	OSeq  uint32

	// GSeqSet and OSeqSet record whether the user named the group and order
	// sequences. When they did not, an auto-resolved lease supplies its own.
	GSeqSet bool
	OSeqSet bool
}

// leaseID builds the gateway LeaseID for this scope under the given provider.
func (s leaseScope) leaseID(provider string) mtypes.LeaseID {
	return mtypes.LeaseID{
		Owner:    s.Owner,
		DSeq:     s.DSeq,
		GSeq:     s.GSeq,
		OSeq:     s.OSeq,
		Provider: provider,
	}
}

// leaseScopeFromCmd resolves the deployment sequence and the owner for a
// lease-scoped command. The deployment sequence is resolved before anything
// touching the provider, because it is the value the provider is resolved from:
// a command missing both must report the missing sequence, not the missing
// provider.
//
// Only lease-shell, send-manifest, and the migrate-* commands still register
// --dseq (FEEDBACK 2026-07: the flag is disabled on the positional-[dseq]
// commands for the positional-only UX trial); elsewhere the flag lookup below
// yields zero and the positional argument is the sole source.
func leaseScopeFromCmd(cmd *cobra.Command, args []string) (leaseScope, error) {
	cctx := sdkclient.GetClientContextFromCmd(cmd)

	dseq, _ := cmd.Flags().GetUint64("dseq")

	dseq, err := cflags.DSeqFromArgs(args, dseq)
	if err != nil {
		return leaseScope{}, err
	}

	if dseq == 0 {
		return leaseScope{}, missingDSeqError(cmd)
	}

	gseq, _ := cmd.Flags().GetUint32("gseq")
	oseq, _ := cmd.Flags().GetUint32("oseq")

	return leaseScope{
		Owner:   cctx.GetFromAddress().String(),
		DSeq:    dseq,
		GSeq:    gseq,
		OSeq:    oseq,
		GSeqSet: cmd.Flags().Changed("gseq"),
		OSeqSet: cmd.Flags().Changed("oseq"),
	}, nil
}

// missingDSeqError names the way this particular command actually accepts a
// deployment sequence. Commands whose positional slot is the dseq must not be
// told to pass a flag they do not register, and vice versa.
func missingDSeqError(cmd *cobra.Command) error {
	if cmd.Flags().Lookup("dseq") != nil {
		return fmt.Errorf("dseq is required: pass --dseq <dseq> (e.g. %s --dseq 12345)", cmd.CommandPath())
	}

	return fmt.Errorf("dseq is required: pass it as the positional argument (e.g. %s 12345)", cmd.CommandPath())
}

// resolveAuthenticatedLease resolves the complete lease identity and the
// provider gateway URL for a lease-scoped command.
//
// The order is deliberate and differs from provider `status`:
//
//  1. the deployment sequence, because the provider is resolved from it;
//  2. the local signing identity, so a broken context fails before any network
//     work (JWT signing and mTLS both need it);
//  3. the provider — explicitly from --provider, otherwise from the
//     deployment's single active lease on chain;
//  4. the gateway URL, from --provider-url or the provider's on-chain host URI.
func resolveAuthenticatedLease(cmd *cobra.Command, args []string) (mtypes.LeaseID, string, error) {
	return resolveLeaseWith(cmd, args, chainLeaseQuery(cmd), providerHostURILookup(cmd), func() error {
		_, _, _, err := gatewayAuthenticationFromCmd(cmd)
		return err
	})
}

func resolveLeaseWith(
	cmd *cobra.Command,
	args []string,
	leases leaseQuery,
	hostURI hostURIQuery,
	preflight func() error,
) (mtypes.LeaseID, string, error) {
	scope, err := leaseScopeFromCmd(cmd, args)
	if err != nil {
		return mtypes.LeaseID{}, "", err
	}

	if preflight != nil {
		if err := preflight(); err != nil {
			return mtypes.LeaseID{}, "", err
		}
	}

	lid, err := resolveLeaseProvider(cmd, scope, leases)
	if err != nil {
		return mtypes.LeaseID{}, "", err
	}

	providerURL, err := gatewayURL(cmd, lid.Provider, hostURI)
	if err != nil {
		return mtypes.LeaseID{}, "", err
	}

	return lid, providerURL, nil
}

// resolveLeaseProvider completes the lease identity with a provider address.
// An explicit --provider always wins; otherwise the deployment's single active
// lease supplies the provider and, unless the user named them, its own
// gseq/oseq.
func resolveLeaseProvider(cmd *cobra.Command, scope leaseScope, leases leaseQuery) (mtypes.LeaseID, error) {
	if addrStr, _ := cmd.Flags().GetString("provider"); addrStr != "" {
		addr, err := sdk.AccAddressFromBech32(addrStr)
		if err != nil {
			return mtypes.LeaseID{}, fmt.Errorf("invalid provider address %q: %w", addrStr, err)
		}

		return scope.leaseID(addr.String()), nil
	}

	lease, err := activeLeaseForScope(cmd.Context(), scope, leases)
	if err != nil {
		return mtypes.LeaseID{}, err
	}

	lid := scope.leaseID(lease.ID.Provider)
	if !scope.GSeqSet {
		lid.GSeq = lease.ID.GSeq
	}
	if !scope.OSeqSet {
		lid.OSeq = lease.ID.OSeq
	}

	return lid, nil
}

// activeLeaseForScope picks the deployment's single active lease. Ambiguous and
// impossible resolutions are refused with an error naming the real cause, the
// way the Console rail does in internal/cli/console/gateway.go — never guessed.
func activeLeaseForScope(ctx context.Context, scope leaseScope, leases leaseQuery) (mtypes.Lease, error) {
	if scope.Owner == "" {
		return mtypes.Lease{}, fmt.Errorf(
			"cannot resolve the provider for deployment %d: the context has no configured default account; "+
				"set one with `akt context edit <context> --default-account <key>`, or name the provider with --provider",
			scope.DSeq)
	}

	found, err := leases(ctx, scope.Owner, scope.DSeq)
	if err != nil {
		return mtypes.Lease{}, err
	}

	found = leasesMatchingScope(scope, found)
	if len(found) == 0 {
		return mtypes.Lease{}, fmt.Errorf("%s has no leases: %s", deploymentLabel(scope), leaseRemedy(scope))
	}

	var (
		active []mtypes.Lease
		states []string
	)
	for _, lease := range found {
		if lease.State == mtypes.LeaseActive {
			active = append(active, lease)
			continue
		}
		states = append(states, lease.State.String())
	}

	switch len(active) {
	case 1:
		return active[0], nil

	case 0:
		sort.Strings(states)
		return mtypes.Lease{}, fmt.Errorf("%s has no active lease (lease states: %s): %s",
			deploymentLabel(scope), strings.Join(states, ", "), leaseRemedy(scope))

	default:
		providers := make([]string, 0, len(active))
		for _, lease := range active {
			providers = append(providers, "  "+lease.ID.Provider)
		}
		sort.Strings(providers)

		return mtypes.Lease{}, fmt.Errorf(
			"%s has %d active leases; choose one with --provider:\n%s",
			deploymentLabel(scope), len(active), strings.Join(providers, "\n"))
	}
}

// leasesMatchingScope narrows the deployment's leases to the group and order
// the user named. Sequences the user did not name are left unfiltered so a
// lease on a re-ordered group (oseq > 1) is still found.
func leasesMatchingScope(scope leaseScope, leases []mtypes.Lease) []mtypes.Lease {
	if !scope.GSeqSet && !scope.OSeqSet {
		return leases
	}

	kept := make([]mtypes.Lease, 0, len(leases))
	for _, lease := range leases {
		if scope.GSeqSet && lease.ID.GSeq != scope.GSeq {
			continue
		}
		if scope.OSeqSet && lease.ID.OSeq != scope.OSeq {
			continue
		}
		kept = append(kept, lease)
	}

	return kept
}

// deploymentLabel identifies the deployment the way the user addressed it,
// including the group and order sequences only when they narrowed the search.
func deploymentLabel(scope leaseScope) string {
	label := fmt.Sprintf("deployment %d", scope.DSeq)
	switch {
	case scope.GSeqSet && scope.OSeqSet:
		return fmt.Sprintf("%s (gseq %d, oseq %d)", label, scope.GSeq, scope.OSeq)
	case scope.GSeqSet:
		return fmt.Sprintf("%s (gseq %d)", label, scope.GSeq)
	case scope.OSeqSet:
		return fmt.Sprintf("%s (oseq %d)", label, scope.OSeq)
	}

	return label
}

// leaseRemedy points at a command that actually exists and shows the leases the
// resolution looked at.
func leaseRemedy(scope leaseScope) string {
	return fmt.Sprintf("inspect it with `akt query market lease %d`, or name the gateway with --provider", scope.DSeq)
}

// gatewayURL resolves the gateway base URL for a provider. An explicit
// --provider-url remains an override for diagnostics and private gateways.
func gatewayURL(cmd *cobra.Command, provider string, hostURI hostURIQuery) (string, error) {
	if providerURL, _ := cmd.Flags().GetString("provider-url"); providerURL != "" {
		return providerURL, nil
	}

	providerURL, err := hostURI(cmd.Context(), provider)
	if err != nil {
		return "", err
	}
	if providerURL == "" {
		return "", fmt.Errorf("provider %s has no host URI on chain", provider)
	}

	return providerURL, nil
}

// activeLeaseProviders returns every provider holding an active lease for the
// deployment, sorted so output and retries are stable. It backs
// `send-manifest` without --provider (SPEC §2.4), which fans out to all of
// them.
func activeLeaseProviders(ctx context.Context, scope leaseScope, leases leaseQuery) ([]string, error) {
	if scope.Owner == "" {
		return nil, fmt.Errorf(
			"cannot resolve providers for deployment %d: the context has no configured default account; "+
				"set one with `akt context edit <context> --default-account <key>`, or name the provider with --provider",
			scope.DSeq)
	}

	found, err := leases(ctx, scope.Owner, scope.DSeq)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(found))
	for _, lease := range found {
		if lease.State == mtypes.LeaseActive && lease.ID.Provider != "" {
			seen[lease.ID.Provider] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("%s has no active lease: %s", deploymentLabel(scope), leaseRemedy(scope))
	}

	providers := make([]string, 0, len(seen))
	for provider := range seen {
		providers = append(providers, provider)
	}
	sort.Strings(providers)

	return providers, nil
}

// chainLeaseQuery reads the deployment's leases from the market module over the
// same query context the host-URI lookup already opens, so provider resolution
// costs one extra round trip and no extra connection.
func chainLeaseQuery(cmd *cobra.Command) leaseQuery {
	return func(ctx context.Context, owner string, dseq uint64) ([]mtypes.Lease, error) {
		cctx, err := providerQueryContext(cmd)
		if err != nil {
			return nil, err
		}

		client := mvbeta.NewQueryClient(cctx)

		var (
			leases   []mtypes.Lease
			pageKey  []byte
			seenKeys = make(map[string]struct{})
		)

		for {
			res, err := client.Leases(ctx, &mvbeta.QueryLeasesRequest{
				Filters: mtypes.LeaseFilters{
					Owner: owner,
					DSeq:  dseq,
				},
				Pagination: &sdkquery.PageRequest{
					Key:   append([]byte(nil), pageKey...),
					Limit: leasePageLimit,
				},
			})
			if err != nil {
				return nil, fmt.Errorf("query leases for deployment %d: %w", dseq, err)
			}
			if res == nil {
				return nil, fmt.Errorf("query leases for deployment %d: empty response", dseq)
			}

			for _, entry := range res.Leases {
				leases = append(leases, entry.Lease)
			}

			var nextKey []byte
			if res.Pagination != nil {
				nextKey = res.Pagination.NextKey
			}
			if len(nextKey) == 0 {
				return leases, nil
			}

			key := string(nextKey)
			if _, seen := seenKeys[key]; seen {
				return nil, fmt.Errorf("query leases for deployment %d: repeated pagination key", dseq)
			}
			seenKeys[key] = struct{}{}
			pageKey = append(pageKey[:0], nextKey...)
		}
	}
}
