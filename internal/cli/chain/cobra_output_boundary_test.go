package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"google.golang.org/grpc"

	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

const ledgerFallbackNotice = "Default sign-mode 'direct' not supported by Ledger, using sign-mode 'amino-json'.\n"

type outputBoundaryErrorWriter struct {
	err error
}

func (w outputBoundaryErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type outputBoundaryShortWriter struct{}

func (outputBoundaryShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

type ledgerRecordKeyring struct {
	keyring.Keyring
	record *keyring.Record
}

func (k ledgerRecordKeyring) Key(string) (*keyring.Record, error) {
	return k.record, nil
}

func newLedgerRecord(t *testing.T) *keyring.Record {
	t.Helper()

	record, err := keyring.NewLedgerRecord(
		"ledger",
		secp256k1.GenPrivKey().PubKey(),
		&hd.BIP44Params{},
	)
	if err != nil {
		t.Fatalf("create Ledger record: %v", err)
	}
	return record
}

func readLedgerTxContext(t *testing.T, stderr io.Writer, quiet bool) (sdkclient.Context, string, error) {
	t.Helper()

	cmd := txFlagCommand()
	cmd.Flags().Bool("quiet", false, "test quiet mode")
	if quiet {
		if err := cmd.Flags().Set("quiet", "true"); err != nil {
			t.Fatalf("set quiet: %v", err)
		}
	}

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(stderr)
	cctx := sdkclient.Context{}.
		WithKeyring(ledgerRecordKeyring{record: newLedgerRecord(t)}).
		WithFrom("ledger")
	cmd.SetContext(context.WithValue(context.Background(), ClientContextKey, &cctx))

	got, err := GetClientTxContext(cmd)
	return got, stdout.String(), err
}

func TestLedgerFallbackNoticeUsesCobraStderrAndHonorsQuiet(t *testing.T) {
	t.Run("diagnostic stream", func(t *testing.T) {
		var stderr bytes.Buffer
		got, stdout, err := readLedgerTxContext(t, &stderr, false)
		if err != nil {
			t.Fatalf("read transaction context: %v", err)
		}
		if got.SignModeStr != cflags.SignModeLegacyAminoJSON {
			t.Fatalf("sign mode = %q, want %q", got.SignModeStr, cflags.SignModeLegacyAminoJSON)
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if stderr.String() != ledgerFallbackNotice {
			t.Fatalf("stderr = %q, want %q", stderr.String(), ledgerFallbackNotice)
		}
	})

	t.Run("quiet", func(t *testing.T) {
		var stderr bytes.Buffer
		got, stdout, err := readLedgerTxContext(t, &stderr, true)
		if err != nil {
			t.Fatalf("read transaction context: %v", err)
		}
		if got.SignModeStr != cflags.SignModeLegacyAminoJSON {
			t.Fatalf("sign mode = %q, want %q", got.SignModeStr, cflags.SignModeLegacyAminoJSON)
		}
		if stdout != "" || stderr.Len() != 0 {
			t.Fatalf("quiet output = stdout %q, stderr %q", stdout, stderr.String())
		}
	})
}

func TestLedgerFallbackNoticePropagatesDestinationFailures(t *testing.T) {
	hardErr := errors.New("Ledger diagnostic destination failed")
	tests := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: outputBoundaryErrorWriter{err: hardErr}, want: hardErr},
		{name: "short write", writer: outputBoundaryShortWriter{}, want: io.ErrShortWrite},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := readLedgerTxContext(t, tc.writer, false)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

type wasmCodeQueryStub struct {
	wasmtypes.QueryClient
	data   []byte
	codeID uint64
}

func (s *wasmCodeQueryStub) Code(_ context.Context, req *wasmtypes.QueryCodeRequest, _ ...grpc.CallOption) (*wasmtypes.QueryCodeResponse, error) {
	s.codeID = req.CodeId
	return &wasmtypes.QueryCodeResponse{Data: append([]byte(nil), s.data...)}, nil
}

type wasmDownloadQueryClient struct {
	clientv1beta3.QueryClient
	wasm wasmtypes.QueryClient
}

func (c *wasmDownloadQueryClient) Wasm() wasmtypes.QueryClient { return c.wasm }
func (c *wasmDownloadQueryClient) ClientContext() sdkclient.Context {
	return sdkclient.Context{}
}

type wasmDownloadLightClient struct {
	query clientv1beta3.QueryClient
}

func (c *wasmDownloadLightClient) Query() clientv1beta3.QueryClient { return c.query }
func (c *wasmDownloadLightClient) Node() clientv1beta3.NodeClient   { return nil }
func (c *wasmDownloadLightClient) ClientContext() sdkclient.Context { return sdkclient.Context{} }
func (c *wasmDownloadLightClient) PrintMessage(interface{}) error   { return nil }
func (c *wasmDownloadLightClient) PrintJSON(interface{}) error      { return nil }

func runWasmCodeDownload(t *testing.T, stderr io.Writer, quiet bool, path string, data []byte) (string, uint64, error) {
	t.Helper()

	wasm := &wasmCodeQueryStub{data: data}
	cl := &wasmDownloadLightClient{query: &wasmDownloadQueryClient{wasm: wasm}}
	cmd := GetQueryWasmCodeCmd()
	cmd.Flags().Bool("quiet", false, "test quiet mode")
	if quiet {
		if err := cmd.Flags().Set("quiet", "true"); err != nil {
			t.Fatalf("set quiet: %v", err)
		}
	}

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(stderr)
	cmd.SetContext(context.WithValue(context.Background(), ContextTypeQueryClient, cl))
	err := cmd.RunE(cmd, []string{"17", path})

	return stdout.String(), wasm.codeID, err
}

func TestWasmCodeDownloadUsesCobraStderrAndWritesExactArtifact(t *testing.T) {
	data := []byte{0x00, 0x61, 0x73, 0x6d, 0xff, 0x00}
	for _, quiet := range []bool{false, true} {
		name := "diagnostic stream"
		if quiet {
			name = "quiet"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "contract.wasm")
			var stderr bytes.Buffer
			stdout, codeID, err := runWasmCodeDownload(t, &stderr, quiet, path, data)
			if err != nil {
				t.Fatalf("download wasm code: %v", err)
			}
			if codeID != 17 {
				t.Fatalf("queried code ID = %d, want 17", codeID)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			wantStatus := "Downloading wasm code to " + path + "\n"
			if quiet {
				wantStatus = ""
			}
			if stderr.String() != wantStatus {
				t.Fatalf("stderr = %q, want %q", stderr.String(), wantStatus)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read downloaded wasm: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("downloaded bytes = %x, want %x", got, data)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat downloaded wasm: %v", err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("downloaded mode = %o, want 600", info.Mode().Perm())
			}
		})
	}
}

func TestWasmCodeDownloadPropagatesStatusDestinationFailures(t *testing.T) {
	hardErr := errors.New("wasm status destination failed")
	tests := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: outputBoundaryErrorWriter{err: hardErr}, want: hardErr},
		{name: "short write", writer: outputBoundaryShortWriter{}, want: io.ErrShortWrite},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "must-not-exist.wasm")
			_, _, err := runWasmCodeDownload(t, tc.writer, false, path, []byte("wasm"))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("artifact exists after failed status write: %v", statErr)
			}
		})
	}
}
