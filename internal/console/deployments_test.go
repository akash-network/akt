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

func TestCreateDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/deployments", r.URL.Path)

		data := decodeDataBody(t, r)
		assert.Equal(t, "test-sdl", data["sdl"])
		assert.InDelta(t, 5.0, data["deposit"], 0.001)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"dseq":"12345","manifest":"[{\"name\":\"web\"}]","signTx":{"code":0,"transactionHash":"ABC123","rawLog":""}}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.CreateDeployment(context.Background(), "test-sdl", 5.0)
	require.NoError(t, err)
	assert.Equal(t, "12345", resp.DSeq.String())
	assert.Equal(t, `[{"name":"web"}]`, resp.Manifest)
	require.NotNil(t, resp.SignTx)
	assert.Equal(t, "ABC123", resp.SignTx.TransactionHash)
}

func TestListDeployments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/deployments", r.URL.Path)
		assert.Equal(t, "0", r.URL.Query().Get("skip"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))

		_, _ = w.Write([]byte(`{"data":{
			"deployments":[
				{"deployment":{"id":{"owner":"akash1x","dseq":"100"},"state":"active","created_at":"123"},
				 "leases":[{"id":{"owner":"akash1x","dseq":"100","gseq":1,"oseq":1,"provider":"akash1p"},"state":"active","price":{"denom":"uakt","amount":"1.5"}}]},
				{"deployment":{"id":{"owner":"akash1x","dseq":"200"},"state":"closed"},"leases":[]}
			],
			"pagination":{"total":25,"skip":0,"limit":10,"hasMore":true}
		}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.ListDeployments(context.Background(), 0, 10)
	require.NoError(t, err)
	require.Len(t, resp.Deployments, 2)
	assert.Equal(t, "100", resp.Deployments[0].Deployment.ID.DSeq.String())
	assert.Equal(t, "active", resp.Deployments[0].Deployment.State)
	require.Len(t, resp.Deployments[0].Leases, 1)
	assert.Equal(t, "akash1p", resp.Deployments[0].Leases[0].ID.Provider)
	assert.Equal(t, "200", resp.Deployments[1].Deployment.ID.DSeq.String())
	assert.Equal(t, 25, resp.Pagination.Total)
	assert.True(t, resp.Pagination.HasMore)
}

func TestGetDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/deployments/777", r.URL.Path)

		_, _ = w.Write([]byte(`{"data":{
			"deployment":{"id":{"owner":"akash1x","dseq":"777"},"state":"active"},
			"leases":[{
				"id":{"owner":"akash1x","dseq":"777","gseq":1,"oseq":1,"provider":"akash1p"},
				"state":"active",
				"price":{"denom":"uakt","amount":"2.75"},
				"status":{"services":{"web":{"uris":["web.example.com"]}},"forwarded_ports":{},"ips":null}
			}],
			"escrow_account":{"state":{"funds":{"denom":"uakt","amount":"5000000"},"transferred":{"denom":"uakt","amount":"100"}}}
		}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.GetDeployment(context.Background(), "777")
	require.NoError(t, err)
	assert.Equal(t, "777", resp.Deployment.ID.DSeq.String())
	require.Len(t, resp.Leases, 1)

	lease := resp.Leases[0]
	assert.Equal(t, uint32(1), lease.ID.GSeq)
	require.NotNil(t, lease.Price)
	assert.Equal(t, "2.75", lease.Price.Amount.String())
	require.NotNil(t, lease.Status)
	assert.Contains(t, string(lease.Status.Services), "web.example.com")
	assert.Contains(t, string(resp.EscrowAccount), "transferred")
}

func TestUpdateDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/v1/deployments/555", r.URL.Path)

		data := decodeDataBody(t, r)
		assert.Equal(t, "new-sdl", data["sdl"])

		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"555"},"state":"active"}}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.UpdateDeployment(context.Background(), "555", "new-sdl")
	require.NoError(t, err)
	assert.Equal(t, "555", resp.Deployment.ID.DSeq.String())
}

func TestCloseDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v1/deployments/99999", r.URL.Path)

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	require.NoError(t, c.CloseDeployment(context.Background(), "99999"))
}

