package rpc

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	providerv1beta4 "pkg.akt.dev/go/node/provider/v1beta4"
	attributesv1 "pkg.akt.dev/go/node/types/attributes/v1"
)

func TestGetProvidersOnChainPaginatesAndDecodesAttributes(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/abci_query" {
			http.NotFound(w, r)
			return
		}
		call := requests.Add(1)
		if path := r.URL.Query().Get("path"); path != `"`+ProvidersQueryPath+`"` {
			t.Errorf("path query = %q, want %q", path, `"`+ProvidersQueryPath+`"`)
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}

		response := providerv1beta4.QueryProvidersResponse{}
		switch call {
		case 1:
			if data := r.URL.Query().Get("data"); data != "" {
				t.Errorf("first page data query = %q, want empty", data)
				http.Error(w, "unexpected first-page data", http.StatusBadRequest)
				return
			}
			response.Providers = providerv1beta4.Providers{{
				Owner:   "akash1provider1",
				HostURI: "https://provider-one.example",
				Attributes: attributesv1.Attributes{
					{Key: "region", Value: "us-west"},
					{Key: "tier", Value: "community"},
				},
			}}
			response.Pagination = &querytypes.PageResponse{NextKey: []byte("next-page")}
		case 2:
			data, err := hex.DecodeString(strings.TrimPrefix(r.URL.Query().Get("data"), "0x"))
			if err != nil {
				t.Errorf("decode request data: %v", err)
				http.Error(w, "invalid request data", http.StatusBadRequest)
				return
			}
			var request providerv1beta4.QueryProvidersRequest
			if err := request.Unmarshal(data); err != nil {
				t.Errorf("unmarshal request data: %v", err)
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			if request.Pagination == nil {
				t.Error("second page pagination is nil")
				http.Error(w, "missing pagination", http.StatusBadRequest)
				return
			}
			if key := request.Pagination.Key; string(key) != "next-page" {
				t.Errorf("pagination key = %q, want next-page", key)
				http.Error(w, "unexpected pagination key", http.StatusBadRequest)
				return
			}
			if limit := request.Pagination.Limit; limit != 100 {
				t.Errorf("pagination limit = %d, want 100", limit)
				http.Error(w, "unexpected pagination limit", http.StatusBadRequest)
				return
			}
			response.Providers = providerv1beta4.Providers{{
				Owner:      "akash1provider2",
				HostURI:    "https://provider-two.example",
				Attributes: attributesv1.Attributes{{Key: "region", Value: "eu-central"}},
			}}
			response.Pagination = &querytypes.PageResponse{}
		default:
			http.Error(w, "too many pages", http.StatusBadRequest)
			return
		}

		encoded, err := response.Marshal()
		if err != nil {
			t.Errorf("marshal response: %v", err)
			http.Error(w, "failed to marshal response", http.StatusInternalServerError)
			return
		}
		writeABCIResult(w, 0, "", encoded)
	}))
	t.Cleanup(server.Close)

	client := NewRPCProviderClient(server.URL)
	require.Equal(t, server.URL, client.rpcEndpoint)
	require.Equal(t, ABCIQueryTimeout, client.httpClient.Timeout)
	providers, err := client.GetProvidersOnChain(context.Background())
	require.NoError(t, err)
	require.Equal(t, []OnChainProvider{
		{
			Owner:   "akash1provider1",
			HostURI: "https://provider-one.example",
			Attributes: map[string]string{
				"region": "us-west",
				"tier":   "community",
			},
		},
		{
			Owner:      "akash1provider2",
			HostURI:    "https://provider-two.example",
			Attributes: map[string]string{"region": "eu-central"},
		},
	}, providers)
	require.Equal(t, int32(2), requests.Load())
	require.NoError(t, client.Close())
}

func TestProviderABCIQueryFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		write  func(http.ResponseWriter)
		want   string
	}{
		{name: "HTTP status", status: http.StatusBadGateway, body: "gateway failed", want: "ABCI query failed with status 502: gateway failed"},
		{name: "malformed JSON", body: "not-json", want: "failed to parse ABCI response"},
		{name: "ABCI error", write: func(w http.ResponseWriter) { writeABCIResult(w, 12, "bad query", nil) }, want: "ABCI query error: code=12 log=bad query"},
		{name: "invalid base64", body: `{"result":{"response":{"code":0,"value":"%%%"}}}`, want: "failed to decode base64 value"},
		{name: "invalid protobuf", write: func(w http.ResponseWriter) { writeABCIResult(w, 0, "", []byte{0xff}) }, want: "failed to unmarshal providers response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.write != nil {
					tc.write(w)
					return
				}
				status := tc.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)

			_, err := NewRPCProviderClient(server.URL).GetProvidersOnChain(context.Background())
			require.ErrorContains(t, err, tc.want)
		})
	}

	invalid := NewRPCProviderClient("://bad endpoint")
	_, _, err := invalid.fetchProvidersPage(context.Background(), nil)
	require.ErrorContains(t, err, "failed to create request")

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	_, err = NewRPCProviderClient(closedURL).GetProvidersOnChain(context.Background())
	require.ErrorContains(t, err, "failed to query providers")

	readError := NewRPCProviderClient("http://rpc.example")
	readError.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: errorReader{err: errors.New("read failed")}, Header: make(http.Header)}, nil
	})
	_, err = readError.GetProvidersOnChain(context.Background())
	require.ErrorContains(t, err, "failed to read response: read failed")
}

