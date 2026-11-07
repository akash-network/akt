package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

const (
	testOrganizationID = "0b8052e2-6c1b-4b9a-9f3e-1d2c3b4a5f60"
	testProjectID      = "7d4c3b2a-1e0f-4a9b-8c7d-6e5f4a3b2c1d"
	otherProjectID     = "3a2b1c0d-9e8f-4a7b-8c6d-5e4f3a2b1c0d"
)

func projectsBody() string {
	return `{"data":[
		{"id":"` + testProjectID + `","name":"Checkout API","slug":"checkout-api","description":null,"isDefault":false,
		 "createdAt":"2026-10-01T00:00:00Z","createdBy":null,"lastActivityAt":"2026-10-02T00:00:00Z",
		 "deployments":{"activeCount":1,"preview":[{"dseq":"42","name":"web","status":"running"}]},
		 "access":{"canDeploy":true,"canManage":false}},
		{"id":"` + otherProjectID + `","name":"default","slug":"default","description":null,"isDefault":true,
		 "createdAt":"2026-09-01T00:00:00Z","createdBy":null,"lastActivityAt":"2026-09-02T00:00:00Z",
		 "deployments":null,"access":{"canDeploy":true,"canManage":true}}
	]}`
}

func TestClientWithoutScopeSendsNoTeamHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, orgSent := r.Header[http.CanonicalHeaderKey(console.HeaderOrganizationID)]
		_, projectSent := r.Header[http.CanonicalHeaderKey(console.HeaderProjectID)]
		assert.False(t, orgSent, "an account without teams must not send x-organization-id")
		assert.False(t, projectSent, "an account without teams must not send x-project-id")
		assert.NotContains(t, r.URL.RawQuery, "projectId")
		_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":20,"hasMore":false}}}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").WithScope(" ", "").ListDeployments(context.Background(), 0, 20)
	require.NoError(t, err)
}

func TestScopedClientSendsOrganizationAndProjectHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "acme", r.Header.Get(console.HeaderOrganizationID))
		assert.Equal(t, testProjectID, r.Header.Get(console.HeaderProjectID))
		assert.Equal(t, testProjectID, r.URL.Query().Get("projectId"), "listing by project also filters explicitly")
		_, _ = w.Write([]byte(`{"data":{"deployments":[{"deployment":{"id":{"owner":"akash1x","dseq":"42"},"state":"active"},"leases":[],"projectId":"` + testProjectID + `"}],"pagination":{"total":1,"skip":0,"limit":20,"hasMore":false}}}`))
	}))
	defer srv.Close()

	list, err := console.New(srv.URL, "test-key").
		WithScope("acme", strings.ToUpper(testProjectID)).
		ListDeployments(context.Background(), 0, 20)
	require.NoError(t, err)
	require.Len(t, list.Deployments, 1)
	require.NotNil(t, list.Deployments[0].ProjectID)
	assert.Equal(t, testProjectID, *list.Deployments[0].ProjectID)
}

func TestProjectNameResolvesOnceWithoutTheProjectHeader(t *testing.T) {
	var listings atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/projects" {
			listings.Add(1)
			assert.Empty(t, r.Header.Get(console.HeaderProjectID), "resolving a project must not be narrowed to it")
			assert.Equal(t, "acme", r.Header.Get(console.HeaderOrganizationID))
			_, _ = w.Write([]byte(projectsBody()))
			return
		}
		assert.Equal(t, testProjectID, r.Header.Get(console.HeaderProjectID))
		_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":20,"hasMore":false}}}`))
	}))
	defer srv.Close()

	cl := console.New(srv.URL, "test-key").WithScope("acme", "checkout")
	_, err := cl.ListDeployments(context.Background(), 0, 20)
	require.Error(t, err, "a name that matches nothing must fail rather than act across every project")

	cl = console.New(srv.URL, "test-key").WithScope("acme", "CHECKOUT-API")
	for range 2 {
		_, err := cl.ListDeployments(context.Background(), 0, 20)
		require.NoError(t, err)
	}
	assert.Equal(t, int32(2), listings.Load(), "one listing per client: the failed lookup and the cached slug")
}

func TestMatchProjectSelectors(t *testing.T) {
	var projects []console.ListedProject
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(projectsBody(), `{"data":`), "}")), &projects))

	tests := []struct {
		selector string
		wantID   string
		wantErr  string
	}{
		{selector: testProjectID, wantID: testProjectID},
		{selector: strings.ToUpper(otherProjectID), wantID: otherProjectID},
		{selector: "checkout-api", wantID: testProjectID},
		{selector: "Checkout API", wantID: testProjectID},
		{selector: " DEFAULT ", wantID: otherProjectID},
		{selector: "missing", wantErr: `no project named "missing"`},
		{selector: "11111111-1111-4111-8111-111111111111", wantErr: "no project with id"},
	}
	for _, tc := range tests {
		t.Run(tc.selector, func(t *testing.T) {
			project, err := console.MatchProject(projects, tc.selector)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantID, project.ID)
		})
	}

	ambiguous := append(projects, console.ListedProject{Project: console.Project{ID: "aaaaaaaa-1111-4111-8111-111111111111", Name: "checkout-api", Slug: "checkout-api-2"}})
	_, err := console.MatchProject(ambiguous, "checkout-api")
	require.ErrorContains(t, err, "names more than one project")
}

