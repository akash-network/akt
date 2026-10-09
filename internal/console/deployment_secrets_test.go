package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/deploymentconfig"
)

func savedSecretSDL() string {
	return strings.Replace(actionLogTestSDL, "    image: nginx:1.27-alpine\n", "    image: nginx:1.27-alpine\n    env:\n      - TOKEN=ac-secret://TOKEN\n      - PLAIN=ordinary\n", 1)
}

func writeSavedDefinition(w http.ResponseWriter, rawSDL string) {
	_ = json.NewEncoder(w).Encode(envelope(DeploymentDetail{
		Deployment:      Deployment{ID: DeploymentID{DSeq: "42"}, State: "active", Hash: "old-version"},
		ConsoleSettings: &DeploymentDefinition{SDL: rawSDL, ManifestVersion: "old-version"},
	}))
}

func TestCreateDeploymentSealsValuesAndInherits(t *testing.T) {
	key, encryption := secretTestContext(t)
	secrets, err := ReadSecretValues(strings.NewReader(`{"TOKEN":"private-value"}`))
	if err != nil {
		t.Fatal(err)
	}
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/sdl-secrets-context":
			_ = json.NewEncoder(w).Encode(envelope(encryption))
		case "POST /v1/deployments":
			posts++
			raw, _ := io.ReadAll(r.Body)
			if strings.Contains(string(raw), "private-value") {
				t.Error("plaintext secret in request")
			}
			var request struct {
				Data struct {
					SDL     string `json:"sdl"`
					Sealed  string `json:"sealedSecrets"`
					Inherit string `json:"inheritSecretsFrom"`
				} `json:"data"`
			}
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Error(err)
				return
			}
			if request.Data.SDL != savedSecretSDL() || request.Data.Inherit != "41" {
				t.Error("SDL or inheritance changed")
			}
			header, values := decryptSecretToken(t, key, request.Data.Sealed)
			if values["TOKEN"] != "private-value" || header["sdlHash"] == nil {
				t.Error("create secrets or SDL binding missing")
			}
			_, _ = io.WriteString(w, `{"data":{"dseq":"42","manifest":"unresolved","signTx":{"code":0,"transactionHash":"tx"}}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	result, err := New(srv.URL, "").CreateDeployment(context.Background(), savedSecretSDL(), CreateDeploymentOptions{Secrets: secrets, InheritSecretsFrom: "41"})
	if err != nil {
		t.Fatal(err)
	}
	if posts != 1 || result.DSeq != "42" {
		t.Fatalf("posts=%d result=%v", posts, result)
	}
}

func TestCreateWithSecretsNeverReconcilesUnresolvedHash(t *testing.T) {
	_, encryption := secretTestContext(t)
	log := openTestLog(t)
	posts, reads := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sdl-secrets-context" {
			_ = json.NewEncoder(w).Encode(envelope(encryption))
			return
		}
		if r.Method == "POST" {
			posts++
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"message":"private-value"}`)
			return
		}
		reads++
		writeSavedDefinition(w, savedSecretSDL())
	}))
	defer srv.Close()
	_, err := New(srv.URL, "").WithActionLog(log).CreateDeployment(context.Background(), savedSecretSDL(), CreateDeploymentOptions{InheritSecretsFrom: "41"})
	if err == nil || !strings.Contains(err.Error(), "outcome unknown") || strings.Contains(err.Error(), "private-value") {
		t.Fatalf("error=%v", err)
	}
	if posts != 1 || reads != 0 {
		t.Fatalf("posts=%d unexpected reconciliation reads=%d", posts, reads)
	}
	entries, err := log.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Status != "pending" {
		t.Fatalf("entries=%v", entries)
	}
}

