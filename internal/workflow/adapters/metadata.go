package adapters

import (
	"context"
	"encoding/json"
	"sort"

	"pkg.akt.dev/akt/internal/console"
	aktprovider "pkg.akt.dev/akt/internal/provider"
)

func attachProviderMetadata(raw json.RawMessage, metadata map[string]aktprovider.Metadata) (json.RawMessage, error) {
	if len(metadata) == 0 {
		return raw, nil
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	document["provider_metadata"] = encoded

	return json.Marshal(document)
}

func fetchConsoleProviderMetadata(ctx context.Context, cc *console.Client, bids []console.Bid) map[string]aktprovider.Metadata {
	unique := make(map[string]struct{}, len(bids))
	for _, bid := range bids {
		if bid.ID.Provider != "" {
			unique[bid.ID.Provider] = struct{}{}
		}
	}

	owners := make([]string, 0, len(unique))
	for owner := range unique {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	metadata := make(map[string]aktprovider.Metadata, len(owners))
	for _, owner := range owners {
		detail, err := cc.GetProvider(ctx, owner)
		if err != nil {
			continue
		}

		attributes := aktprovider.AttributesFromProviderJSON(detail.Raw)
		if attributes == nil {
			attributes = map[string]string{}
		}
		metadata[owner] = aktprovider.Metadata{
			Attributes: attributes,
			Audited:    detail.IsAudited,
		}
	}

	return metadata
}
