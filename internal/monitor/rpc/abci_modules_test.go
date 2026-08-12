package rpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
)

func TestGetOracleStateProbesCachesAndReturnsPartialAggregates(t *testing.T) {
	var probeCalls atomic.Int32
	var pricesCalls atomic.Int32
	var aggregateCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.Trim(r.URL.Query().Get("path"), `"`)
		data, err := decodeABCIRequestData(r)
		if err != nil {
			t.Errorf("decode ABCI data: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		switch path {
		case "/akash.oracle.v2.Query/Prices":
			var request oracletypes.QueryPricesRequest
			if err := request.Unmarshal(data); err != nil {
				t.Errorf("unmarshal prices request: %v", err)
				http.Error(w, "invalid prices request", http.StatusBadRequest)
				return
			}
			if request.Pagination == nil {
				t.Error("prices pagination is nil")
				http.Error(w, "missing pagination", http.StatusBadRequest)
				return
			}

			response := &oracletypes.QueryPricesResponse{}
			switch request.Pagination.Limit {
			case 1:
				probeCalls.Add(1)
			case 50:
				pricesCalls.Add(1)
				response.Prices = []oracletypes.PriceData{
					{ID: oracletypes.PriceDataRecordID{Denom: "uakt"}},
					{ID: oracletypes.PriceDataRecordID{Denom: "uakt"}},
					{ID: oracletypes.PriceDataRecordID{}},
					{ID: oracletypes.PriceDataRecordID{Denom: "uusdc"}},
				}
			default:
				t.Errorf("prices pagination limit = %d, want 1 or 50", request.Pagination.Limit)
				http.Error(w, "unexpected limit", http.StatusBadRequest)
				return
			}
			writeTestProtoABCIResponse(t, w, response)

		case "/akash.oracle.v2.Query/AggregatedPrice":
			aggregateCalls.Add(1)
			var request oracletypes.QueryAggregatedPriceRequest
			if err := request.Unmarshal(data); err != nil {
				t.Errorf("unmarshal aggregate request: %v", err)
				http.Error(w, "invalid aggregate request", http.StatusBadRequest)
				return
			}
			if request.Denom == "uusdc" {
				writeABCIResult(w, 9, "price unavailable", nil)
				return
			}
			if request.Denom != "uakt" {
				t.Errorf("aggregate denom = %q, want uakt or uusdc", request.Denom)
				http.Error(w, "unexpected denom", http.StatusBadRequest)
				return
			}
			writeTestProtoABCIResponse(t, w, &oracletypes.QueryAggregatedPriceResponse{
				AggregatedPrice: oracletypes.AggregatedPrice{Denom: request.Denom, NumSources: 3},
			})

		default:
			t.Errorf("unexpected ABCI path %q", path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, server.URL)
	for range 2 {
		state, err := client.GetOracleState(context.Background())
		require.NoError(t, err)
		require.Equal(t, "v2", state.Version)
		require.Len(t, state.Prices.Prices, 4)
		require.Equal(t, "uakt", state.Aggregated["uakt"].AggregatedPrice.Denom)
		require.Equal(t, uint32(3), state.Aggregated["uakt"].AggregatedPrice.NumSources)
		require.NotContains(t, state.Aggregated, "uusdc")
	}

	require.Equal(t, int32(1), probeCalls.Load())
	require.Equal(t, int32(2), pricesCalls.Load())
	require.Equal(t, int32(4), aggregateCalls.Load())
}

func TestGetOracleStateFallsBackAndCachesUnavailableVersion(t *testing.T) {
	t.Run("v1 fallback", func(t *testing.T) {
		var v2Calls atomic.Int32
		var v1Calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.Trim(r.URL.Query().Get("path"), `"`)
			switch path {
			case "/akash.oracle.v2.Query/Prices":
				v2Calls.Add(1)
				writeABCIResult(w, 1, "unknown service", nil)
			case "/akash.oracle.v1.Query/Prices":
				v1Calls.Add(1)
				writeTestProtoABCIResponse(t, w, &oracletypes.QueryPricesResponse{})
			default:
				t.Errorf("unexpected ABCI path %q", path)
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(server.Close)

		client := NewClient(server.URL, server.URL)
		for range 2 {
			state, err := client.GetOracleState(context.Background())
			require.NoError(t, err)
			require.Equal(t, "v1", state.Version)
			require.NotNil(t, state.Prices)
			require.Empty(t, state.Aggregated)
		}
		require.Equal(t, int32(1), v2Calls.Load())
		require.Equal(t, int32(3), v1Calls.Load())
	})

	t.Run("no oracle module", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			writeABCIResult(w, 1, "unknown service", nil)
		}))
		t.Cleanup(server.Close)

		client := NewClient(server.URL, server.URL)
		for range 2 {
			state, err := client.GetOracleState(context.Background())
			require.NoError(t, err)
			require.Equal(t, "none", state.Version)
			require.Nil(t, state.Prices)
			require.Empty(t, state.Aggregated)
		}
		require.Equal(t, int32(2), calls.Load())
	})
}

