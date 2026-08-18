package console_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		switch r.Method {
		case http.MethodGet:
			assert.Equal(t, "/v1/deployments", r.URL.Path)
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":1000,"hasMore":false}}}`))
		case http.MethodPost:
			assert.Equal(t, "/v1/deployments", r.URL.Path)
			data := decodeDataBody(t, r)
			assert.Equal(t, validUpdateSDL, data["sdl"])
			assert.InDelta(t, 5.0, data["deposit"], 0.001)

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"dseq":"12345","manifest":"[{\"name\":\"web\"}]","signTx":{"code":0,"transactionHash":"ABC123","rawLog":""}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.CreateDeployment(context.Background(), validUpdateSDL, 5.0)
	require.NoError(t, err)
	assert.Equal(t, "12345", resp.DSeq.String())
	assert.Equal(t, `[{"name":"web"}]`, resp.Manifest)
	require.NotNil(t, resp.SignTx)
	assert.Equal(t, "ABC123", resp.SignTx.TransactionHash)
}

func TestCreateDeploymentReconcilesAmbiguousResponseWithoutReplayingPost(t *testing.T) {
	expectedHash := versionHash(t, validUpdateSDL)
	var posts atomic.Int32
	var submitted atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if !submitted.Load() {
				_, _ = w.Write([]byte(`{"data":{"deployments":[{"deployment":{"id":{"owner":"akash1old","dseq":"100"},"hash":"old"},"leases":[]}],"pagination":{"total":1,"skip":0,"limit":1000,"hasMore":false}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"deployments":[` +
				`{"deployment":{"id":{"owner":"akash1old","dseq":"100"},"hash":"old"},"leases":[]},` +
				`{"deployment":{"id":{"owner":"akash1new","dseq":"12345"},"hash":"` + expectedHash + `"},"leases":[]}` +
				`],"pagination":{"total":2,"skip":0,"limit":1000,"hasMore":false}}}`))
		case http.MethodPost:
			posts.Add(1)
			submitted.Store(true)
			w.WriteHeader(http.StatusBadGateway)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	result, err := console.New(srv.URL, "test-key").CreateDeployment(
		context.Background(), validUpdateSDL, 5,
	)
	require.NoError(t, err)
	assert.Equal(t, "12345", result.DSeq.String())
	assert.NotEmpty(t, result.Manifest, "reconciled result must carry the locally derived manifest")
	assert.Equal(t, int32(1), posts.Load(), "deployment POST must be submitted exactly once")
}

func TestCreateDeploymentReconcilesMalformedReceiptWithoutReplayingPost(t *testing.T) {
	expectedHash := versionHash(t, validUpdateSDL)
	var posts atomic.Int32
	var submitted atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if !submitted.Load() {
				_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"hasMore":false}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"deployments":[{"deployment":{"id":{"dseq":"888"},"hash":"` + expectedHash + `"},"leases":[]}],"pagination":{"hasMore":false}}}`))
		case http.MethodPost:
			posts.Add(1)
			submitted.Store(true)
			_, _ = w.Write([]byte(`{"data":{"dseq":"777","manifest":"server-manifest","signTx":{"transactionHash":""}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	result, err := console.New(srv.URL, "test-key").CreateDeployment(
		context.Background(), validUpdateSDL, 5,
	)
	require.NoError(t, err)
	assert.Equal(t, "888", result.DSeq.String(), "malformed acknowledgement must be replaced by exact read-back")
	assert.Nil(t, result.SignTx)
	assert.Equal(t, int32(1), posts.Load(), "deployment POST must be submitted exactly once")
}

func TestCreateDeploymentAbortsBeforePostWhenSnapshotFails(t *testing.T) {
	var posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			_, _ = w.Write([]byte(`{"data":{"dseq":"duplicate"}}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateDeployment(
		context.Background(), validUpdateSDL, 5,
	)
	require.ErrorIs(t, err, console.ErrUnauthorized)
	assert.Zero(t, posts.Load(), "creation without a baseline cannot be reconciled safely")
}

func TestCreateDeploymentSnapshotsMultipleDeploymentPages(t *testing.T) {
	var gets atomic.Int32
	var posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets.Add(1)
			assert.Equal(t, "1000", r.URL.Query().Get("limit"))

			switch r.URL.Query().Get("skip") {
			case "0":
				_, _ = w.Write([]byte(`{"data":{"deployments":[{"deployment":{"id":{"dseq":"100"}}}],"pagination":{"skip":0,"limit":1,"hasMore":true}}}`))
			case "1":
				_, _ = w.Write([]byte(`{"data":{"deployments":[{"deployment":{"id":{"dseq":"101"}}}],"pagination":{"skip":1,"limit":1,"hasMore":false}}}`))
			default:
				t.Errorf("unexpected deployment page skip %q", r.URL.Query().Get("skip"))
				http.Error(w, "unexpected page", http.StatusBadRequest)
			}
		case http.MethodPost:
			posts.Add(1)
			_, _ = w.Write([]byte(`{"data":{"dseq":"12345","manifest":"manifest","signTx":{"code":0,"transactionHash":"tx-create"}}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	result, err := console.New(srv.URL, "test-key").CreateDeployment(
		context.Background(), validUpdateSDL, 5,
	)
	require.NoError(t, err)
	assert.Equal(t, "12345", result.DSeq.String())
	assert.Equal(t, int32(2), gets.Load())
	assert.Equal(t, int32(1), posts.Load())
}

func TestCreateDeploymentRejectsEndlessDeploymentPaginationBeforePost(t *testing.T) {
	const secret = "pagination-secret"

	var gets atomic.Int32
	var posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			page := gets.Add(1)
			skip := r.URL.Query().Get("skip")
			dseq := strconv.FormatInt(int64(page), 10)
			_, _ = w.Write([]byte(`{"data":{"deployments":[{"deployment":{"id":{"dseq":"` + dseq + `"},"hash":"` + secret + `"}}],"pagination":{"skip":` + skip + `,"limit":1,"hasMore":true}}}`))
		case http.MethodPost:
			posts.Add(1)
			http.Error(w, secret, http.StatusBadGateway)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, secret).CreateDeployment(
		context.Background(), validUpdateSDL, 5,
	)
	require.ErrorContains(t, err, "deployment pagination exceeded safety limit")
	assert.NotContains(t, err.Error(), secret)
	assert.Equal(t, int32(100), gets.Load(), "the page boundary must make an endless valid sequence deterministic")
	assert.Zero(t, posts.Load(), "an incomplete baseline cannot authorize deployment creation")
}

func TestCreateDeploymentRejectsDeploymentRecordLimitBeforePost(t *testing.T) {
	var posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			deployments := make([]console.DeploymentListItem, 10_001)
			for i := range deployments {
				deployments[i] = console.DeploymentListItem{
					Deployment: console.Deployment{
						ID: console.DeploymentID{
							Owner: "akash1owner",
							DSeq:  console.FlexString(strconv.Itoa(i + 1)),
						},
						State:     "active",
						Hash:      "version-hash",
						CreatedAt: json.RawMessage(`"1"`),
					},
					Leases: []console.Lease{},
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": console.DeploymentList{
					Deployments: deployments,
					Pagination:  console.Pagination{Skip: 0, Limit: 10_001, HasMore: false},
				},
			})
		case http.MethodPost:
			posts.Add(1)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateDeployment(
		context.Background(), validUpdateSDL, 5,
	)
	require.ErrorContains(t, err, "deployment pagination exceeded safety limit")
	assert.Zero(t, posts.Load(), "an incomplete baseline cannot authorize deployment creation")
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

func TestListDeploymentsByStateFiltersBeforeApplyingPageWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("skip") {
		case "0":
			_, _ = w.Write([]byte(`{"data":{"deployments":[
				{"deployment":{"id":{"dseq":"1"},"state":"active"}},
				{"deployment":{"id":{"dseq":"2"},"state":"closed"}}
			],"pagination":{"skip":0,"limit":2,"hasMore":true}}}`))
		case "2":
			_, _ = w.Write([]byte(`{"data":{"deployments":[
				{"deployment":{"id":{"dseq":"3"},"state":"active"}},
				{"deployment":{"id":{"dseq":"4"},"state":"active"}}
			],"pagination":{"skip":2,"limit":2,"hasMore":false}}}`))
		default:
			t.Fatalf("unexpected pagination request %s", r.URL.RawQuery)
		}
	}))
	defer srv.Close()

	result, err := console.New(srv.URL, "test-key").ListDeploymentsByState(
		context.Background(), "active", 1, 1,
	)
	require.NoError(t, err)
	require.Len(t, result.Deployments, 1)
	assert.Equal(t, "3", result.Deployments[0].Deployment.ID.DSeq.String())
	assert.Equal(t, 3, result.Pagination.Total)
	assert.True(t, result.Pagination.HasMore)
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
	expectedHash := versionHash(t, validUpdateSDL)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			assert.Equal(t, "/v1/deployments/555", r.URL.Path)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"555"},"state":"active"}}}`))
			return
		}
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/v1/deployments/555", r.URL.Path)

		data := decodeDataBody(t, r)
		assert.Equal(t, validUpdateSDL, data["sdl"])

		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"555"},"state":"active","hash":"` + expectedHash + `"}}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.UpdateDeployment(context.Background(), "555", validUpdateSDL)
	require.NoError(t, err)
	assert.Equal(t, "555", resp.Deployment.ID.DSeq.String())
	assert.Equal(t, expectedHash, resp.Deployment.Hash)
}

func TestUpdateDeploymentRejectsInvalidSDLBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").UpdateDeployment(
		context.Background(), "555", "not-valid-sdl: [",
	)
	require.ErrorContains(t, err, "prepare deployment SDL")
	assert.Equal(t, int32(0), requests.Load(), "invalid SDL must fail before an update is submitted")
}

func TestUpdateDeploymentReconcilesMalformedAcknowledgement(t *testing.T) {
	expectedHash := versionHash(t, validUpdateSDL)
	var puts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			puts.Add(1)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"999"},"hash":"wrong"}}}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"555"},"hash":"` + expectedHash + `"},"leases":[]}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	got, err := console.New(srv.URL, "test-key").UpdateDeployment(context.Background(), "555", validUpdateSDL)
	require.NoError(t, err)
	assert.Equal(t, "555", got.Deployment.ID.DSeq.String())
	assert.Equal(t, expectedHash, got.Deployment.Hash)
	assert.Equal(t, int32(1), puts.Load(), "malformed success must be reconciled before any replay")
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
	expectedHash := versionHash(t, validUpdateSDL)
	var puts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if puts.Add(1) == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"manifest version validation failed"}`))
				return
			}

			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"555"},"state":"active","hash":"` + expectedHash + `"}}}`))
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
	assert.Equal(t, expectedHash, resp.Deployment.Hash)
	assert.Equal(t, int32(2), puts.Load())
}

func TestCloseDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v1/deployments/99999", r.URL.Path)

		_, _ = w.Write([]byte(`{"data":{"success":true}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	require.NoError(t, c.CloseDeployment(context.Background(), "99999"))
}

