package console_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

type redirectedRequest struct {
	method string
	apiKey string
	body   string
}

func TestClientRejectsRedirectsWithoutForwardingCredentialsOrBody(t *testing.T) {
	const apiKey = "sandbox-secret-key"

	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			t.Run("authenticated read", func(t *testing.T) {
				redirected := make(chan redirectedRequest, 1)
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, _ := io.ReadAll(r.Body)
					redirected <- redirectedRequest{
						method: r.Method,
						apiKey: r.Header.Get("x-api-key"),
						body:   string(body),
					}
					_, _ = w.Write([]byte(`{"data":{"username":"redirected"}}`))
				}))
				defer target.Close()

				source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, target.URL+"/captured-read", status)
				}))
				defer source.Close()

				_, err := console.New(source.URL, apiKey).GetUser(context.Background())
				assertNoRedirectedRequest(t, redirected)
				require.Error(t, err)
				require.Contains(t, strings.ToLower(err.Error()), "redirect")
			})

			t.Run("managed wallet mutation", func(t *testing.T) {
				redirected := make(chan redirectedRequest, 1)
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, _ := io.ReadAll(r.Body)
					redirected <- redirectedRequest{
						method: r.Method,
						apiKey: r.Header.Get("x-api-key"),
						body:   string(body),
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				defer target.Close()

				source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, target.URL+"/captured-api-key-create", status)
				}))
				defer source.Close()

				_, err := console.New(source.URL, apiKey).CreateAPIKey(context.Background(), "redirect-test", "")
				assertNoRedirectedRequest(t, redirected)
				require.Error(t, err)
				require.Contains(t, strings.ToLower(err.Error()), "redirect")
			})
		})
	}
}

func assertNoRedirectedRequest(t *testing.T, redirected <-chan redirectedRequest) {
	t.Helper()

	select {
	case request := <-redirected:
		t.Fatalf(
			"redirect target received method=%s, x-api-key=%q, body=%q",
			request.method,
			request.apiKey,
			request.body,
		)
	default:
	}
}
