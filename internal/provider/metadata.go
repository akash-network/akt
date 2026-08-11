package provider

import (
	"context"
	"encoding/json"
	"sort"

	atypes "pkg.akt.dev/go/node/audit/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	attrv1 "pkg.akt.dev/go/node/types/attributes/v1"
)

// Metadata is the provider information persisted alongside a bid.
type Metadata struct {
	Attributes map[string]string `json:"attributes" yaml:"attributes"`
	Audited    bool              `json:"audited"    yaml:"audited"`
}

// MetadataQueries is the narrow chain-query surface needed to enrich bids.
type MetadataQueries interface {
	Provider() ptypes.QueryClient
	Audit() atypes.QueryClient
}

// FetchChainMetadata resolves each unique provider once. Metadata is
// best-effort: an entry is returned only when both the provider registration
// and audit query completed, allowing callers to distinguish a known false
// audit result from an unavailable refresh.
func FetchChainMetadata(ctx context.Context, queries MetadataQueries, owners []string) map[string]Metadata {
	unique := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		if owner != "" {
			unique[owner] = struct{}{}
		}
	}

	ordered := make([]string, 0, len(unique))
	for owner := range unique {
		ordered = append(ordered, owner)
	}
	sort.Strings(ordered)

	result := make(map[string]Metadata, len(ordered))
	for _, owner := range ordered {
		provider, err := queries.Provider().Provider(ctx, &ptypes.QueryProviderRequest{Owner: owner})
		if err != nil {
			continue
		}

		audits, err := queries.Audit().ProviderAttributes(ctx, &atypes.QueryProviderAttributesRequest{Owner: owner})
		if err != nil {
			continue
		}

		result[owner] = Metadata{
			Attributes: attributeMap(provider.Provider.Attributes),
			Audited:    len(audits.Providers) > 0,
		}
	}

	return result
}

func attributeMap(attributes attrv1.Attributes) map[string]string {
	result := make(map[string]string, len(attributes))
	for _, attribute := range attributes {
		result[attribute.Key] = attribute.Value
	}

	return result
}

// AttributesFromProviderJSON reads the attributes field of a Console provider
// detail response. Console deployments have returned both [{key,value}] and
// object forms over time, so both wire shapes are accepted at this boundary.
func AttributesFromProviderJSON(raw json.RawMessage) map[string]string {
	var document struct {
		Attributes json.RawMessage `json:"attributes"`
	}
	if err := json.Unmarshal(raw, &document); err != nil || len(document.Attributes) == 0 {
		return nil
	}

	var list []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(document.Attributes, &list); err == nil {
		result := make(map[string]string, len(list))
		for _, attribute := range list {
			result[attribute.Key] = attribute.Value
		}

		return result
	}

	var object map[string]string
	if err := json.Unmarshal(document.Attributes, &object); err != nil {
		return nil
	}
	if object == nil {
		return map[string]string{}
	}

	return object
}
