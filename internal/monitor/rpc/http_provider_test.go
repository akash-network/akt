package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatorMonikersFollowPaginationAndSkipIncompleteEntries(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cosmos/staking/v1beta1/validators" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		if limit := r.URL.Query().Get("pagination.limit"); limit != "100" {
			t.Errorf("pagination.limit = %q, want 100", limit)
			http.Error(w, "unexpected page limit", http.StatusBadRequest)
			return
		}
		switch r.URL.Query().Get("pagination.key") {
		case "":
			_, _ = fmt.Fprint(w, `{"validators":[{"description":{"moniker":"alpha"},"consensus_pubkey":{"@type":"/cosmos.crypto.ed25519.PubKey","key":"key-a"}},{"description":{"moniker":"missing-key"},"consensus_pubkey":{}}],"pagination":{"next_key":"next+/="}}`)
		case "next+/=":
			_, _ = fmt.Fprint(w, `{"validators":[{"description":{"moniker":"beta"},"consensus_pubkey":{"key":"key-b"}},{"description":{},"consensus_pubkey":{"key":"missing-moniker"}}],"pagination":{"next_key":""}}`)
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	monikers, err := NewClient(server.URL, server.URL).GetValidatorMonikers(context.Background())
	require.NoError(t, err)
	require.Equal(t, map[string]string{"key-a": "alpha", "key-b": "beta"}, monikers)
	require.Equal(t, int32(2), requests.Load())
}

func TestValidatorMonikerFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "status", status: http.StatusBadGateway, want: "LCD returned status 502"},
		{name: "malformed", body: "{", want: "failed to parse LCD validators"},
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

			_, err := NewClient(server.URL, server.URL).GetValidatorMonikers(context.Background())
			require.ErrorContains(t, err, tc.want)
		})
	}

	invalid := NewClient("http://rpc.example", "://bad endpoint")
	_, _, err := invalid.fetchMonikersPage(context.Background(), "")
	require.ErrorContains(t, err, "failed to create request")

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	_, err = NewClient(server.URL, server.URL).GetValidatorMonikers(context.Background())
	require.ErrorContains(t, err, "failed to fetch validators from LCD")
}

