package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/actionlog"
)

func TestSensitiveCreateSessionRefusalIsDefinitiveAndRedacted(t *testing.T) {
	const credential = "private-api-key"
	_, encryption := secretTestContext(t)
	log := openTestLog(t)
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/sdl-secrets-context":
			_ = json.NewEncoder(w).Encode(envelope(encryption))
		case "POST /v1/deployments":
			posts++
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"code":"session_required","message":"private-api-key private-workload"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	result, err := New(server.URL, credential).WithActionLog(log).CreateDeployment(
		context.Background(), savedSecretSDL(), CreateDeploymentOptions{InheritSecretsFrom: "41"},
	)
	var sessionErr *SessionRequiredError
	require.ErrorAs(t, err, &sessionErr)
	var httpErr *HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, CodeSessionRequired, httpErr.Code)
	require.Empty(t, httpErr.Body)
	require.Contains(t, err.Error(), DefaultUIBaseURL)
	require.ErrorContains(t, err, "HTTP 403")
	require.NotContains(t, err.Error(), credential)
	require.NotContains(t, err.Error(), "private-workload")
	require.Nil(t, result)
	require.Equal(t, 1, posts, "a refused mutation must not be replayed")

	redacted := sanitizeSecretError(fmt.Errorf("private wrapper: %w", err))
	require.ErrorAs(t, redacted, &sessionErr)
	require.EqualError(t, redacted, sessionErr.Error())
	entries, err := log.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "create-deployment", entries[0].Action)
	require.Equal(t, "failed", entries[0].Status)
	require.Equal(t, sessionErr.Error(), entries[0].Error)
}
