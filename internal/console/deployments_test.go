package console_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/go/sdl"
)

const validUpdateSDL = `version: "2.0"
services:
  web:
    image: nginx:1.27-alpine
    expose:
      - port: 80
        as: 80
        to:
          - global: true
profiles:
  compute:
    web:
      resources:
        cpu:
          units: 0.5
        memory:
          size: 512Mi
        storage:
          size: 512Mi
  placement:
    dcloud:
      pricing:
        web:
          denom: uact
          amount: 10000
deployment:
  web:
    dcloud:
      profile: web
      count: 1
`

func versionHash(t *testing.T, rawSDL string) string {
	t.Helper()

	doc, err := sdl.Read([]byte(rawSDL))
	require.NoError(t, err)

	version, err := doc.Version()
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(version)
}

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
			"deployment":{"id":{"owner":"akash1x","dseq":"777"},"state":"active","hash":"version-hash"},
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
	assert.Equal(t, "version-hash", resp.Deployment.Hash)
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

func TestUpdateDeploymentReconcilesFailedResponseByVersionHash(t *testing.T) {
	expectedHash := versionHash(t, validUpdateSDL)
	var puts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			puts.Add(1)
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"manifest version validation failed"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"555"},"state":"active","hash":"` + expectedHash + `"}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	resp, err := console.New(srv.URL, "test-key").UpdateDeployment(
		context.Background(), "555", validUpdateSDL,
	)
	require.NoError(t, err)
	assert.Equal(t, expectedHash, resp.Deployment.Hash)
	assert.Equal(t, int32(1), puts.Load(), "a proven update must not be replayed")
}

func TestUpdateDeploymentRetriesTransientManifestValidation(t *testing.T) {
	var puts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if puts.Add(1) == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"manifest version validation failed"}`))
				return
			}

			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"555"},"state":"active","hash":"new-hash"}}}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"555"},"state":"active","hash":"old-hash"}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	resp, err := console.New(srv.URL, "test-key").UpdateDeployment(
		context.Background(), "555", validUpdateSDL,
	)
	require.NoError(t, err)
	assert.Equal(t, "new-hash", resp.Deployment.Hash)
	assert.Equal(t, int32(2), puts.Load())
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
	tests := []struct {
		name   string
		status int
		body   string
	}{
		// 404 = resource gone = desired end state, regardless of body.
		{"404", http.StatusNotFound, `{"error":"no such deployment"}`},
		// 400 maps to the sentinel only when the body says so.
		{"400 already closed", http.StatusBadRequest, `{"error":"deployment already closed"}`},
		{"400 closed", http.StatusBadRequest, `{"error":"Deployment closed"}`},
		{"400 not found", http.StatusBadRequest, `{"error":"Deployment Not Found"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := console.New(srv.URL, "test-key")
			err := c.CloseDeployment(context.Background(), "1")
			require.Error(t, err)
			assert.ErrorIs(t, err, console.ErrAlreadyClosed,
				"HTTP %d %q must map to ErrAlreadyClosed for idempotent close", tt.status, tt.body)
		})
	}
}

func TestCloseDeploymentBadRequestValidationIsRealError(t *testing.T) {
	// The pinned contract documents only 200 for DELETE
	// /v1/deployments/{dseq}: a 400 whose body does not indicate
	// already-closed semantics is a genuine failure (validation, not-owner,
	// active leases) and must NOT be swallowed as ErrAlreadyClosed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid dseq parameter"}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	err := c.CloseDeployment(context.Background(), "1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, console.ErrAlreadyClosed,
		"a validation 400 must not be reported as already-closed")

	var httpErr *console.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
	assert.Contains(t, httpErr.Body, "invalid dseq parameter")
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

func TestCreateLeaseReconcilesFailedResponseWithoutReplayingPost(t *testing.T) {
	var posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			posts.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case http.MethodGet:
			assert.Equal(t, "/v1/deployments/888", r.URL.Path)
			_, _ = w.Write([]byte(`{"data":{
				"deployment":{"id":{"owner":"akash1x","dseq":"888"},"state":"active"},
				"leases":[{"id":{"owner":"akash1x","dseq":"888","gseq":1,"oseq":1,"provider":"akash1p"},"state":"active"}]
			}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	resp, err := console.New(srv.URL, "test-key").CreateLease(
		context.Background(), "manifest-json", []console.LeaseRequest{
			{DSeq: "888", GSeq: 1, OSeq: 1, Provider: "akash1p"},
		},
	)
	require.NoError(t, err)
	require.Len(t, resp.Leases, 1)
	assert.Equal(t, "active", resp.Leases[0].State)
	assert.Equal(t, int32(1), posts.Load(), "the non-idempotent POST must not be replayed")
}

func TestCreateLeaseKeepsOriginalErrorWhenReadBackDoesNotMatch(t *testing.T) {
	var posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			posts.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{
				"deployment":{"id":{"owner":"akash1x","dseq":"888"},"state":"open"},
				"leases":[]
			}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateLease(
		context.Background(), "manifest-json", []console.LeaseRequest{
			{DSeq: "888", GSeq: 1, OSeq: 1, Provider: "akash1p"},
		},
	)
	require.ErrorIs(t, err, console.ErrNotFound)
	assert.Equal(t, int32(1), posts.Load())
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