func TestProviderHTTPQueriesAndNodeExtraction(t *testing.T) {
	paths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		switch r.URL.Path {
		case "/status":
			_, _ = fmt.Fprint(w, `{"cluster":{"inventory":{"available":{"nodes":[{"name":"gpu-1","allocatable":{"cpu":8000,"gpu":2,"memory":64000},"available":{"cpu":3500,"gpu":1,"memory":24000}},{"name":"cpu-1","allocatable":{"cpu":4000,"gpu":0,"memory":32000},"available":{"cpu":1000,"gpu":0,"memory":8000}}]}}}}`)
		case "/version":
			_, _ = fmt.Fprint(w, `{"akash":{"version":"v0.14.0-rc2"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewProviderHTTPClient(false)
	status, err := QueryProviderStatus(context.Background(), client, server.URL+"/")
	require.NoError(t, err)
	require.Equal(t, []ProviderNode{
		{Name: "gpu-1", CPUAllocatable: 8000, CPUAvailable: 3500, MemAllocatable: 64000, MemAvailable: 24000, GPUAllocatable: 2, GPUAvailable: 1},
		{Name: "cpu-1", CPUAllocatable: 4000, CPUAvailable: 1000, MemAllocatable: 32000, MemAvailable: 8000},
	}, status.GetNodes())

	version, err := QueryProviderVersion(context.Background(), client, server.URL+"/")
	require.NoError(t, err)
	require.Equal(t, "v0.14.0-rc2", version.Akash.Version)
	require.Equal(t, "/status", <-paths)
	require.Equal(t, "/version", <-paths)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	require.Equal(t, 100, transport.MaxIdleConns)
	require.Equal(t, 10, transport.MaxIdleConnsPerHost)
	require.Equal(t, 10, transport.MaxConnsPerHost)
	require.Equal(t, ProviderQueryTimeout, client.Timeout)
}

func TestProviderHTTPQueryFailures(t *testing.T) {
	for _, path := range []string{"/status", "/version"} {
		t.Run("status "+path, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != path {
					t.Errorf("path = %q, want %q", r.URL.Path, path)
					http.NotFound(w, r)
					return
				}
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			}))
			t.Cleanup(server.Close)

			var err error
			if path == "/status" {
				_, err = QueryProviderStatus(context.Background(), server.Client(), server.URL)
			} else {
				_, err = QueryProviderVersion(context.Background(), server.Client(), server.URL)
			}
			require.EqualError(t, err, "status 503")
		})

		t.Run("malformed "+path, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, "not-json")
			}))
			t.Cleanup(server.Close)

			var err error
			if path == "/status" {
				_, err = QueryProviderStatus(context.Background(), server.Client(), server.URL)
			} else {
				_, err = QueryProviderVersion(context.Background(), server.Client(), server.URL)
			}
			require.Error(t, err)
		})
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	_, err := QueryProviderStatus(context.Background(), closed.Client(), closedURL)
	require.Error(t, err)
	_, err = QueryProviderVersion(context.Background(), closed.Client(), closedURL)
	require.Error(t, err)

	_, err = QueryProviderStatus(context.Background(), http.DefaultClient, "://bad endpoint")
	require.ErrorContains(t, err, "failed to create request")
	_, err = QueryProviderVersion(context.Background(), http.DefaultClient, "://bad endpoint")
	require.ErrorContains(t, err, "failed to create request")

	readErrClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(errorReader{err: errors.New("read failed")}),
			Header:     make(http.Header),
		}, nil
	})}
	_, err = QueryProviderStatus(context.Background(), readErrClient, "http://provider.example")
	require.ErrorContains(t, err, "read failed")
	_, err = QueryProviderVersion(context.Background(), readErrClient, "http://provider.example")
	require.ErrorContains(t, err, "read failed")
}

func TestProviderVersionOrderingAndHostExtraction(t *testing.T) {
	providers := []Provider{
		{AkashVersion: "v0.14.0-rc2"},
		{AkashVersion: "v0.9.0"},
		{AkashVersion: "v0.14.0"},
		{AkashVersion: "v0.14.0-rc10"},
		{AkashVersion: "v0.14.0"},
		{},
	}
	require.Equal(t, []string{
		"v0.14.0", "v0.14.0-rc10", "v0.14.0-rc2", "v0.9.0",
	}, GetProviderVersions(providers))

	for _, tc := range []struct {
		left, right string
		want        int
	}{
		{left: "1.2.0+build7", right: "1.2", want: 0},
		{left: "1.0.0-1", right: "1.0.0-alpha", want: -1},
		{left: "1.0.0-alpha", right: "1.0.0-1", want: 1},
		{left: "1.0.0-alpha.1", right: "1.0.0-alpha", want: 1},
		{left: "garbage-b", right: "garbage-a", want: 1},
		{left: "1..2", right: "1.0.2", want: -1},
	} {
		require.Equal(t, tc.want, CompareVersions(tc.left, tc.right), "%q vs %q", tc.left, tc.right)
	}

	require.Equal(t, "provider.example.com", ExtractHostname("https://provider.example.com:8443"))
	require.Equal(t, "provider.example.com/path", ExtractHostname("http://provider.example.com/path"))
	require.Equal(t, "provider.example.com", ExtractHostname("provider.example.com:80"))
}

func TestModuleAndGenericParamsHonorResponseSemantics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/module/params":
			_, _ = fmt.Fprint(w, `{"params":{"enabled":true}}`)
		case "/cosmos/params/v1beta1/subspaces":
			_, _ = fmt.Fprint(w, `{"subspaces":[{"subspace":"staking","keys":["MaxValidators","Broken"]},{"subspace":"other","keys":[]}]}`)
		case "/cosmos/params/v1beta1/params":
			if subspace := r.URL.Query().Get("subspace"); subspace != "staking" {
				t.Errorf("subspace = %q, want staking", subspace)
				http.Error(w, "unexpected subspace", http.StatusBadRequest)
				return
			}
			switch r.URL.Query().Get("key") {
			case "MaxValidators":
				_, _ = fmt.Fprint(w, `{"param":{"subspace":"staking","key":"MaxValidators","value":100}}`)
			case "Broken":
				http.Error(w, "broken", http.StatusInternalServerError)
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, server.URL)
	raw, err := client.GetModuleParams(context.Background(), "module", "/module/params")
	require.NoError(t, err)
	require.JSONEq(t, `{"params":{"enabled":true}}`, string(raw))

	params, err := client.GetGenericParams(context.Background(), "staking")
	require.NoError(t, err)
	require.Len(t, params, 1)
	require.Equal(t, "MaxValidators", params[0].Key)
	require.JSONEq(t, `100`, string(params[0].Value))

	_, err = client.GetGenericParams(context.Background(), "missing")
	require.ErrorContains(t, err, "subspace missing not found")
}

func TestModuleAndGenericParamFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		call func(*Client) error
		want string
	}{
		{
			name: "module status", path: "/module", want: "bank params returned status 502",
			call: func(c *Client) error {
				_, err := c.GetModuleParams(context.Background(), "bank", "/module")
				return err
			},
		},
		{
			name: "subspaces status", path: "/cosmos/params/v1beta1/subspaces", want: "subspaces returned status 502",
			call: func(c *Client) error {
				_, err := c.GetGenericParams(context.Background(), "bank")
				return err
			},
		},
		{
			name: "param status", path: "/cosmos/params/v1beta1/params", want: "param bank/key returned status 502",
			call: func(c *Client) error {
				_, err := c.GetGenericParam(context.Background(), "bank", "key")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path = %q, want %q", r.URL.Path, tc.path)
					http.NotFound(w, r)
					return
				}
				http.Error(w, "bad gateway", http.StatusBadGateway)
			}))
			t.Cleanup(server.Close)
			require.ErrorContains(t, tc.call(NewClient(server.URL, server.URL)), tc.want)
		})
	}

	malformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "not-json")
	}))
	t.Cleanup(malformed.Close)
	client := NewClient(malformed.URL, malformed.URL)
	_, err := client.GetGenericParams(context.Background(), "bank")
	require.ErrorContains(t, err, "failed to parse subspaces")
	_, err = client.GetGenericParam(context.Background(), "bank", "key")
	require.ErrorContains(t, err, "failed to parse param")

	invalid := NewClient("http://rpc.example", "://bad endpoint")
	_, err = invalid.GetModuleParams(context.Background(), "bank", "/params")
	require.ErrorContains(t, err, "failed to create request")
	_, err = invalid.GetGenericParams(context.Background(), "bank")
	require.ErrorContains(t, err, "failed to create subspaces request")
	_, err = invalid.GetGenericParam(context.Background(), "bank", "key")
	require.ErrorContains(t, err, "failed to create param request")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func (errorReader) Close() error {
	return nil
}