func TestExtractOracleDenomsPreservesFirstSeenOrder(t *testing.T) {
	require.Nil(t, extractOracleDenoms(nil))
	require.Equal(t, []string{"uakt", "uusdc"}, extractOracleDenoms(&oracletypes.QueryPricesResponse{
		Prices: []oracletypes.PriceData{
			{ID: oracletypes.PriceDataRecordID{Denom: "uakt"}},
			{ID: oracletypes.PriceDataRecordID{}},
			{ID: oracletypes.PriceDataRecordID{Denom: "uakt"}},
			{ID: oracletypes.PriceDataRecordID{Denom: "uusdc"}},
		},
	}))
}

func TestGetBMEStateMapsRequestsAndKeepsPartialResults(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			path := strings.Trim(r.URL.Query().Get("path"), `"`)
			switch path {
			case "/akash.bme.v1.Query/Status":
				if data := r.URL.Query().Get("data"); data != "" {
					t.Errorf("status data = %q, want empty", data)
					http.Error(w, "unexpected status data", http.StatusBadRequest)
					return
				}
				writeTestProtoABCIResponse(t, w, &bmetypes.QueryStatusResponse{
					Status: bmetypes.MintStatusHealthy, MintsAllowed: true, RefundsAllowed: true,
				})
			case "/akash.bme.v1.Query/LedgerRecords":
				data, err := decodeABCIRequestData(r)
				if err != nil {
					t.Errorf("decode ledger data: %v", err)
					http.Error(w, "invalid ledger data", http.StatusBadRequest)
					return
				}
				var request bmetypes.QueryLedgerRecordsRequest
				if err := request.Unmarshal(data); err != nil {
					t.Errorf("unmarshal ledger request: %v", err)
					http.Error(w, "invalid ledger request", http.StatusBadRequest)
					return
				}
				if request.Pagination == nil || request.Pagination.Limit != 20 || !request.Pagination.Reverse {
					t.Errorf("ledger pagination = %#v, want limit 20 reverse", request.Pagination)
					http.Error(w, "unexpected ledger pagination", http.StatusBadRequest)
					return
				}
				writeTestProtoABCIResponse(t, w, &bmetypes.QueryLedgerRecordsResponse{
					Records: []bmetypes.QueryLedgerRecordEntry{{
						ID:     bmetypes.LedgerRecordID{Denom: "uakt", ToDenom: "uact", Height: 42},
						Status: bmetypes.LedgerRecordSatusExecuted,
					}},
				})
			default:
				t.Errorf("unexpected ABCI path %q", path)
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(server.Close)

		state, err := NewClient(server.URL, server.URL).GetBMEState(context.Background())
		require.NoError(t, err)
		require.Equal(t, bmetypes.MintStatusHealthy, state.Status.Status)
		require.True(t, state.Status.MintsAllowed)
		require.Len(t, state.Ledger.Records, 1)
		require.Equal(t, int64(42), state.Ledger.Records[0].ID.Height)
		require.Equal(t, int32(2), calls.Load())
	})

	t.Run("failed queries remain nil", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Query().Get("path"), "Status") {
				writeABCIResult(w, 2, "status unavailable", nil)
				return
			}
			_, _ = fmt.Fprint(w, `{"result":{"response":{"code":0,"value":"%%%"}}}`)
		}))
		t.Cleanup(server.Close)

		state, err := NewClient(server.URL, server.URL).GetBMEState(context.Background())
		require.NoError(t, err)
		require.Nil(t, state.Status)
		require.Nil(t, state.Ledger)
	})
}