func TestGetActiveLeaseProvidersPaginatesAndDeduplicates(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/akash/market/v1beta5/leases/list" {
			http.NotFound(w, r)
			return
		}
		if state := r.URL.Query().Get("filters.state"); state != "active" {
			t.Errorf("filters.state = %q, want active", state)
			http.Error(w, "unexpected lease state", http.StatusBadRequest)
			return
		}
		if limit := r.URL.Query().Get("pagination.limit"); limit != "500" {
			t.Errorf("pagination.limit = %q, want 500", limit)
			http.Error(w, "unexpected page limit", http.StatusBadRequest)
			return
		}
		requests.Add(1)
		switch r.URL.Query().Get("pagination.key") {
		case "":
			_, _ = fmt.Fprint(w, `{"leases":[{"lease":{"id":{"provider":"akash1provider1"},"state":"active"}},{"lease":{"id":{"provider":""},"state":"active"}}],"pagination":{"next_key":"next+/="}}`)
		case "next+/=":
			_, _ = fmt.Fprint(w, `{"leases":[{"lease":{"id":{"provider":"akash1provider1"},"state":"active"}},{"lease":{"id":{"provider":"akash1provider2"},"state":"active"}}],"pagination":{"next_key":""}}`)
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	providers, err := NewRPCProviderClient(server.URL).GetActiveLeaseProviders(context.Background(), server.URL)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{
		"akash1provider1": true,
		"akash1provider2": true,
	}, providers)
	require.Equal(t, int32(2), requests.Load())
}

func TestActiveLeaseQueryFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "status", status: http.StatusServiceUnavailable, body: "maintenance", want: "leases query failed with status 503: maintenance"},
		{name: "malformed", body: "not-json", want: "failed to parse leases response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				status := tc.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)

			_, err := NewRPCProviderClient(server.URL).GetActiveLeaseProviders(context.Background(), server.URL)
			require.ErrorContains(t, err, tc.want)
		})
	}

	client := NewRPCProviderClient("http://rpc.example")
	_, _, err := client.fetchLeasesPage(context.Background(), "://bad endpoint", "")
	require.ErrorContains(t, err, "failed to create request")

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	_, err = client.GetActiveLeaseProviders(context.Background(), closedURL)
	require.ErrorContains(t, err, "failed to query active leases")

	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: errorReader{err: errors.New("read failed")}, Header: make(http.Header)}, nil
	})
	_, err = client.GetActiveLeaseProviders(context.Background(), "http://rest.example")
	require.ErrorContains(t, err, "failed to read response: read failed")
}

func TestGetProvidersFromSeedMapsResponseAndFailures(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := NewRPCProviderClient("http://rpc.example")
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, SeedURL, req.URL.String())
			return stringResponse(http.StatusOK, `[{"owner":"akash1provider","hostUri":"https://provider.example"}]`), nil
		})

		providers, err := client.GetProvidersFromSeed(context.Background())
		require.NoError(t, err)
		require.Equal(t, []OnChainProvider{{
			Owner: "akash1provider", HostURI: "https://provider.example", IsOnline: false,
		}}, providers)
	})

	for _, tc := range []struct {
		name     string
		response func() *http.Response
		err      error
		want     string
	}{
		{name: "network", err: errors.New("offline"), want: "failed to fetch from seed:"},
		{name: "status", response: func() *http.Response { return stringResponse(http.StatusBadGateway, "bad gateway") }, want: "seed fetch returned status 502"},
		{name: "malformed", response: func() *http.Response { return stringResponse(http.StatusOK, "not-json") }, want: "failed to parse seed response"},
		{name: "read", response: func() *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Body: errorReader{err: errors.New("read failed")}, Header: make(http.Header)}
		}, want: "failed to read seed response: read failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewRPCProviderClient("http://rpc.example")
			client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				if tc.response == nil {
					return nil, tc.err
				}
				return tc.response(), tc.err
			})
			_, err := client.GetProvidersFromSeed(context.Background())
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func writeABCIResult(w http.ResponseWriter, code int, log string, value []byte) {
	encoded := base64.StdEncoding.EncodeToString(value)
	_, _ = fmt.Fprintf(w, `{"result":{"response":{"code":%d,"log":%q,"value":%q}}}`, code, log, encoded)
}

func stringResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
