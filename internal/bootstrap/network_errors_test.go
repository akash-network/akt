package bootstrap

import (
	"errors"
	"net/http"
	"testing"
)

type networkRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn networkRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type failingResponseBody struct {
	err error
}

func (body failingResponseBody) Read([]byte) (int, error) { return 0, body.err }
func (failingResponseBody) Close() error                  { return nil }

func TestNetworkFetchersSurfaceTransportAndBodyFailures(t *testing.T) {
	transportErr := errors.New("network route unavailable")
	bodyErr := errors.New("response stream interrupted")

	for _, test := range []struct {
		name string
		call func(*http.Client) error
	}{
		{name: "repository listing", call: func(client *http.Client) error {
			_, err := listRepoDirs(client)
			return err
		}},
		{name: "network metadata", call: func(client *http.Client) error {
			_, err := fetchMeta(client, "mainnet")
			return err
		}},
	} {
		t.Run(test.name+" transport", func(t *testing.T) {
			client := &http.Client{Transport: networkRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, transportErr
			})}
			if err := test.call(client); !errors.Is(err, transportErr) {
				t.Fatalf("error = %v, want transport failure", err)
			}
		})

		t.Run(test.name+" body", func(t *testing.T) {
			client := &http.Client{Transport: networkRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       failingResponseBody{err: bodyErr},
					Header:     make(http.Header),
					Request:    req,
				}, nil
			})}
			if err := test.call(client); !errors.Is(err, bodyErr) {
				t.Fatalf("error = %v, want response-body failure", err)
			}
		})
	}
}
