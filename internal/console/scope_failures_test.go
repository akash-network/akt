package console_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

func newProjectLookupServer(t *testing.T, projects func(w http.ResponseWriter)) (*httptest.Server, func() []string) {
	t.Helper()

	var (
		mu    sync.Mutex
		other []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/projects" {
			projects(w)
			return
		}
		mu.Lock()
		other = append(other, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), other...)
	}
}

func refuseProjectListing(w http.ResponseWriter) {
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"ForbiddenError","message":"Forbidden","code":"forbidden","type":"client_error"}`))
}

func listTwoProjects(w http.ResponseWriter) {
	_, _ = w.Write([]byte(projectsBody()))
}

func TestAnUnresolvableProjectStopsTheRequest(t *testing.T) {
	srv, otherCalls := newProjectLookupServer(t, refuseProjectListing)

	_, err := console.New(srv.URL, "test-key").WithScope("", "checkout").GetUser(context.Background())
	require.ErrorContains(t, err, `resolve project "checkout"`)
	assert.Empty(t, otherCalls(), "no request may run in an unresolved project")
}

func TestCreateDeploymentStopsBeforeAnyCallOnAnUnresolvableProject(t *testing.T) {
	tests := []struct {
		name    string
		options []console.CreateDeploymentOptions
	}{
		{name: "legacy"},
		{name: "sealed", options: []console.CreateDeploymentOptions{{Secrets: console.SecretValues{}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, otherCalls := newProjectLookupServer(t, listTwoProjects)

			_, err := console.New(srv.URL, "test-key").WithScope("", "missing").CreateDeployment(context.Background(), validUpdateSDL, tc.options...)
			require.ErrorContains(t, err, `no project named "missing"`)
			assert.Empty(t, otherCalls(), "neither the snapshot, the sealing context, nor the create may run")
		})
	}
}

func TestFindProjectReportsAFailedListing(t *testing.T) {
	srv, _ := newProjectLookupServer(t, refuseProjectListing)

	_, err := console.New(srv.URL, "test-key").FindProject(context.Background(), "checkout-api")
	var refusal *console.HTTPError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, http.StatusForbidden, refusal.StatusCode)
}

func TestCreateProjectRejectsAResponseWithoutAnID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":" ","name":"payments","slug":"payments","description":null,"isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null}}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateProject(context.Background(), "payments", "")
	require.ErrorContains(t, err, "project response omitted its id")
}

func TestMatchProjectTreatsAUUIDShapedNameAsAName(t *testing.T) {
	var projects []console.ListedProject
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(projectsBody(), `{"data":`), "}")), &projects))

	_, err := console.MatchProject(projects, "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz")
	require.ErrorContains(t, err, `no project named "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"`)
}
