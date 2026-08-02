package adapters

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
)

func TestReadSDLFromPath(t *testing.T) {
	obj, err := readSDL([]byte(testSDLPath))
	if err != nil {
		t.Fatalf("readSDL(path): %v", err)
	}

	mani, err := obj.Manifest()
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(mani) != 1 {
		t.Fatalf("manifest groups = %d, want 1", len(mani))
	}
	if len(mani[0].Services) != 1 || mani[0].Services[0].Name != "web" {
		t.Errorf("manifest services = %+v, want single web service", mani[0].Services)
	}
}

func TestReadSDLFromContent(t *testing.T) {
	content, err := os.ReadFile(testSDLPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	obj, err := readSDL(content)
	if err != nil {
		t.Fatalf("readSDL(content): %v", err)
	}

	if _, err := obj.Manifest(); err != nil {
		t.Fatalf("Manifest: %v", err)
	}
}

func TestReadSDLErrors(t *testing.T) {
	if _, err := readSDL(nil); err == nil {
		t.Error("expected error for empty SDL")
	}
	if _, err := readSDL([]byte("   \n\t")); err == nil {
		t.Error("expected error for blank SDL")
	}
}

func TestSendManifestRequiresDSeq(t *testing.T) {
	p := NewProviderClient(testClientContext(), "")

	if err := p.SendManifest(context.Background(), testProviderAddr().String(), 0, []byte(testSDLPath)); err == nil {
		t.Error("expected error for zero dseq")
	}
}

func TestSendManifestToActiveLeasesRequiresDSeq(t *testing.T) {
	p := NewProviderClient(testClientContext(), "")

	if _, err := p.SendManifestToActiveLeases(context.Background(), 0, []byte(testSDLPath)); err == nil {
		t.Error("expected error for zero dseq")
	}
}

func TestLeaseStatusRequiresDSeq(t *testing.T) {
	p := NewProviderClient(testClientContext(), "")

	if _, err := p.LeaseStatus(context.Background(), testProviderAddr().String(), 0); err == nil {
		t.Error("expected error for zero dseq")
	}
}

func TestActiveLeaseProvidersPaginatesDeduplicatesAndSorts(t *testing.T) {
	var requests []*mtypes.QueryLeasesRequest
	fetch := func(_ context.Context, req *mtypes.QueryLeasesRequest) (*mtypes.QueryLeasesResponse, error) {
		requests = append(requests, req)
		switch len(requests) {
		case 1:
			return leasePage([]string{"akash1z", "akash1a"}, []byte("next")), nil
		case 2:
			return leasePage([]string{"akash1a", "akash1m"}, nil), nil
		default:
			t.Fatalf("unexpected page %d", len(requests))
			return nil, nil
		}
	}

	got, err := activeLeaseProviders(context.Background(), "akash1owner", 4242, fetch)
	if err != nil {
		t.Fatalf("activeLeaseProviders: %v", err)
	}
	want := []string{"akash1a", "akash1m", "akash1z"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("providers = %v, want %v", got, want)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	for _, req := range requests {
		if req.Filters.Owner != "akash1owner" || req.Filters.DSeq != 4242 || req.Filters.State != "active" {
			t.Errorf("filters = %+v", req.Filters)
		}
		if req.Pagination == nil || req.Pagination.Limit != 100 {
			t.Errorf("pagination = %+v, want limit 100", req.Pagination)
		}
	}
	if string(requests[0].Pagination.Key) != "" || string(requests[1].Pagination.Key) != "next" {
		t.Errorf("page keys = %q, %q", requests[0].Pagination.Key, requests[1].Pagination.Key)
	}
}

func TestActiveLeaseProvidersRejectsRepeatedPageKey(t *testing.T) {
	fetch := func(_ context.Context, _ *mtypes.QueryLeasesRequest) (*mtypes.QueryLeasesResponse, error) {
		return leasePage(nil, []byte("same")), nil
	}

	_, err := activeLeaseProviders(context.Background(), "akash1owner", 1, fetch)
	if err == nil {
		t.Fatal("expected repeated pagination key error")
	}
}

func TestSendManifestToProvidersAttemptsAll(t *testing.T) {
	wantErr := errors.New("provider b refused")
	var attempted []string

	sent, err := sendManifestToProviders(
		context.Background(),
		[]string{"akash1a", "akash1b", "akash1c"},
		func(_ context.Context, provider string) error {
			attempted = append(attempted, provider)
			if provider == "akash1b" {
				return wantErr
			}
			return nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped provider failure", err)
	}
	if !reflect.DeepEqual(attempted, []string{"akash1a", "akash1b", "akash1c"}) {
		t.Errorf("attempted = %v", attempted)
	}
	if !reflect.DeepEqual(sent, []string{"akash1a", "akash1c"}) {
		t.Errorf("successful sends = %v", sent)
	}
}

func leasePage(providers []string, nextKey []byte) *mtypes.QueryLeasesResponse {
	leases := make([]mtypes.QueryLeaseResponse, 0, len(providers))
	for _, provider := range providers {
		leases = append(leases, mtypes.QueryLeaseResponse{Lease: mv1.Lease{ID: mv1.LeaseID{Provider: provider}}})
	}

	return &mtypes.QueryLeasesResponse{
		Leases:     leases,
		Pagination: &sdkquery.PageResponse{NextKey: nextKey},
	}
}
