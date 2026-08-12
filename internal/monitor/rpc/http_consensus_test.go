package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientDefaultsAndEndpoints(t *testing.T) {
	client := NewClient("", "")
	require.Equal(t, DefaultRPCEndpoint, client.Endpoint())
	require.Equal(t, DefaultRESTEndpoint, client.RESTEndpoint())
	require.Equal(t, DefaultTimeout, client.httpClient.Timeout)

	client = NewClient("http://rpc.example", "http://rest.example")
	require.Equal(t, "http://rpc.example", client.Endpoint())
	require.Equal(t, "http://rest.example", client.RESTEndpoint())
}

func TestClientFetchesPaginatedValidatorsAndConsensusState(t *testing.T) {
	var validatorRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/validators":
			validatorRequests.Add(1)
			if r.URL.Query().Get("per_page") != "100" {
				http.Error(w, "wrong page size", http.StatusBadRequest)
				return
			}
			switch r.URL.Query().Get("page") {
			case "1":
				_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"type":"tendermint/PubKeyEd25519","value":"key-a"},"voting_power":"10"},{"address":"BB","pub_key":{"type":"tendermint/PubKeyEd25519","value":"key-b"},"voting_power":"20"}],"count":"2","total":"3"}}`)
			case "2":
				_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"block_height":"42","validators":[{"address":"CC","pub_key":{"type":"tendermint/PubKeyEd25519","value":"key-c"},"voting_power":"30"}],"count":"1","total":"3"}}`)
			default:
				http.Error(w, "unexpected page", http.StatusBadRequest)
			}
		case "/consensus_state":
			_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"round_state":{"height/round/step":"42/1/6","start_time":"2026-08-11T12:00:00Z","height_vote_set":[{"round":0,"prevotes":[],"precommits":[]},{"round":1,"prevotes":["vote-a","nil-Vote","vote-c"],"prevotes_bit_array":"BA{3:x_x} 40/60 = 0.67","precommits":["nil-Vote","vote-b","vote-c"],"precommits_bit_array":"BA{3:_xx} 50/60 = 0.83"}],"proposer":{"address":"BB","index":1}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, server.URL)
	validators, err := client.GetValidators(context.Background())
	require.NoError(t, err)
	require.Len(t, validators, 3)
	require.Equal(t, "AA", validators[0].Address)
	require.Equal(t, "CC", validators[2].Address)
	require.Equal(t, int32(2), validatorRequests.Load())

	// The validator set is immutable for a client session and is fetched once.
	cached, err := client.GetValidators(context.Background())
	require.NoError(t, err)
	require.Equal(t, validators, cached)
	require.Equal(t, int32(2), validatorRequests.Load())

	state, err := client.GetConsensusStateWithValidators(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(42), state.Height)
	require.Equal(t, 1, state.Round)
	require.Equal(t, 6, state.Step)
	require.Equal(t, "BB", state.ProposerAddress)
	require.Equal(t, 1, state.ProposerIndex)
	require.Equal(t, 3, state.TotalValidators)
	require.Equal(t, int64(60), state.TotalVotingPower)
	require.Equal(t, 2, state.PrevoteCount)
	require.Equal(t, int64(40), state.PrevotePower)
	require.InDelta(t, 0.67, state.PrevotePercent, 0.0001)
	require.True(t, state.Validators[0].Prevoted)
	require.False(t, state.Validators[1].Prevoted)
	require.True(t, state.Validators[1].Precommited)
	require.True(t, state.Validators[1].IsProposer)
}

