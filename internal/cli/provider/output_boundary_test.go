package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

type providerOutputBoundaryWriter struct {
	err   error
	short bool
}

func (w providerOutputBoundaryWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.short && len(p) > 0 {
		return len(p) - 1, nil
	}

	return len(p), nil
}

func providerOutputTestContext(t *testing.T) context.Context {
	t.Helper()

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := kr.NewMnemonic(
		"owner",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create provider test identity: %v", err)
	}
	owner, err := record.GetAddress()
	if err != nil {
		t.Fatalf("get provider test address: %v", err)
	}

	cctx := sdkclient.Context{}.WithKeyring(kr).WithFromAddress(owner)
	return context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx)
}

func TestProviderMutationAcknowledgementsPropagateDestinationFailures(t *testing.T) {
	const validSDL = `version: "2.0"
services:
  web:
    image: nginx:1.27
    expose:
      - port: 80
        as: 80
        to:
          - global: true
profiles:
  compute:
    web:
      resources:
        cpu:
          units: 0.5
        memory:
          size: 512Mi
        storage:
          size: 512Mi
  placement:
    dcloud:
      pricing:
        web:
          denom: uact
          amount: 10000
deployment:
  web:
    dcloud:
      profile: web
      count: 1
`

	sdlPath := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(sdlPath, []byte(validSDL), 0o600); err != nil {
		t.Fatalf("write SDL: %v", err)
	}

	var (
		mu       sync.Mutex
		requests = make(map[string]int)
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		if r.Header.Get("Authorization") == "" {
			t.Errorf("%s %s omitted provider authorization", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hardErr := errors.New("provider destination failed")
	failures := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: providerOutputBoundaryWriter{err: hardErr}, want: hardErr},
		{name: "short write", writer: providerOutputBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}
	tests := []struct {
		name string
		args []string
		path string
	}{
		{
			name: "manifest",
			args: []string{"send-manifest", sdlPath, "--dseq", "42", "--provider", testProviderAddr, "--provider-url", srv.URL},
			path: http.MethodPut + " /deployment/42/manifest",
		},
		{
			name: "hostnames",
			args: []string{"migrate-hostnames", "--dseq", "42", "--provider", testProviderAddr, "--provider-url", srv.URL, "--hostnames", "web.example.test"},
			path: http.MethodPost + " /hostname/migrate",
		},
		{
			name: "endpoints",
			args: []string{"migrate-endpoints", "--dseq", "42", "--provider", testProviderAddr, "--provider-url", srv.URL, "--endpoints", "ep1"},
			path: http.MethodPost + " /endpoint/migrate",
		},
	}

	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					cmd := Commands()
					cmd.SetContext(providerOutputTestContext(t))
					cmd.SetOut(failure.writer)
					cmd.SetErr(io.Discard)
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					cmd.SetArgs(test.args)

					if err := cmd.Execute(); !errors.Is(err, failure.want) {
						t.Fatalf("error = %v, want destination failure %v", err, failure.want)
					}
				})
			}
		})
	}

	mu.Lock()
	defer mu.Unlock()
	for _, test := range tests {
		if requests[test.path] != len(failures) {
			t.Fatalf("%s requests = %d, want %d completed mutations", test.path, requests[test.path], len(failures))
		}
	}
}