func TestForProjectLeavesTheReceiverUnchanged(t *testing.T) {
	base := console.New("", "k").WithScope("acme", testProjectID)
	other := base.ForProject(otherProjectID)
	cleared := base.ForProject("")

	assert.Equal(t, testProjectID, base.Project())
	assert.Equal(t, otherProjectID, other.Project())
	assert.Equal(t, "acme", other.Organization())
	assert.Empty(t, cleared.Project())
	assert.Equal(t, "acme", cleared.Organization())
	assert.Empty(t, console.New("", "k").ForProject("").Organization())
}

func TestCreateDeploymentFilesTheSelectedProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, testProjectID, r.Header.Get(console.HeaderProjectID))
		switch r.Method {
		case http.MethodGet:
			assert.Equal(t, testProjectID, r.URL.Query().Get("projectId"))
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":100,"hasMore":false}}}`))
		case http.MethodPost:
			data := decodeDataBody(t, r)
			assert.Equal(t, testProjectID, data["projectId"])
			_, _ = w.Write([]byte(`{"data":{"dseq":"12345","manifest":"[]","signTx":{"code":0,"transactionHash":"ABC","rawLog":""}}}`))
		}
	}))
	defer srv.Close()

	result, err := console.New(srv.URL, "test-key").WithScope("", testProjectID).CreateDeployment(context.Background(), validUpdateSDL)
	require.NoError(t, err)
	assert.Equal(t, "12345", result.DSeq.String())
}

func TestCreateDeploymentWithoutProjectOmitsProjectID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			assert.NotContains(t, decodeDataBody(t, r), "projectId")
			_, _ = w.Write([]byte(`{"data":{"dseq":"12345","manifest":"[]","signTx":{"code":0,"transactionHash":"ABC","rawLog":""}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":100,"hasMore":false}}}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateDeployment(context.Background(), validUpdateSDL)
	require.NoError(t, err)
}

func TestTeamRefusalsExplainTheirRemedy(t *testing.T) {
	tests := []struct {
		code       string
		status     int
		message    string
		wantInText []string
	}{
		{console.CodeProjectRequired, http.StatusBadRequest, "Choose a project to deploy into", []string{"Choose a project to deploy into (project_required, HTTP 400)", "--console-project", "AKT_CONSOLE_PROJECT", "akt console project use"}},
		{console.CodeProjectForbidden, http.StatusForbidden, "The project is not reachable in this organization", []string{"(project_forbidden, HTTP 403)", "akt console project list"}},
		{console.CodeOrganizationMismatch, http.StatusBadRequest, "The organization header names another organization than the API key's", []string{"(organization_mismatch, HTTP 400)", "--console-organization"}},
		{console.CodeOrganizationForbidden, http.StatusForbidden, "Not a member of this organization", []string{"Not a member of this organization (organization_forbidden, HTTP 403)"}},
		{console.CodeProjectBudgetExceeded, http.StatusForbidden, "This would take the project past its monthly budget, so it was not signed. An owner or admin can raise the budget.", []string{"(project_budget_exceeded, HTTP 403)", "monthly budget"}},
		{console.CodeSessionRequired, http.StatusForbidden, "", []string{"needs a signed-in Console session (session_required, HTTP 403)", console.DefaultUIBaseURL}},
		{console.CodeProjectNotEmpty, http.StatusConflict, "Move or close the project's deployments before deleting it", []string{"(project_not_empty, HTTP 409)", "akt console deployment move"}},
		{console.CodeProjectLimitReached, http.StatusForbidden, "The organization has reached the number of projects it can have", []string{"(project_limit_reached, HTTP 403)", "akt console project delete", "default project counts toward the limit"}},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				body, _ := json.Marshal(map[string]any{"error": "Error", "message": tc.message, "code": tc.code, "type": "client_error"})
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			_, err := console.New(srv.URL, "test-key").GetUser(context.Background())
			require.Error(t, err)
			var httpErr *console.HTTPError
			require.True(t, errors.As(err, &httpErr))
			assert.Equal(t, tc.code, httpErr.Code)
			for _, want := range tc.wantInText {
				assert.Contains(t, err.Error(), want)
			}
			assert.NotContains(t, err.Error(), "unexpected status")
		})
	}
}

func TestUnrecognizedRefusalKeepsTheRawBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"ForbiddenError","message":"Forbidden","code":"forbidden"}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").GetUser(context.Background())
	var httpErr *console.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Empty(t, httpErr.Code)
	assert.Contains(t, err.Error(), "unexpected status 403")
}

func TestRefusalDetailIsBoundedAndCleaned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		body, _ := json.Marshal(map[string]any{"message": "line\x1b[31m" + strings.Repeat("x", 400), "code": console.CodeProjectForbidden})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").GetUser(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "\x1b")
	assert.Contains(t, err.Error(), "[truncated]")
}

func TestSealedCreateKeepsTheTeamRefusalCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sdl-secrets-context":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Choose a project to deploy into","code":"project_required"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "test-key").CreateDeployment(context.Background(), validUpdateSDL, console.CreateDeploymentOptions{Secrets: console.SecretValues{}})
	require.Error(t, err)
	var httpErr *console.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, console.CodeProjectRequired, httpErr.Code)
	assert.Contains(t, err.Error(), "akt console project use")
	assert.NotContains(t, err.Error(), "Choose a project to deploy into", "sealed requests keep only the code")
}
