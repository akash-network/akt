package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/console"
)

func openProjectTestLog(t *testing.T) (*actionlog.Logger, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "actions.log")
	logger, err := actionlog.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = logger.Close() })

	return logger, path
}

func loggedEntries(t *testing.T, path string) []actionlog.Entry {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var entries []actionlog.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var entry actionlog.Entry
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		entries = append(entries, entry)
	}

	return entries
}

func TestListOrganizationsMarksTheActiveOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/organizations", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"id":"` + testOrganizationID + `","name":"Acme","slug":"acme","type":"team","role":"member","isActive":true,"hasPaymentMethod":true,"createdAt":"2026-10-01T00:00:00Z"}]}`))
	}))
	defer srv.Close()

	organization, err := console.New(srv.URL, "test-key").ActiveOrganization(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "acme", organization.Slug)
	assert.Equal(t, "member", organization.Role)
	assert.Equal(t, "team", organization.Type)
}

func TestActiveOrganizationRequiresAnActiveEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"` + testOrganizationID + `","name":"Acme","slug":"acme","type":"team","role":"owner","isActive":false}]}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").ActiveOrganization(context.Background())
	require.ErrorContains(t, err, "marks no organization as the active one")
}

func TestTeamRoutesAnswering404MeanTeamsAreNotEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cl := console.New(srv.URL, "test-key")
	_, err := cl.ListOrganizations(context.Background())
	assert.ErrorIs(t, err, console.ErrTeamsUnavailable)
	_, err = cl.ListProjects(context.Background())
	assert.ErrorIs(t, err, console.ErrTeamsUnavailable)
	_, err = cl.CreateProject(context.Background(), "web", "")
	assert.ErrorIs(t, err, console.ErrTeamsUnavailable)
}

func TestCreateProjectSendsNameAndDescriptionAndLogs(t *testing.T) {
	logger, path := openProjectTestLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/projects", r.URL.Path)
		data := decodeDataBody(t, r)
		assert.Equal(t, "checkout-api", data["name"])
		assert.Equal(t, "Payments", data["description"])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"` + testProjectID + `","name":"checkout-api","slug":"checkout-api","description":"Payments","isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null}}`))
	}))
	defer srv.Close()

	project, err := console.New(srv.URL, "test-key").WithActionLog(logger).CreateProject(context.Background(), " checkout-api ", " Payments ")
	require.NoError(t, err)
	assert.Equal(t, testProjectID, project.ID)

	entries := loggedEntries(t, path)
	require.Len(t, entries, 1)
	assert.Equal(t, "create-project", entries[0].Action)
	assert.Equal(t, "success", entries[0].Status)
	assert.Contains(t, string(entries[0].Params), testProjectID)
}

