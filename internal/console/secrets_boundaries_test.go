package console

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/deploymentconfig"
)

type secretReadFailure struct{}

func (secretReadFailure) Read([]byte) (int, error) { return 0, errors.New("private-value") }

func TestSecretFileAndKeyFailuresAreValueSafe(t *testing.T) {
	_, err := ReadSecretValues(secretReadFailure{})
	require.ErrorContains(t, err, "cannot read")
	require.NotContains(t, err.Error(), "private-value")
	_, err = ReadSecretValues(strings.NewReader("TOKEN: ["))
	require.ErrorContains(t, err, "JSON or YAML")
	_, err = ReadSecretValuesFile("-", nil)
	require.ErrorContains(t, err, "unavailable")
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.yaml")
	_, err = ReadSecretValuesFile(path, nil)
	require.ErrorContains(t, err, "regular file")
	_, err = ReadSecretValuesFile(dir, nil)
	require.ErrorContains(t, err, "regular file")
	require.NoError(t, os.WriteFile(path, []byte("TOKEN: private-value"), 0600))
	values, err := ReadSecretValuesFile(path, nil)
	require.NoError(t, err)
	require.Equal(t, 1, values.Len())
	require.NoError(t, os.Chmod(path, 0000))
	_, err = ReadSecretValuesFile(path, nil)
	require.ErrorContains(t, err, "cannot open")
	require.NoError(t, os.Chmod(path, 0600))
	file, err := os.Open(dir)
	require.NoError(t, err)
	_, err = readSecretValuesFile(file)
	require.ErrorContains(t, err, "regular file")
	require.NoError(t, file.Close())
	_, err = readSecretValuesFile(file)
	require.ErrorContains(t, err, "regular file")
	_, valid := secretTestContext(t)
	for _, mutate := range []func(*SecretsContext){
		func(c *SecretsContext) { c.JWK.KeyID = "mismatch" }, func(c *SecretsContext) { c.RequiredClaims = []string{"sub", "kid"} },
		func(c *SecretsContext) { c.JWK.Key = (*rsa.PublicKey)(nil) },
	} {
		candidate := valid
		mutate(&candidate)
		_, err = sealSecrets(candidate, values, "", time.Now())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private-value")
	}
	// A malformed modulus passes size checks but is refused by the crypto implementation.
	invalidKey := valid
	invalidKey.JWK.Key = &rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 2047), E: 65537}
	_, err = sealSecrets(invalidKey, values, "", time.Now())
	require.ErrorContains(t, err, "cannot encrypt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope(SecretsContext{JWK: valid.JWK, RequiredClaims: valid.RequiredClaims, KeyID: valid.KeyID}))
	}))
	defer srv.Close()
	_, err = New(srv.URL, "").GetSecretsContext(context.Background())
	require.ErrorContains(t, err, "invalid secret encryption context")
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, ErrUnauthorized, ErrInsufficientFunds, ErrNotFound, ErrAlreadyClosed} {
		require.ErrorIs(t, sanitizeSecretError(fmt.Errorf("private-value: %w", known)), known)
	}
}

