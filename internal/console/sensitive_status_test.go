package console

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSensitiveMutationPreservesHTTPStatusWithoutResponseText(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusTemporaryRedirect} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			const secret = "private-api-key"
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				if r.Method != http.MethodPost || r.Header.Get("x-api-key") != secret {
					t.Error("mutation did not preserve its method and credential")
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, secret+" private-workload")
			}))
			t.Cleanup(server.Close)

			client := New(server.URL, secret).sensitiveClient()
			var result CreateDeploymentResult
			err := client.doData(context.Background(), http.MethodPost, "/v1/deployments", envelope(map[string]string{"sdl": "fixture"}), &result)
			require.ErrorContains(t, err, fmt.Sprintf("(HTTP %d)", status))
			require.NotContains(t, err.Error(), secret)
			require.NotContains(t, err.Error(), "private-workload")
			require.Equal(t, 1, attempts, "a mutation must never be replayed")
			require.Empty(t, result.DSeq)

			wrapped := fmt.Errorf("private wrapper: %w", err)
			require.EqualError(t, sanitizeSecretError(wrapped), err.Error())
		})
	}
}