func TestCreateProjectOmitsAnEmptyDescription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NotContains(t, decodeDataBody(t, r), "description")
		_, _ = w.Write([]byte(`{"data":{"id":"` + testProjectID + `","name":"web","slug":"web","description":null,"isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null}}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateProject(context.Background(), "web", "")
	require.NoError(t, err)
}

func TestProjectInputIsValidatedBeforeTheNetwork(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer srv.Close()

	logger, path := openProjectTestLog(t)
	cl := console.New(srv.URL, "test-key").WithActionLog(logger)
	ctx := context.Background()

	_, err := cl.CreateProject(ctx, "  ", "")
	require.ErrorContains(t, err, "name must not be blank")
	_, err = cl.CreateProject(ctx, strings.Repeat("n", console.MaxProjectNameLength+1), "")
	require.ErrorContains(t, err, "at most 64 characters")
	_, err = cl.CreateProject(ctx, "web", strings.Repeat("d", console.MaxProjectDescriptionLength+1))
	require.ErrorContains(t, err, "at most 140 characters")
	_, err = cl.RenameProject(ctx, "not-a-uuid", "web")
	require.ErrorContains(t, err, "project id must be a UUID")
	_, err = cl.RenameProject(ctx, testProjectID, "")
	require.ErrorContains(t, err, "name must not be blank")
	require.ErrorContains(t, cl.DeleteProject(ctx, "web"), "project id must be a UUID")
	_, err = cl.MoveDeployment(ctx, "0", testProjectID)
	require.Error(t, err)
	_, err = cl.MoveDeployment(ctx, "42", "web")
	require.ErrorContains(t, err, "project id must be a UUID")

	assert.Equal(t, int32(0), requests.Load())
	for _, entry := range loggedEntries(t, path) {
		assert.Equal(t, "failed", entry.Status, entry.Action)
	}
	assert.Len(t, loggedEntries(t, path), 8)
}

func TestAmbiguousProjectCreateIsLoggedPending(t *testing.T) {
	logger, path := openProjectTestLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").WithActionLog(logger).CreateProject(context.Background(), "web", "")
	require.ErrorContains(t, err, "outcome unknown")
	require.ErrorContains(t, err, "akt console project list")

	entries := loggedEntries(t, path)
	require.Len(t, entries, 1)
	assert.Equal(t, "pending", entries[0].Status)
}

func TestRenameAndDeleteProject(t *testing.T) {
	logger, path := openProjectTestLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/projects/"+testProjectID, r.URL.Path)
		switch r.Method {
		case http.MethodPatch:
			assert.Equal(t, map[string]any{"name": "payments"}, decodeDataBody(t, r))
			_, _ = w.Write([]byte(`{"data":{"id":"` + testProjectID + `","name":"payments","slug":"checkout-api","description":null,"isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null}}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	cl := console.New(srv.URL, "test-key").WithActionLog(logger)
	project, err := cl.RenameProject(context.Background(), strings.ToUpper(testProjectID), "payments")
	require.NoError(t, err)
	assert.Equal(t, "checkout-api", project.Slug, "a rename keeps the slug")
	require.NoError(t, cl.DeleteProject(context.Background(), testProjectID))

	entries := loggedEntries(t, path)
	require.Len(t, entries, 2)
	assert.Equal(t, "rename-project", entries[0].Action)
	assert.Equal(t, "delete-project", entries[1].Action)
	assert.Equal(t, "success", entries[1].Status)
}

func TestRenameProjectRejectsAnUnechoedName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"` + testProjectID + `","name":"old","slug":"old","description":null,"isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null}}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").RenameProject(context.Background(), testProjectID, "new")
	require.ErrorContains(t, err, `named "old" instead of "new"`)
}

func TestDeleteDefaultProjectExplainsTheRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"ConflictError","message":"The default project cannot be deleted","code":"project_is_default","type":"client_error"}`))
	}))
	defer srv.Close()

	err := console.New(srv.URL, "test-key").DeleteProject(context.Background(), testProjectID)
	require.ErrorContains(t, err, "The default project cannot be deleted (project_is_default, HTTP 409)")
}

func TestMoveDeploymentDropsTheProjectHeader(t *testing.T) {
	logger, path := openProjectTestLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/v1/deployments/42/project", r.URL.Path)
		assert.Empty(t, r.Header.Get(console.HeaderProjectID), "a request narrowed to the source project could not reach the target")
		assert.Equal(t, "acme", r.Header.Get(console.HeaderOrganizationID))
		assert.Equal(t, map[string]any{"projectId": otherProjectID}, decodeDataBody(t, r))
		_, _ = w.Write([]byte(`{"data":{"dseq":"42","projectId":"` + otherProjectID + `"}}`))
	}))
	defer srv.Close()

	moved, err := console.New(srv.URL, "test-key").WithActionLog(logger).WithScope("acme", testProjectID).
		MoveDeployment(context.Background(), "42", otherProjectID)
	require.NoError(t, err)
	assert.Equal(t, "42", moved.DSeq.String())

	entries := loggedEntries(t, path)
	require.Len(t, entries, 1)
	assert.Equal(t, "move-deployment", entries[0].Action)
	assert.Equal(t, uint64(42), entries[0].DSeq)
}

func TestMoveDeploymentRejectsAMismatchedAcknowledgement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"dseq":"42","projectId":"` + testProjectID + `"}}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").MoveDeployment(context.Background(), "42", otherProjectID)
	require.ErrorContains(t, err, "move response reported")
}

func TestCreateAPIKeyForProjectBindsTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := decodeDataBody(t, r)
		assert.Equal(t, testProjectID, data["projectId"])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"key-1","name":"ci","apiKey":"ak-secret","organizationId":"` + testOrganizationID + `","projectId":"` + testProjectID + `"}}`))
	}))
	defer srv.Close()

	created, err := console.New(srv.URL, "test-key").CreateAPIKeyForProject(context.Background(), "ci", "", strings.ToUpper(testProjectID))
	require.NoError(t, err)
	require.NotNil(t, created.ProjectID)
	assert.Equal(t, testProjectID, *created.ProjectID)

	_, err = console.New(srv.URL, "test-key").CreateAPIKeyForProject(context.Background(), "ci", "", "checkout-api")
	require.ErrorContains(t, err, "project id must be a UUID")
}