func TestGetLatestCommitUsesCanonicalPreviousHeight(t *testing.T) {
	queries := make(chan url.Values, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/commit" {
			http.NotFound(w, r)
			return
		}
		queries <- r.URL.Query()
		if r.URL.Query().Get("height") == "" {
			_, _ = fmt.Fprint(w, `{"result":{"signed_header":{"commit":{"height":"10","signatures":[{"block_id_flag":2,"validator_address":"tip-only"}]}}}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"result":{"signed_header":{"commit":{"height":"9","signatures":[{"block_id_flag":2,"validator_address":"a1b2"},{"block_id_flag":1,"validator_address":"ignored-absent"},{"block_id_flag":2,"validator_address":""}]}}}}`)
	}))
	t.Cleanup(server.Close)

	height, signers, err := NewClient(server.URL, server.URL).GetLatestCommit(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(9), height)
	require.Equal(t, map[string]bool{"A1B2": true}, signers)
	require.Empty(t, (<-queries).Get("height"))
	require.Equal(t, "9", (<-queries).Get("height"))
}

func TestGetLatestCommitAtGenesisRefetchesTip(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if height := r.URL.Query().Get("height"); height != "" {
			t.Errorf("height query = %q, want empty", height)
			http.Error(w, "unexpected height", http.StatusBadRequest)
			return
		}
		call := requests.Add(1)
		address := "first"
		if call == 2 {
			address = "second"
		}
		_, _ = fmt.Fprintf(w, `{"result":{"signed_header":{"commit":{"height":"1","signatures":[{"block_id_flag":2,"validator_address":%q}]}}}}`, address)
	}))
	t.Cleanup(server.Close)

	height, signers, err := NewClient(server.URL, server.URL).GetLatestCommit(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), height)
	require.Equal(t, map[string]bool{"SECOND": true}, signers)
	require.Equal(t, int32(2), requests.Load())
}

func TestConsensusValidatorAndCommitFailures(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		status    int
		body      string
		operation func(*Client) error
		want      string
	}{
		{
			name:   "consensus status",
			path:   "/consensus_state",
			status: http.StatusServiceUnavailable,
			operation: func(c *Client) error {
				_, err := c.GetConsensusState(context.Background())
				return err
			},
			want: "unexpected status code: 503",
		},
		{
			name: "consensus malformed JSON",
			path: "/consensus_state",
			body: "{",
			operation: func(c *Client) error {
				_, err := c.GetConsensusState(context.Background())
				return err
			},
			want: "failed to parse consensus state",
		},
		{
			name:   "validator status",
			path:   "/validators",
			status: http.StatusBadGateway,
			operation: func(c *Client) error {
				_, err := c.GetValidators(context.Background())
				return err
			},
			want: "unexpected status code: 502",
		},
		{
			name: "validator malformed JSON",
			path: "/validators",
			body: "not-json",
			operation: func(c *Client) error {
				_, err := c.GetValidators(context.Background())
				return err
			},
			want: "failed to parse validators",
		},
		{
			name: "validator malformed total",
			path: "/validators",
			body: `{"result":{"validators":[],"total":"many"}}`,
			operation: func(c *Client) error {
				_, err := c.GetValidators(context.Background())
				return err
			},
			want: "failed to parse total validators",
		},
		{
			name:   "commit status",
			path:   "/commit",
			status: http.StatusBadGateway,
			operation: func(c *Client) error {
				_, _, err := c.GetLatestCommit(context.Background())
				return err
			},
			want: "commit returned status 502",
		},
		{
			name: "commit malformed JSON",
			path: "/commit",
			body: "not-json",
			operation: func(c *Client) error {
				_, _, err := c.GetLatestCommit(context.Background())
				return err
			},
			want: "failed to parse commit",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != tc.path {
					http.NotFound(w, r)
					return
				}
				status := tc.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)

			client := NewClient(server.URL, server.URL)
			err := tc.operation(client)
			require.ErrorContains(t, err, tc.want)
		})
	}

	invalid := NewClient("://bad endpoint", "://bad endpoint")
	_, err := invalid.GetConsensusState(context.Background())
	require.ErrorContains(t, err, "failed to create request")
	_, _, err = invalid.fetchCommit(context.Background(), "")
	require.ErrorContains(t, err, "failed to create commit request")
	_, _, err = invalid.fetchValidatorsPage(context.Background(), 1, 100)
	require.ErrorContains(t, err, "failed to create request")

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewClient("http://127.0.0.1:1", "http://127.0.0.1:1")
	_, err = client.GetConsensusState(cancelled)
	require.ErrorIs(t, err, context.Canceled)
	_, _, err = client.fetchCommit(cancelled, "")
	require.ErrorIs(t, err, context.Canceled)
}

func TestGetValidatorsRetriesAfterTransientFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/validators" {
			http.NotFound(w, r)
			return
		}
		if requests.Add(1) == 1 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"result":{"validators":[{"address":"AA","voting_power":"10"}],"total":"1"}}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, server.URL)
	_, err := client.GetValidators(context.Background())
	require.ErrorContains(t, err, "unexpected status code: 503")

	validators, err := client.GetValidators(context.Background())
	require.NoError(t, err)
	require.Len(t, validators, 1)
	require.Equal(t, "AA", validators[0].Address)
	require.Equal(t, int32(2), requests.Load())

	_, err = client.GetValidators(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(2), requests.Load(), "successful response should remain cached")
}

func TestRefreshValidatorsAtHeightRejectsMismatchedResponseWithoutReplacingCache(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/validators" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		switch r.URL.Query().Get("height") {
		case "":
			_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"10"}],"total":"1"}}`)
		case "43":
			// A proxy or lagging RPC that ignores the requested height must not
			// be allowed to relabel this set as height 43.
			_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"BB","voting_power":"90"}],"total":"1"}}`)
		default:
			http.Error(w, "unexpected height", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, server.URL)
	initial, err := client.GetValidators(context.Background())
	require.NoError(t, err)
	require.Equal(t, "AA", initial[0].Address)

	_, err = client.refreshValidatorsAtHeight(context.Background(), 43)
	require.ErrorContains(t, err, `validator response height "42" does not match requested height 43`)

	cached, err := client.GetValidators(context.Background())
	require.NoError(t, err)
	require.Equal(t, "AA", cached[0].Address, "a rejected height response must not replace the last successful cache")
	require.Equal(t, int32(2), requests.Load())

	_, err = client.refreshValidatorsAtHeight(context.Background(), 0)
	require.ErrorContains(t, err, "validator height must be positive")
}

func TestGetConsensusStateWithValidatorsPropagatesEachBoundaryFailure(t *testing.T) {
	t.Run("validators", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		t.Cleanup(server.Close)

		_, err := NewClient(server.URL, server.URL).GetConsensusStateWithValidators(context.Background())
		require.ErrorContains(t, err, "unexpected status code: 503")
	})

	t.Run("consensus parse", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/validators":
				_, _ = fmt.Fprint(w, `{"result":{"validators":[],"total":"0"}}`)
			case "/consensus_state":
				_, _ = fmt.Fprint(w, `{"result":{"round_state":{"height/round/step":"10/-1/2"}}}`)
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(server.Close)

		_, err := NewClient(server.URL, server.URL).GetConsensusStateWithValidators(context.Background())
		require.ErrorContains(t, err, "invalid round")
	})
}

func TestGetConsensusStateWithValidatorsCancelsValidatorLoad(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/validators" {
			http.NotFound(w, r)
			return
		}
		close(requestStarted)
		select {
		case <-r.Context().Done():
		case <-releaseRequest:
			http.Error(w, "released without cancellation", http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := NewClient(server.URL, server.URL).GetConsensusStateWithValidators(ctx)
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		close(releaseRequest)
		t.Fatal("validator request did not start")
	}
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(250 * time.Millisecond):
		close(releaseRequest)
		err := <-result
		t.Fatalf("validator request ignored runtime cancellation: %v", err)
	}
}

func TestHTTPFailuresPreserveContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	client := NewClient(server.URL, server.URL)
	_, err := client.GetConsensusState(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded), "error = %v", err)
}
