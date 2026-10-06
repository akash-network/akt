package console_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	jose "github.com/go-jose/go-jose/v4"

	"pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/akt/internal/deploymentconfig"
)

func TestSavedDefinitionRequestsMatchOpenAPIContract(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encryption := console.SecretsContext{Subject: "test-user", KeyID: "key-1", JWK: jose.JSONWebKey{Key: &key.PublicKey, Use: "enc", Algorithm: "RSA-OAEP-256"}, RequiredClaims: []string{"sub", "kid", "exp"}}
	router := loadContractRouter(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, params, err := router.FindRoute(r)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		validation := &openapi3filter.RequestValidationInput{Request: r, Route: route, PathParams: params, Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, SkipSettingDefaults: true}}
		if err := openapi3filter.ValidateRequest(r.Context(), validation); err != nil {
			t.Errorf("%s %s violates contract: %v", r.Method, route.Path, err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + route.Path {
		case "GET /v1/sdl-secrets-context":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": encryption})
		case "GET /v1/deployments/{dseq}":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": console.DeploymentDetail{Deployment: console.Deployment{ID: console.DeploymentID{DSeq: "42"}, State: "active", Hash: "old"}, ConsoleSettings: &console.DeploymentDefinition{SDL: validUpdateSDL, ManifestVersion: "old"}}})
		case "POST /v1/deployments":
			_, _ = io.WriteString(w, `{"data":{"dseq":"42","manifest":"unresolved","signTx":{"code":0,"transactionHash":"tx"}}}`)
		case "PATCH /v1/deployments/{dseq}":
			_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"},"hash":"new"},"manifestVersion":"new"}}`)
		case "POST /v1/leases":
			_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[{"id":{"dseq":"42","gseq":1,"oseq":1,"provider":"akash1p"},"state":"active"}]}}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, route.Path)
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	client := console.New(srv.URL, "test-key")
	ctx := context.Background()
	secrets, err := console.ReadSecretValues(strings.NewReader(`{"TOKEN":"private-value"}`))
	if err != nil {
		t.Fatal(err)
	}
	referenceSDL := strings.Replace(validUpdateSDL, "    image:", "    env:\n      - TOKEN=ac-secret://TOKEN\n    image:", 1)
	if _, err := client.CreateDeployment(ctx, referenceSDL, console.CreateDeploymentOptions{Secrets: secrets, InheritSecretsFrom: "41"}); err != nil {
		t.Fatal(err)
	}
	patch, err := deploymentconfig.ParsePatch([]byte(`{"services":{"web":{"image":"nginx:latest","env":{"TOKEN":"ac-secret://TOKEN","OLD":null},"command":null,"args":[],"credentials":null,"expose":{"80":{"port":8080,"as":80}}}},"ifManifestVersion":"old"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.PatchDeployment(ctx, "42", patch, secrets); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateDeployment(ctx, "42", strings.Replace(validUpdateSDL, "nginx:1.27-alpine", "nginx:latest", 1), console.UpdateDeploymentOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetDeploymentDefinition(ctx, "42"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateLease(ctx, "", []console.LeaseRequest{{DSeq: "42", GSeq: 1, OSeq: 1, Provider: "akash1p"}}); err != nil {
		t.Fatal(err)
	}
}