func TestListAPIKeysReportsTheirOrganizationAndProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"key-1","name":"team","organizationId":"` + testOrganizationID + `","projectId":null},
			{"id":"key-2","name":"project","organizationId":"` + testOrganizationID + `","projectId":"` + testProjectID + `"},
			{"id":"key-3","name":"legacy","organizationId":null,"projectId":null}
		]}`))
	}))
	defer srv.Close()

	keys, err := console.New(srv.URL, "test-key").ListAPIKeys(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 3)
	require.NotNil(t, keys[1].ProjectID)
	assert.Equal(t, testProjectID, *keys[1].ProjectID)

	out, err := json.Marshal(keys)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "null", "keys without a team or project keep their previous shape")
}

func TestDeploymentJWTFallsBackToADeploymentGrantOnProjectForbidden(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := decodeDataBody(t, r)
		bodies = append(bodies, data)
		leases := data["leases"].(map[string]any)
		if leases["access"] == "scoped" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Name the deployments the token is for","code":"project_forbidden"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"token":"granular-token"}}`))
	}))
	defer srv.Close()

	token, err := console.New(srv.URL, "test-key").CreateDeploymentJWTToken(context.Background(), 300, "akash1provider", "42", []string{"logs", "status"})
	require.NoError(t, err)
	assert.Equal(t, "granular-token", token)
	require.Len(t, bodies, 2)

	granular, err := json.Marshal(bodies[1]["leases"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"access":"granular","permissions":[{"provider":"akash1provider","access":"granular","deployments":[{"dseq":42,"scope":["logs","status"],"services":["*"]}]}]}`, string(granular))
}

func TestDeploymentJWTWithASelectedProjectNamesTheDeploymentAtOnce(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		leases := decodeDataBody(t, r)["leases"].(map[string]any)
		assert.Equal(t, "granular", leases["access"])
		assert.Equal(t, testProjectID, r.Header.Get(console.HeaderProjectID))
		_, _ = w.Write([]byte(`{"data":{"token":"t"}}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").WithScope("", testProjectID).
		CreateDeploymentJWTToken(context.Background(), 3600, "akash1provider", "42", []string{"shell", "status"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), requests.Load())
}

func TestDeploymentJWTKeepsTheScopedTokenWithoutTeams(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		leases := decodeDataBody(t, r)["leases"].(map[string]any)
		assert.Equal(t, "scoped", leases["access"])
		_, _ = w.Write([]byte(`{"data":{"token":"t"}}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateDeploymentJWTToken(context.Background(), 300, "akash1provider", "42", []string{"status"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), requests.Load())
}

func TestDeploymentJWTSurfacesOtherRefusalsWithoutRetrying(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"ttl must be at most 3600 seconds","code":"bad_request"}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateDeploymentJWTToken(context.Background(), 300, "akash1provider", "42", []string{"status"})
	var httpErr *console.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, int32(1), requests.Load())

	for _, bad := range []struct{ provider, dseq string }{{"", "42"}, {"akash1p", "0"}, {"akash1p", "x"}} {
		_, err := console.New(srv.URL, "test-key").CreateDeploymentJWTToken(context.Background(), 300, bad.provider, bad.dseq, []string{"status"})
		require.Error(t, err)
	}
	_, err = console.New(srv.URL, "test-key").WithScope("", testProjectID).CreateDeploymentJWTToken(context.Background(), 7200, "akash1p", "42", []string{"status"})
	require.ErrorContains(t, err, "between 1 and 3600")
	assert.Equal(t, int32(1), requests.Load())
}