func TestSecretMutationPreflightAndAcknowledgements(t *testing.T) {
	_, encryption := secretTestContext(t)
	image := "nginx:1.28-alpine"
	patch := deploymentconfig.Patch{Services: map[string]deploymentconfig.ServicePatch{"web": {Image: &image}}}
	name := "renamed"
	for _, tc := range []struct {
		name, operation, sdl, dseq, source, state, definition, version, response string
		status, keyStatus                                                        int
		patch                                                                    deploymentconfig.Patch
		secrets                                                                  SecretValues
		want                                                                     string
		writes                                                                   int
	}{
		{name: "invalid inheritance", operation: "create", sdl: actionLogTestSDL, source: "bad", want: "dseq"},
		{name: "invalid create SDL", operation: "create", sdl: "[", want: "invalid deployment SDL"},
		{name: "create key refused", operation: "create", sdl: actionLogTestSDL, keyStatus: 401, want: "API key"},
		{name: "create rejected", operation: "create", sdl: actionLogTestSDL, status: 422, want: "422", writes: 1},
		{name: "invalid update dseq", operation: "update", dseq: "bad", sdl: actionLogTestSDL, want: "dseq"},
		{name: "invalid update SDL", operation: "update", sdl: "[", want: "invalid deployment SDL"},
		{name: "closed update", operation: "update", sdl: actionLogTestSDL, state: "closed", want: "closed"},
		{name: "legacy refs", operation: "update", sdl: savedSecretSDL(), want: "legacy updates"},
		{name: "legacy secrets", operation: "update", sdl: actionLogTestSDL, secrets: EmptySecretValues(), want: "legacy updates"},
		{name: "resource edit", operation: "update", sdl: strings.Replace(actionLogTestSDL, "units: 0.5", "units: 2", 1), definition: actionLogTestSDL, version: "old", want: "redeploy"},
		{name: "invalid patch", operation: "patch", patch: deploymentconfig.Patch{Name: new(string)}, want: "deployment name"},
		{name: "empty patch", operation: "patch", want: "empty patch"},
		{name: "closed patch", operation: "patch", patch: patch, state: "closed", want: "closed"},
		{name: "missing definition", operation: "patch", patch: patch, want: "no saved definition"},
		{name: "malformed saved SDL", operation: "patch", patch: patch, definition: "[", version: "old", want: "invalid SDL"},
		{name: "key fetch refused", operation: "patch", patch: patch, definition: actionLogTestSDL, version: "old", keyStatus: 401, want: "API key"},
		{name: "mismatched dseq", operation: "patch", patch: patch, definition: actionLogTestSDL, version: "old", response: `{"data":{"deployment":{"id":{"dseq":"43"},"hash":"new"},"manifestVersion":"new"}}`, want: "requested deployment", writes: 1},
		{name: "mismatched rename", operation: "patch", patch: deploymentconfig.Patch{Name: &name}, response: `{"data":{"deployment":{"id":{"dseq":"42"}},"name":"wrong"}}`, want: "requested name", writes: 1},
		{name: "mismatched version", operation: "patch", patch: patch, definition: actionLogTestSDL, version: "old", response: `{"data":{"deployment":{"id":{"dseq":"42"},"hash":"different"},"manifestVersion":"new"}}`, want: "chain version", writes: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			log := openTestLog(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/sdl-secrets-context":
					if tc.keyStatus != 0 {
						w.WriteHeader(tc.keyStatus)
						return
					}
					_ = json.NewEncoder(w).Encode(envelope(encryption))
				case r.Method == http.MethodGet:
					state := tc.state
					if state == "" {
						state = "active"
					}
					detail := DeploymentDetail{Deployment: Deployment{ID: DeploymentID{DSeq: "42"}, State: state}}
					if tc.definition != "" {
						detail.ConsoleSettings = &DeploymentDefinition{SDL: tc.definition, ManifestVersion: tc.version}
					}
					_ = json.NewEncoder(w).Encode(envelope(detail))
				default:
					writes++
					status := tc.status
					if status == 0 {
						status = 200
					}
					w.WriteHeader(status)
					_, _ = io.WriteString(w, tc.response)
				}
			}))
			defer srv.Close()
			client := New(srv.URL, "").WithActionLog(log)
			ctx := context.Background()
			dseq := tc.dseq
			if dseq == "" {
				dseq = "42"
			}
			var err error
			switch tc.operation {
			case "create":
				_, err = client.CreateDeployment(ctx, tc.sdl, CreateDeploymentOptions{InheritSecretsFrom: tc.source})
			case "update":
				_, err = client.UpdateDeployment(ctx, dseq, tc.sdl, UpdateDeploymentOptions{Secrets: tc.secrets})
			case "patch":
				_, err = client.PatchDeployment(ctx, dseq, tc.patch, SecretValues{})
			}
			require.ErrorContains(t, err, tc.want)
			require.Equal(t, tc.writes, writes)
			entries, err := log.Read(actionlog.Filter{Type: actionlog.TypeConsole})
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.NotEqual(t, "success", entries[0].Status)
		})
	}
}

func TestDefinitionReadAndLegacyInputs(t *testing.T) {
	ctx := context.Background()
	require.Error(t, rejectUnsealedReferences("["))
	client := New("http://127.0.0.1:1", "")
	_, err := client.CreateDeployment(ctx, actionLogTestSDL, CreateDeploymentOptions{}, CreateDeploymentOptions{})
	require.ErrorContains(t, err, "one deployment create")
	_, err = client.UpdateDeployment(ctx, "42", actionLogTestSDL, UpdateDeploymentOptions{}, UpdateDeploymentOptions{})
	require.ErrorContains(t, err, "one deployment update")
	for _, status := range []int{http.StatusUnauthorized, http.StatusOK} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"}}}}`)
		}))
		_, err = New(srv.URL, "").GetDeploymentDefinition(ctx, "42")
		require.Error(t, err)
		if status == http.StatusOK {
			require.Contains(t, err.Error(), "original SDL")
		}
		srv.Close()
	}
	for _, tc := range []struct {
		code   string
		status int
		want   string
	}{{"provider_unreachable", 502, "choose another bid"}, {"unknown", 422, "422"}} {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(tc.status)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": tc.code, "message": "private-value"})
		}))
		_, err = New(srv.URL, "").CreateLease(ctx, "", []LeaseRequest{{DSeq: "42", GSeq: 1, OSeq: 1, Provider: "p"}})
		require.ErrorContains(t, err, tc.want)
		require.NotContains(t, err.Error(), "private-value")
		require.Equal(t, 1, calls)
		srv.Close()
	}
	require.Empty(t, leaseResponseCode([]byte(`{"code":"unknown"}`)))
}

func TestLegacyDefinitionUpdateUsesOrdinarySDLOnly(t *testing.T) {
	puts := 0
	hash, _, err := deploymentArtifacts(actionLogTestSDL)
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
			var request struct {
				Data struct {
					SDL string `json:"sdl"`
				} `json:"data"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			require.Equal(t, actionLogTestSDL, request.Data.SDL)
		} else {
			require.Equal(t, http.MethodGet, r.Method)
		}
		_ = json.NewEncoder(w).Encode(envelope(DeploymentDetail{Deployment: Deployment{ID: DeploymentID{DSeq: "42"}, State: "active", Hash: hash}}))
	}))
	defer srv.Close()
	result, err := New(srv.URL, "").UpdateDeployment(context.Background(), "42", actionLogTestSDL, UpdateDeploymentOptions{})
	require.NoError(t, err)
	require.Equal(t, "42", result.Deployment.ID.DSeq.String())
	require.Equal(t, 1, puts)
}
