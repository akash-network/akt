package provider

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	atypes "pkg.akt.dev/go/node/audit/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	attrv1 "pkg.akt.dev/go/node/types/attributes/v1"
)

type metadataQueries struct {
	provider ptypes.QueryClient
	audit    atypes.QueryClient
}

func (q metadataQueries) Provider() ptypes.QueryClient { return q.provider }
func (q metadataQueries) Audit() atypes.QueryClient    { return q.audit }

type providerMetadataQuery struct {
	ptypes.QueryClient
	calls []string
}

func (q *providerMetadataQuery) Provider(_ context.Context, req *ptypes.QueryProviderRequest, _ ...grpc.CallOption) (*ptypes.QueryProviderResponse, error) {
	q.calls = append(q.calls, req.Owner)
	if req.Owner == "akash1missing" {
		return nil, errors.New("provider not found")
	}

	attrs := attrv1.Attributes{}
	if req.Owner == "akash1audited" {
		attrs = append(attrs, attrv1.Attribute{Key: "region", Value: "us-west"})
	}

	return &ptypes.QueryProviderResponse{Provider: ptypes.Provider{
		Owner:      req.Owner,
		Attributes: attrs,
	}}, nil
}

type auditMetadataQuery struct {
	atypes.QueryClient
	calls []string
}

func (q *auditMetadataQuery) ProviderAttributes(_ context.Context, req *atypes.QueryProviderAttributesRequest, _ ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	q.calls = append(q.calls, req.Owner)
	if req.Owner == "akash1audit-error" {
		return nil, errors.New("audit query failed")
	}
	if req.Owner == "akash1audited" {
		return &atypes.QueryProvidersResponse{Providers: atypes.AuditedProviders{{Owner: req.Owner, Auditor: "akash1auditor"}}}, nil
	}

	return &atypes.QueryProvidersResponse{}, nil
}

func TestFetchChainMetadataDeduplicatesAndPreservesKnownFalse(t *testing.T) {
	providerQuery := &providerMetadataQuery{}
	auditQuery := &auditMetadataQuery{}

	got := FetchChainMetadata(context.Background(), metadataQueries{
		provider: providerQuery,
		audit:    auditQuery,
	}, []string{"akash1plain", "akash1audited", "akash1audited", "akash1missing", "akash1audit-error"})

	if len(got) != 2 {
		t.Fatalf("metadata = %#v, want two successfully resolved providers", got)
	}
	if got["akash1audited"].Attributes["region"] != "us-west" || !got["akash1audited"].Audited {
		t.Errorf("audited metadata = %#v", got["akash1audited"])
	}
	if got["akash1plain"].Attributes == nil || got["akash1plain"].Audited {
		t.Errorf("plain metadata = %#v, want known empty attributes and audited=false", got["akash1plain"])
	}
	if len(providerQuery.calls) != 4 {
		t.Errorf("provider calls = %v, want each unique owner once", providerQuery.calls)
	}
	if len(auditQuery.calls) != 3 {
		t.Errorf("audit calls = %v, want only providers whose registration resolved", auditQuery.calls)
	}
}

func TestAttributesFromProviderJSONAcceptsArrayAndObject(t *testing.T) {
	for name, raw := range map[string]string{
		"array":       `{"attributes":[{"key":"region","value":"eu-west"}]}`,
		"object":      `{"attributes":{"region":"eu-west"}}`,
		"null object": `{"attributes":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			got := AttributesFromProviderJSON([]byte(raw))
			if name == "null object" {
				if got == nil || len(got) != 0 {
					t.Fatalf("attributes = %#v, want known empty map", got)
				}
				return
			}
			if got["region"] != "eu-west" {
				t.Fatalf("attributes = %#v", got)
			}
		})
	}
}

func TestAttributesFromProviderJSONRejectsMalformedBoundaries(t *testing.T) {
	for name, raw := range map[string]string{
		"document":    `not-json`,
		"missing":     `{}`,
		"wrong shape": `{"attributes":7}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := AttributesFromProviderJSON([]byte(raw)); got != nil {
				t.Fatalf("attributes = %#v, want nil", got)
			}
		})
	}
}