func TestCloseDeploymentAlreadyClosedSentinel(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"deployment closed"}`))
			}))
			defer srv.Close()

			c := console.New(srv.URL, "test-key")
			err := c.CloseDeployment(context.Background(), "1")
			require.Error(t, err)
			assert.ErrorIs(t, err, console.ErrAlreadyClosed,
				"HTTP %d must map to ErrAlreadyClosed for idempotent close", status)
		})
	}
}

func TestDepositBodyShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/deposit-deployment", r.URL.Path)

		data := decodeDataBody(t, r)
		assert.Equal(t, "321", data["dseq"])
		assert.InDelta(t, 12.5, data["deposit"], 0.001)
		assert.NotContains(t, data, "amount", "wire field is deposit, not amount")

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	require.NoError(t, c.Deposit(context.Background(), "321", 12.5))
}

func TestFetchBids(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/bids", r.URL.Path)
		assert.Equal(t, "444", r.URL.Query().Get("dseq"))

		_, _ = w.Write([]byte(`{"data":[
			{"bid":{"id":{"owner":"akash1x","dseq":"444","gseq":1,"oseq":1,"provider":"akash1p1"},"state":"open","price":{"denom":"uakt","amount":"1.1"}}},
			{"bid":{"id":{"owner":"akash1x","dseq":"444","gseq":1,"oseq":1,"provider":"akash1p2"},"state":"open","price":{"denom":"uakt","amount":"0.9"}}}
		]}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	bids, err := c.FetchBids(context.Background(), "444")
	require.NoError(t, err)
	require.Len(t, bids, 2)
	assert.Equal(t, "akash1p1", bids[0].ID.Provider)
	assert.Equal(t, "0.9", bids[1].Price.Amount.String())
}

func TestCreateLeaseBodyNotEnveloped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/leases", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.NotContains(t, body, "data", "/v1/leases request must NOT be data-enveloped")
		assert.Equal(t, "manifest-json", body["manifest"])

		leases, ok := body["leases"].([]any)
		require.True(t, ok)
		require.Len(t, leases, 1)
		lease := leases[0].(map[string]any)
		assert.Equal(t, "888", lease["dseq"])
		assert.InDelta(t, 1.0, lease["gseq"], 0.001, "gseq must be a JSON number")
		assert.InDelta(t, 1.0, lease["oseq"], 0.001, "oseq must be a JSON number")
		assert.Equal(t, "akash1p", lease["provider"])

		_, _ = w.Write([]byte(`{"data":{
			"deployment":{"id":{"owner":"akash1x","dseq":"888"},"state":"active"},
			"leases":[{"id":{"owner":"akash1x","dseq":"888","gseq":1,"oseq":1,"provider":"akash1p"},"state":"active"}]
		}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.CreateLease(context.Background(), "manifest-json", []console.LeaseRequest{
		{DSeq: "888", GSeq: 1, OSeq: 1, Provider: "akash1p"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Leases, 1)
	assert.Equal(t, "active", resp.Leases[0].State)
}

func TestGetDeploymentSettings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v2/deployment-settings/123", r.URL.Path)

		_, _ = w.Write([]byte(`{"data":{"dseq":"123","autoTopUpEnabled":true,"estimatedTopUpAmount":4.2,"topUpFrequencyMs":86400000}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	s, err := c.GetDeploymentSettings(context.Background(), "123")
	require.NoError(t, err)
	assert.True(t, s.AutoTopUpEnabled)
	assert.InDelta(t, 4.2, s.EstimatedTopUpAmount, 0.001)
	assert.Equal(t, int64(86400000), s.TopUpFrequencyMs)
}

func TestGetDeploymentSettingsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	_, err := c.GetDeploymentSettings(context.Background(), "123")
	assert.ErrorIs(t, err, console.ErrNotFound)
}

func TestSetDeploymentAutoTopUpPatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/v2/deployment-settings/321", r.URL.Path)

		data := decodeDataBody(t, r)
		assert.Equal(t, true, data["autoTopUpEnabled"])

		_, _ = w.Write([]byte(`{"data":{"dseq":"321","autoTopUpEnabled":true}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	s, err := c.SetDeploymentAutoTopUp(context.Background(), "321", true)
	require.NoError(t, err)
	assert.True(t, s.AutoTopUpEnabled)
}

func TestSetDeploymentAutoTopUpFallsBackToPost(t *testing.T) {
	var patchCalled, postCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			patchCalled = true
			assert.Equal(t, "/v2/deployment-settings/654", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)

		case http.MethodPost:
			postCalled = true
			assert.Equal(t, "/v2/deployment-settings", r.URL.Path)

			data := decodeDataBody(t, r)
			assert.Equal(t, "654", data["dseq"])
			assert.Equal(t, false, data["autoTopUpEnabled"])

			_, _ = w.Write([]byte(`{"data":{"dseq":"654","autoTopUpEnabled":false}}`))

		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	s, err := c.SetDeploymentAutoTopUp(context.Background(), "654", false)
	require.NoError(t, err)
	assert.True(t, patchCalled, "PATCH must be attempted first")
	assert.True(t, postCalled, "POST fallback must run after PATCH 404")
	assert.False(t, s.AutoTopUpEnabled)
	assert.Equal(t, "654", s.DSeq.String())
}