func TestCloseDeploymentRejectsMissingOrFalseSuccess(t *testing.T) {
	for _, body := range []string{
		`{"data":{}}`,
		`{"data":{"success":false}}`,
	} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			err := console.New(srv.URL, "test-key").CloseDeployment(context.Background(), "1")
			require.ErrorContains(t, err, "success: true")
		})
	}
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

func TestCloseDeploymentCannotBeClosedIsRealError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"deployment cannot be closed while active leases exist"}`))
	}))
	defer srv.Close()

	err := console.New(srv.URL, "test-key").CloseDeployment(context.Background(), "1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, console.ErrAlreadyClosed)
}

func TestDepositBodyShape(t *testing.T) {
	var gets atomic.Int32
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets.Add(1)
			amount := "1000000"
			if posts.Load() > 0 {
				amount = "13500000"
			}
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"321"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"` + amount + `"}],"transferred":[]}}}}`))
		case http.MethodPost:
			posts.Add(1)
			assert.Equal(t, "/v1/deposit-deployment", r.URL.Path)

			data := decodeDataBody(t, r)
			assert.Equal(t, "321", data["dseq"])
			assert.InDelta(t, 12.5, data["deposit"], 0.001)
			assert.NotContains(t, data, "amount", "wire field is deposit, not amount")

			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"321"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"13500000"}],"transferred":[]}}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	require.NoError(t, c.Deposit(context.Background(), "321", 12.5))
	assert.Equal(t, int32(1), gets.Load(), "an exact returned post-state must avoid a redundant read-back")
	assert.Equal(t, int32(1), posts.Load())
}

func TestDepositRejectsInvalidAmountBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer srv.Close()

	err := console.New(srv.URL, "test-key").Deposit(context.Background(), "321", 0)
	require.ErrorContains(t, err, "at least $0.50")
	assert.Equal(t, int32(0), requests.Load(), "invalid amount must not read or mutate remote state")
}

func TestDepositRequiresAuthoritativePreStateBeforePost(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantDetail string
	}{
		{
			name:       "deployment lookup fails",
			status:     http.StatusNotFound,
			body:       `{"message":"deployment missing"}`,
			wantDetail: "snapshot deployment escrow before deposit",
		},
		{
			name:       "deployment identity differs",
			status:     http.StatusOK,
			body:       `{"data":{"deployment":{"id":{"dseq":"999"}},"escrow_account":{"state":{"funds":[],"transferred":[]}}}}`,
			wantDetail: `pre-state returned dseq "999", want "321"`,
		},
		{
			name:       "escrow is omitted",
			status:     http.StatusOK,
			body:       `{"data":{"deployment":{"id":{"dseq":"321"}}}}`,
			wantDetail: "omitted its escrow account",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gets, posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				gets.Add(1)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			err := console.New(srv.URL, "test-key").Deposit(context.Background(), "321", 1)
			require.ErrorContains(t, err, tt.wantDetail)
			assert.Equal(t, int32(1), gets.Load())
			assert.Equal(t, int32(0), posts.Load(), "untrusted pre-state must prevent the deposit POST")
		})
	}
}