func TestABCIQueryReportsEveryTransportAndDecodeBoundary(t *testing.T) {
	destination := &testABCIUnmarshaler{}
	client := NewClient("http://rpc.example", "http://rest.example")

	err := client.abciQuery(context.Background(), "/test", testABCIMarshaler{err: errors.New("marshal failed")}, destination)
	require.EqualError(t, err, "marshal request: marshal failed")

	invalid := NewClient("://bad endpoint", "http://rest.example")
	err = invalid.abciQuery(context.Background(), "/test", testABCIMarshaler{}, destination)
	require.Error(t, err)

	for _, tc := range []struct {
		name     string
		response func() *http.Response
		err      error
		want     string
	}{
		{name: "network", err: errors.New("offline"), want: "offline"},
		{name: "status", response: func() *http.Response { return stringResponse(http.StatusBadGateway, "bad gateway") }, want: "status 502"},
		{name: "read", response: func() *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Body: errorReader{err: errors.New("read failed")}, Header: make(http.Header)}
		}, want: "read failed"},
		{name: "malformed JSON", response: func() *http.Response { return stringResponse(http.StatusOK, "not-json") }, want: "parse ABCI response"},
		{name: "ABCI error", response: func() *http.Response {
			return stringResponse(http.StatusOK, `{"result":{"response":{"code":8,"log":"bad query"}}}`)
		}, want: "ABCI error: code=8 log=bad query"},
		{name: "base64", response: func() *http.Response {
			return stringResponse(http.StatusOK, `{"result":{"response":{"code":0,"value":"%%%"}}}`)
		}, want: "decode base64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient("http://rpc.example", "http://rest.example")
			client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				if tc.response == nil {
					return nil, tc.err
				}
				return tc.response(), tc.err
			})
			err := client.abciQuery(context.Background(), "/test", testABCIMarshaler{}, &testABCIUnmarshaler{})
			require.ErrorContains(t, err, tc.want)
		})
	}

	unmarshalFailure := NewClient("http://rpc.example", "http://rest.example")
	unmarshalFailure.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return stringResponse(http.StatusOK, `{"result":{"response":{"code":0,"value":"AQI="}}}`), nil
	})
	destination = &testABCIUnmarshaler{err: errors.New("unmarshal failed")}
	err = unmarshalFailure.abciQuery(context.Background(), "/test", testABCIMarshaler{}, destination)
	require.EqualError(t, err, "unmarshal failed")
	require.Equal(t, []byte{1, 2}, destination.data)
}

type testABCIMarshaler struct {
	data []byte
	err  error
}

func (m testABCIMarshaler) Marshal() ([]byte, error) {
	return m.data, m.err
}

type testABCIUnmarshaler struct {
	data []byte
	err  error
}

func (u *testABCIUnmarshaler) Unmarshal(data []byte) error {
	u.data = append([]byte(nil), data...)
	return u.err
}

func decodeABCIRequestData(r *http.Request) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(r.URL.Query().Get("data"), "0x"))
}

func writeTestProtoABCIResponse(t *testing.T, w http.ResponseWriter, message interface{ Marshal() ([]byte, error) }) {
	t.Helper()
	encoded, err := message.Marshal()
	if err != nil {
		t.Errorf("marshal ABCI response: %v", err)
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}
	writeABCIResult(w, 0, "", encoded)
}
