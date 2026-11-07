package provider

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestLeaseEventsStopsBeforeStreamingWhenLeaseStatusFails(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.Path
		http.Error(w, "lease unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	cmd := Commands()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"lease-events", "42", "--provider", testProviderAddr, "--provider-url", server.URL})
	err := cmd.ExecuteContext(providerOutputTestContext(t))
	require.ErrorContains(t, err, "query lease status")
	require.ErrorContains(t, err, "lease unavailable")
	require.Len(t, requests, 1)
	require.Equal(t, "GET /lease/42/1/1/status", <-requests)
}

func TestServiceStatusRejectsIncompleteInputAndMissingCertificate(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"service-status", "--dseq", "42"}, "service is required"},
		{[]string{"service-status", "--service", "web"}, "dseq is required"},
		{[]string{"service-status", "42", "web", "--provider", testProviderAddr,
			"--provider-url", "https://provider.example.com", "--auth-type", "mtls"}, "load mTLS certificate"},
	} {
		ctx := providerOutputTestContext(t)
		cctx := ctx.Value(sdkclient.ClientContextKey).(*sdkclient.Context)
		*cctx = cctx.WithHomeDir(t.TempDir())
		cmd := Commands()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(tc.args)
		require.ErrorContains(t, cmd.ExecuteContext(ctx), tc.want)
	}
}