func TestDepositAcceptsExactReturnedPostState(t *testing.T) {
	var gets, posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets.Add(1)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"500000.000000000000000000"}],"transferred":[]}}}}`))
		case http.MethodPost:
			posts.Add(1)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"1000000.000000000000000000"}],"transferred":[]}}}}`))
		default:
			t.Fatalf("unexpected request %s", r.Method)
		}
	}))
	defer srv.Close()

	err := console.New(srv.URL, "test-key").Deposit(context.Background(), "42", 0.5)
	require.NoError(t, err)
	assert.Equal(t, int32(1), gets.Load(), "exact acknowledgement must be semantic proof without a read-back")
	assert.Equal(t, int32(1), posts.Load())
}

func TestDepositStaleAcknowledgementTimesOutWithoutReplaying(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var gets, posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets.Add(1)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"1000000"}],"transferred":[]}}}}`))
			if posts.Load() > 0 {
				cancel()
			}
		case http.MethodPost:
			posts.Add(1)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"1000000"}],"transferred":[]}}}}`))
		default:
			t.Fatalf("unexpected request %s", r.Method)
		}
	}))
	defer srv.Close()

	err := console.New(srv.URL, "test-key").Deposit(ctx, "42", 1)
	require.ErrorContains(t, err, "outcome unknown")
	assert.Equal(t, int32(1), posts.Load(), "stale acknowledgement must not cause a replay")
	assert.GreaterOrEqual(t, gets.Load(), int32(2), "deposit must inspect post-state")
}

