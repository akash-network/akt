package rpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetValidatorsAtHeightRequiresAndUsesExactHeight(t *testing.T) {
	client := NewClient("http://rpc.example.test", "http://rest.example.test")
	for _, height := range []int64{0, -1} {
		_, err := client.GetValidatorsAtHeight(context.Background(), height)
		require.EqualError(t, err, fmt.Sprintf("validator height must be positive, got %d", height))
	}

	requestedHeight := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestedHeight <- request.URL.Query().Get("height")
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"77","validators":[{"address":"AA","voting_power":"10"}],"count":"1","total":"1"}}`)
	}))
	t.Cleanup(server.Close)

	client = NewClient(server.URL, server.URL)
	validators, err := client.GetValidatorsAtHeight(context.Background(), 77)
	require.NoError(t, err)
	require.Len(t, validators, 1)
	require.Equal(t, "AA", validators[0].Address)
	require.Equal(t, "77", <-requestedHeight)

	// Historical sampling must not replace the latest-height cache used by the
	// live consensus view.
	require.False(t, client.validatorsLoaded)
	require.Empty(t, client.validators)
}
