package console_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

func TestListProvidersTopLevelArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/providers", r.URL.Path)
		assert.Equal(t, "trial", r.URL.Query().Get("scope"))
		assert.Equal(t, "akash1p1,akash1p2", r.URL.Query().Get("addresses"))

		_, _ = w.Write([]byte(`[
			{"owner":"akash1p1","name":"provider-one","hostUri":"https://p1.example.com:8443","isOnline":true,"isAudited":true,"uptime7d":0.999,"gpuModels":["rtx4090"],"ipRegion":{"country":"US"}},
			{"owner":"akash1p2","name":"provider-two","hostUri":"https://p2.example.com:8443","isOnline":false}
		]`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "")
	ps, err := c.ListProviders(context.Background(), "trial", []string{"akash1p1", "akash1p2"})
	require.NoError(t, err)
	require.Len(t, ps, 2)
	assert.Equal(t, "akash1p1", ps[0].Owner)
	assert.True(t, ps[0].IsAudited)
	assert.InDelta(t, 0.999, ps[0].Uptime7D, 1e-6)
	assert.Contains(t, string(ps[0].GPUModels), "rtx4090")
	assert.False(t, ps[1].IsOnline)
}

func TestGetProviderDetailKeepsRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/providers/akash1p1", r.URL.Path)
		_, _ = w.Write([]byte(`{"owner":"akash1p1","hostUri":"https://p1.example.com:8443","stats":{"leaseCount":42}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "")
	p, err := c.GetProvider(context.Background(), "akash1p1")
	require.NoError(t, err)
	assert.Equal(t, "akash1p1", p.Owner)
	assert.Equal(t, "https://p1.example.com:8443", p.HostURI)
	assert.Contains(t, string(p.Raw), "leaseCount", "full document preserved in Raw")
}

func TestListProviderRegionsAndAuditors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/provider-regions":
			_, _ = w.Write([]byte(`[{"key":"us-west","description":"US West","providers":["akash1p1"]}]`))
		case "/v1/auditors":
			_, _ = w.Write([]byte(`[{"id":"1","name":"audited-by-overclock","address":"akash1aud","website":"https://ovrclk.com"}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := console.New(srv.URL, "")

	regions, err := c.ListProviderRegions(context.Background())
	require.NoError(t, err)
	require.Len(t, regions, 1)
	assert.Equal(t, "us-west", regions[0].Key)
	assert.Equal(t, []string{"akash1p1"}, regions[0].Providers)

	auditors, err := c.ListAuditors(context.Background())
	require.NoError(t, err)
	require.Len(t, auditors, 1)
	assert.Equal(t, "akash1aud", auditors[0].Address)
}

func TestGetGPUPrices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/gpu-prices", r.URL.Path)
		_, _ = w.Write([]byte(`{
			"availability":{"total":100,"available":40},
			"models":[{"vendor":"nvidia","model":"h100","ram":"80Gi","price":{"min":1.0,"max":3.0,"avg":2.1}}]
		}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "")
	prices, err := c.GetGPUPrices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 40, prices.Availability.Available)
	require.Len(t, prices.Models, 1)
	assert.Equal(t, "h100", prices.Models[0].Model)
	assert.InDelta(t, 2.1, prices.Models[0].Price.Avg, 0.001)
}

func TestScreenBids(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/bid-screening", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.NotContains(t, body, "data", "bid-screening request must NOT be data-enveloped")

		reqs, ok := body["requirements"].(map[string]any)
		require.True(t, ok)
		signedBy := reqs["signedBy"].(map[string]any)
		assert.Equal(t, []any{"akash1aud"}, signedBy["anyOf"])
		assert.Equal(t, "America/Los_Angeles", body["timezone"])
		assert.Contains(t, body, "resources")

		_, _ = w.Write([]byte(`{"providers":[
			{"owner":"akash1p1","hostUri":"https://p1.example.com:8443","isAudited":true,"location":{"region":"us-west"},"organization":"Overclock"}
		]}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "")
	providers, err := c.ScreenBids(context.Background(), &console.BidScreeningRequest{
		Requirements: console.BidScreeningRequirements{
			SignedBy:   console.SignedBy{AnyOf: []string{"akash1aud"}, AllOf: []string{}},
			Attributes: []console.Attribute{{Key: "region", Value: "us-west"}},
		},
		Resources: json.RawMessage(`[{"cpu":{"units":1000}}]`),
		Timezone:  "America/Los_Angeles",
	})
	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Equal(t, "akash1p1", providers[0].Owner)
	assert.True(t, providers[0].IsAudited)
	assert.Equal(t, "Overclock", providers[0].Organization)
}

func TestTemplates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/templates-list":
			_, _ = w.Write([]byte(`{"data":{"categories":[{"title":"AI","templates":[{"id":"tpl-1"}]}]}}`))
		case "/v1/templates/tpl-1":
			_, _ = w.Write([]byte(`{"data":{"id":"tpl-1","name":"Jupyter","summary":"notebooks","deploy":"---\nversion: \"2.0\"","readme":"# Jupyter"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := console.New(srv.URL, "")

	raw, err := c.ListTemplates(context.Background())
	require.NoError(t, err)
	assert.Contains(t, string(raw), "categories", "envelope unwrapped, raw categories returned")

	tpl, err := c.GetTemplate(context.Background(), "tpl-1")
	require.NoError(t, err)
	assert.Equal(t, "Jupyter", tpl.Name)
	assert.Contains(t, tpl.Deploy, "version")
}