func TestDepositReconcilesUnusableAcknowledgementWithoutReplayingPost(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{
			name: "wrong deployment identity",
			body: `{"data":{"deployment":{"id":{"dseq":"999"}}}}`,
		},
		{
			name: "missing escrow state",
			body: `{"data":{"deployment":{"id":{"dseq":"42"}}}}`,
		},
		{
			name: "stale escrow state",
			body: `{"data":{"deployment":{"id":{"dseq":"42"}},"escrow_account":{"state":{"funds":{"denom":"uact","amount":"500000"},"transferred":[]}}}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var gets atomic.Int32
			var posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					gets.Add(1)
					amount := "500000"
					if posts.Load() > 0 {
						amount = "1500000"
					}
					_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[],"escrow_account":{"state":{"funds":{"denom":"uact","amount":"` + amount + `"},"transferred":[]}}}}`))
				case http.MethodPost:
					posts.Add(1)
					_, _ = w.Write([]byte(tt.body))
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
			}))
			defer srv.Close()

			err := console.New(srv.URL, "test-key").Deposit(context.Background(), "42", 1)
			require.NoError(t, err)
			assert.Equal(t, int32(2), gets.Load())
			assert.Equal(t, int32(1), posts.Load())
		})
	}
}

func TestDepositReturnedPostStateIncludesCumulativeTransferredDuringSettlement(t *testing.T) {
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			funds, transferred := "5000000.100000000000000001", "999999.899999999999999999"
			if posts.Load() > 0 {
				// While the $1 deposit lands, an active lease settles $1.5:
				// current funds fall by $0.5, but total escrow value rises by
				// exactly the requested $1 when cumulative transfers are included.
				// Fractional endpoints prove reconciliation retains the chain's
				// fixed-point values instead of rounding each coin to micro units.
				funds, transferred = "4500000.200000000000000001", "2499999.799999999999999999"
			}
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"` + funds + `"}],"transferred":[{"denom":"uact","amount":"` + transferred + `"}]}}}}`))
		case http.MethodPost:
			posts.Add(1)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"4500000.200000000000000001"}],"transferred":[{"denom":"uact","amount":"2499999.799999999999999999"}]}}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	err := console.New(srv.URL, "test-key").Deposit(context.Background(), "42", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(1), posts.Load(), "deposit POST must never be replayed")
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

func TestCreateLeaseUnresolvedServerErrorIsOutcomeUnknown(t *testing.T) {
	var posts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			posts.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"888"}},"leases":[]}}`))
			cancel()
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateLease(ctx, "manifest", []console.LeaseRequest{{
		DSeq: "888", GSeq: 1, OSeq: 1, Provider: "akash1p",
	}})
	require.ErrorContains(t, err, "outcome unknown")
	assert.Equal(t, int32(1), posts.Load())
}

func TestCreateLeaseReconcilesMalformedSuccessWithoutReplayingPost(t *testing.T) {
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			posts.Add(1)
			_, _ = w.Write([]byte(`{"data":{}}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"888"},"state":"active"},"leases":[{"id":{"owner":"akash1x","dseq":"888","gseq":1,"oseq":1,"provider":"akash1p"},"state":"active"}]}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	got, err := console.New(srv.URL, "test-key").CreateLease(context.Background(), "manifest", []console.LeaseRequest{{
		DSeq: "888", GSeq: 1, OSeq: 1, Provider: "akash1p",
	}})
	require.NoError(t, err)
	require.Len(t, got.Leases, 1)
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
		if r.Method == http.MethodGet {
			assert.Equal(t, "/v1/deployments/321", r.URL.Path)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"321"},"state":"active"}}}`))
			return
		}
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
		case http.MethodGet:
			assert.Equal(t, "/v1/deployments/654", r.URL.Path)
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"654"},"state":"active"}}}`))
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

func TestSetDeploymentAutoTopUpRejectsMissingOrMismatchedAcknowledgement(t *testing.T) {
	for _, body := range []string{
		`{"data":{}}`,
		`{"data":{"dseq":"321","autoTopUpEnabled":false}}`,
		`{"data":{"dseq":"999","autoTopUpEnabled":true}}`,
	} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			_, err := console.New(srv.URL, "test-key").SetDeploymentAutoTopUp(context.Background(), "321", true)
			require.ErrorContains(t, err, "did not echo")
		})
	}
}