func TestPatchDeploymentPreservesAndRotates(t *testing.T) {
	key, encryption := secretTestContext(t)
	secrets, err := ReadSecretValues(strings.NewReader(`{"TOKEN":"rotated-value"}`))
	if err != nil {
		t.Fatal(err)
	}
	patch, err := deploymentconfig.ParsePatch([]byte(`{"services":{"web":{"image":"nginx:latest","env":{"NEW":"plain","OLD":null},"command":null,"credentials":{"host":"registry.example","username":"ac-secret://USER","password":"ac-secret://PASS"},"expose":{"80":{"port":8080}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/deployments/42":
			writeSavedDefinition(w, savedSecretSDL())
		case "GET /v1/sdl-secrets-context":
			_ = json.NewEncoder(w).Encode(envelope(encryption))
		case "PATCH /v1/deployments/42":
			var request struct {
				Data struct {
					deploymentconfig.Patch
					Sealed string `json:"sealedSecrets"`
				} `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Data.IfManifestVersion != "old-version" {
				t.Error("patch omitted source version")
			}
			header, values := decryptSecretToken(t, key, request.Data.Sealed)
			if values["TOKEN"] != "rotated-value" || len(values) != 1 {
				t.Error("seal changed rotation map")
			}
			if _, exists := header["sdlHash"]; exists {
				t.Error("patch is bound to obsolete SDL")
			}
			service := request.Data.Services["web"]
			if _, exists := service.Env["TOKEN"]; exists {
				t.Error("unchanged secret reference overwritten")
			}
			if old, exists := service.Env["OLD"]; !exists || old != nil {
				t.Error("variable removal lost")
			}
			if string(service.Command) != "null" {
				t.Error("command clear lost")
			}
			_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"active","hash":"new-version"},"manifestVersion":"new-version"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "").PatchDeployment(context.Background(), "42", patch, secrets); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateSavedDefinitionUsesPatchWithEmptySeal(t *testing.T) {
	key, encryption := secretTestContext(t)
	patched := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/deployments/42":
			writeSavedDefinition(w, savedSecretSDL())
		case "GET /v1/sdl-secrets-context":
			_ = json.NewEncoder(w).Encode(envelope(encryption))
		case "PATCH /v1/deployments/42":
			patched = true
			var request struct {
				Data struct {
					deploymentconfig.Patch
					Sealed string `json:"sealedSecrets"`
				} `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			_, values := decryptSecretToken(t, key, request.Data.Sealed)
			if len(values) != 0 {
				t.Error("ordinary update supplied secret values")
			}
			if image := request.Data.Services["web"].Image; image == nil || *image != "nginx:latest" {
				t.Error("image change missing")
			}
			if request.Data.Services["web"].Env != nil {
				t.Error("unchanged variables should be omitted")
			}
			_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"},"hash":"new-version"},"manifestVersion":"new-version"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	updated := strings.Replace(savedSecretSDL(), "nginx:1.27-alpine", "nginx:latest", 1)
	if _, err := New(srv.URL, "").UpdateDeployment(context.Background(), "42", updated, UpdateDeploymentOptions{}); err != nil {
		t.Fatal(err)
	}
	if !patched {
		t.Fatal("no patch submitted")
	}
}

func TestPatchFailureDoesNotInferSuccessFromStoredVersion(t *testing.T) {
	_, encryption := secretTestContext(t)
	for _, status := range []int{400, 409, 500, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			log := openTestLog(t)
			gets, patches := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /v1/deployments/42":
					gets++
					writeSavedDefinition(w, savedSecretSDL())
				case "GET /v1/sdl-secrets-context":
					_ = json.NewEncoder(w).Encode(envelope(encryption))
				case "PATCH /v1/deployments/42":
					patches++
					w.WriteHeader(status)
					if status == 200 {
						_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"},"hash":"old-version"},"manifestVersion":"new-version"}}`)
					} else {
						_, _ = io.WriteString(w, `{"code":"deployment_definition_changed","message":"private-value"}`)
					}
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
				}
			}))
			defer srv.Close()
			image := "nginx:latest"
			_, err := New(srv.URL, "").WithActionLog(log).PatchDeployment(context.Background(), "42", deploymentconfig.Patch{Services: map[string]deploymentconfig.ServicePatch{"web": {Image: &image}}}, SecretValues{})
			if err == nil || strings.Contains(err.Error(), "private-value") {
				t.Fatalf("error=%v", err)
			}
			if gets != 1 || patches != 1 {
				t.Fatalf("GET=%d PATCH=%d; failure was replayed or reconciled", gets, patches)
			}
			entries, err := log.Read(actionlog.Filter{Type: actionlog.TypeConsole})
			if err != nil {
				t.Fatal(err)
			}
			want := "failed"
			if status >= 500 || status == 200 {
				want = "pending"
			}
			if len(entries) != 1 || entries[0].Status != want {
				t.Fatalf("entries=%v want=%s", entries, want)
			}
			raw, _ := json.Marshal(entries)
			if strings.Contains(string(raw), "private-value") {
				t.Fatal("secret in action log")
			}
		})
	}
}

func TestCrossDeviceLeaseOmitsManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if _, ok := body["manifest"]; ok {
			t.Error("empty manifest must be omitted")
		}
		_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[{"id":{"dseq":"42","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active"}]}}`)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "").CreateLease(context.Background(), "", []LeaseRequest{{DSeq: "42", GSeq: 1, OSeq: 1, Provider: "akash1provider"}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLegacyMethodsRejectSecretReferencesBeforeRequests(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(400) }))
	defer srv.Close()
	client := New(srv.URL, "")
	if _, err := client.CreateDeployment(context.Background(), savedSecretSDL()); err == nil || !strings.Contains(err.Error(), "secret options") {
		t.Fatalf("create error=%v", err)
	}
	if _, err := client.UpdateDeployment(context.Background(), "42", savedSecretSDL()); err == nil || !strings.Contains(err.Error(), "secret options") {
		t.Fatalf("update error=%v", err)
	}
	if requests != 0 {
		t.Fatalf("legacy methods sent %d requests", requests)
	}
}

func TestIdenticalSDLRetryStillPatchesForManifestDelivery(t *testing.T) {
	key, encryption := secretTestContext(t)
	patches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/deployments/42":
			writeSavedDefinition(w, savedSecretSDL())
		case "GET /v1/sdl-secrets-context":
			_ = json.NewEncoder(w).Encode(envelope(encryption))
		case "PATCH /v1/deployments/42":
			patches++
			var request struct {
				Data struct {
					deploymentconfig.Patch
					Sealed string `json:"sealedSecrets"`
				} `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			_, values := decryptSecretToken(t, key, request.Data.Sealed)
			if request.Data.HasChanges() || len(values) != 0 || request.Data.IfManifestVersion != "old-version" {
				t.Error("retry should send the guarded empty seal without changing the definition")
			}
			_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"},"hash":"old-version"},"manifestVersion":"old-version"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "").UpdateDeployment(context.Background(), "42", savedSecretSDL(), UpdateDeploymentOptions{}); err != nil {
		t.Fatal(err)
	}
	if patches != 1 {
		t.Fatalf("identical retry skipped PATCH; count=%d", patches)
	}
}

func TestRenameOnlyNeedsNoDefinitionAndRejectsUnsupportedGuard(t *testing.T) {
	for _, guard := range []string{"", "expected-version"} {
		t.Run(guard, func(t *testing.T) {
			patches := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /v1/deployments/42":
					_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"}}}`)
				case "PATCH /v1/deployments/42":
					patches++
					var request struct {
						Data map[string]any `json:"data"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					if request.Data["name"] != "new name" || len(request.Data) != 1 {
						t.Error("rename should send only its normalized name")
					}
					_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"}},"name":"new name"}}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
				}
			}))
			defer srv.Close()
			name := " new name "
			_, err := New(srv.URL, "").PatchDeployment(context.Background(), "42", deploymentconfig.Patch{Name: &name, IfManifestVersion: guard}, SecretValues{})
			if guard != "" {
				if err == nil || patches != 0 {
					t.Fatalf("unsupported guard error=%v patches=%d", err, patches)
				}
			} else if err != nil || patches != 1 {
				t.Fatalf("rename error=%v patches=%d", err, patches)
			}
		})
	}
}

func TestLeaseManifestFailureNeverBecomesActiveLeaseSuccess(t *testing.T) {
	for _, status := range []int{422, 502} {
		for _, manifest := range []string{"", "legacy-manifest"} {
			t.Run(fmt.Sprintf("%d/%s", status, manifest), func(t *testing.T) {
				log := openTestLog(t)
				posts, gets := 0, 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost {
						posts++
						w.WriteHeader(status)
						_, _ = io.WriteString(w, `{"code":"manifest_not_delivered","message":"private-value","data":{"reason":"private-value"}}`)
						return
					}
					gets++
					_, _ = io.WriteString(w, `{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[{"id":{"dseq":"42","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active"}]}}`)
				}))
				defer srv.Close()
				_, err := New(srv.URL, "").WithActionLog(log).CreateLease(context.Background(), manifest, []LeaseRequest{{DSeq: "42", GSeq: 1, OSeq: 1, Provider: "akash1provider"}})
				if err == nil || !strings.Contains(err.Error(), "retry the same lease request") || strings.Contains(err.Error(), "private-value") {
					t.Fatalf("unsafe result: %v", err)
				}
				if posts != 1 || gets != 0 {
					t.Fatalf("POST=%d GET=%d; must not reconcile provider failure as success", posts, gets)
				}
				entries, err := log.Read(actionlog.Filter{Type: actionlog.TypeConsole})
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 1 || entries[0].Status != "failed" {
					t.Fatalf("entries=%v", entries)
				}
			})
		}
	}
}

func TestSavedDefinitionLeaseAmbiguityDoesNotProveDelivery(t *testing.T) {
	posts, gets := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(500)
			return
		}
		gets++
		_, _ = io.WriteString(w, `{"data":{"leases":[{"id":{"dseq":"42","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active"}]}}`)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "").CreateLease(context.Background(), "", []LeaseRequest{{DSeq: "42", GSeq: 1, OSeq: 1, Provider: "akash1provider"}})
	if err == nil || !strings.Contains(err.Error(), "manifest delivery outcome unknown") {
		t.Fatalf("error=%v", err)
	}
	if posts != 1 || gets != 0 {
		t.Fatalf("POST=%d GET=%d", posts, gets)
	}
}
