package console

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
)

const validConsoleDeploymentSDL = `version: "2.0"
services:
  web:
    image: nginx:1.27-alpine
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

// TestDeploymentCreateUnifiedDepositSyntax pins the cross-rail deposit
// contract (transport.ParseDeposit, SPEC §7.4) on `deployment create`: bare
// numbers, "5usd", and "$5" are all USD on the console rail; coin forms fail
// with the transport package's cross-rail error before any request is sent;
// and the shared $0.50 minimum (transport.MinConsoleDepositUSD) is enforced.
func TestDeploymentCreateUnifiedDepositSyntax(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	sdlPath := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(sdlPath, []byte(validConsoleDeploymentSDL), 0o600); err != nil {
		t.Fatalf("write SDL file: %v", err)
	}

	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments" {
			writeJSON(t, w, `{"data":{"deployments":[],"pagination":{"hasMore":false}}}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/deployments" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		writeJSON(t, w, `{"data":{"dseq":"321","manifest":"","signTx":{"code":0,"transactionHash":"tx-create-321","rawLog":""}}}`)
	}))
	defer srv.Close()

	for _, tc := range []struct {
		arg  string
		want string
	}{
		{"5", `"deposit":5`},
		{"5usd", `"deposit":5`},
		{"$5", `"deposit":5`},
		{"2.50usd", `"deposit":2.5`},
	} {
		body = ""
		if _, err := execConsole(t, m, srv.URL, "deployment", "create", sdlPath, tc.arg); err != nil {
			t.Fatalf("create with deposit %q: %v", tc.arg, err)
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("deposit %q: request body = %s, want %s", tc.arg, body, tc.want)
		}
	}

	// Coin forms are chain-rail syntax: rejected client-side with the
	// transport package's cross-rail message, no request sent.
	body = ""
	_, err := execConsole(t, m, srv.URL, "deployment", "create", sdlPath, "5000000uakt")
	if err == nil || !strings.Contains(err.Error(), "console deposits are in USD") {
		t.Errorf("coin deposit must fail with the cross-rail error, got %v", err)
	}
	if body != "" {
		t.Errorf("no request must be sent for a rejected deposit, got body %s", body)
	}

	// Below the shared minimum (transport.MinConsoleDepositUSD).
	_, err = execConsole(t, m, srv.URL, "deployment", "create", sdlPath, "0.25")
	if err == nil || !strings.Contains(err.Error(), "$0.50") {
		t.Errorf("sub-minimum deposit must fail mentioning the $0.50 floor, got %v", err)
	}
}

// TestDeploymentDepositUnifiedSyntax pins the same unified syntax on
// `deployment deposit`: USD forms are accepted, coin forms rejected with the
// cross-rail error, and garbage rejected before any request.
func TestDeploymentDepositUnifiedSyntax(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var body string
	var deposited bool
	var requestedMicros string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/12345" {
			amount := "1000000"
			if deposited {
				amount = requestedMicros
			}
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"`+amount+`"}],"transferred":[]}}}}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		if strings.Contains(body, `"deposit":10`) {
			requestedMicros = "11000000"
		} else {
			requestedMicros = "3500000"
		}
		deposited = true
		writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"`+requestedMicros+`"}],"transferred":[]}}}}`)
	}))
	defer srv.Close()

	for _, tc := range []struct {
		arg  string
		want string
	}{
		{"10usd", `"deposit":10`},
		{"$2.50", `"deposit":2.5`},
	} {
		body = ""
		deposited = false
		if _, err := execConsole(t, m, srv.URL, "deployment", "deposit", "12345", tc.arg); err != nil {
			t.Fatalf("deposit %q: %v", tc.arg, err)
		}
		if !strings.Contains(body, tc.want) || !strings.Contains(body, `"dseq":"12345"`) {
			t.Errorf("deposit %q: request body = %s, want %s", tc.arg, body, tc.want)
		}
	}

	body = ""
	_, err := execConsole(t, m, srv.URL, "deployment", "deposit", "12345", "1akt")
	if err == nil || !strings.Contains(err.Error(), "console deposits are in USD") {
		t.Errorf("coin amount must fail with the cross-rail error, got %v", err)
	}
	if body != "" {
		t.Errorf("no request must be sent for a rejected amount, got body %s", body)
	}

	if _, err := execConsole(t, m, srv.URL, "deployment", "deposit", "12345", "ten"); err == nil {
		t.Error("non-numeric amount must be rejected")
	}
}

func TestDeploymentDepositStructuredAcknowledgement(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var deposited bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/12345" {
			amount := "1000000"
			if deposited {
				amount = "3500000"
			}
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"`+amount+`"}],"transferred":[]}}}}`)
			return
		}
		deposited = true
		if r.Method != http.MethodPost || r.URL.Path != "/v1/deposit-deployment" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"3500000"}],"transferred":[]}}}}`)
	}))
	defer srv.Close()

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			deposited = false
			out, err := execConsole(t, m, srv.URL, "deployment", "deposit", "12345", "$2.50", "-o", format)
			if err != nil {
				t.Fatalf("deposit -o %s: %v", format, err)
			}

			got := decodeStructuredMap(t, format, out)
			want := map[string]any{
				"dseq":       "12345",
				"amount_usd": 2.5,
				"status":     "deposited",
			}
			if len(got) != len(want) {
				t.Fatalf("deposit acknowledgement = %#v, want %#v", got, want)
			}
			for key, wantValue := range want {
				if got[key] != wantValue {
					t.Errorf("deposit acknowledgement %s = %#v, want %#v", key, got[key], wantValue)
				}
			}
		})
	}
}

func TestDeploymentCloseStructuredAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		alreadyClosed bool
	}{
		{name: "closed", status: http.StatusOK},
		{name: "already closed", status: http.StatusNotFound, alreadyClosed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestManager(t)
			if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
				t.Fatalf("SetConsoleAPIKey: %v", err)
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete || r.URL.Path != "/v1/deployments/42" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if tc.status == http.StatusOK {
					writeJSON(t, w, `{"data":{"success":true}}`)
					return
				}
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			for _, format := range []string{"json", "yaml"} {
				t.Run(format, func(t *testing.T) {
					out, err := execConsole(t, m, srv.URL, "deployment", "close", "42", "-o", format)
					if err != nil {
						t.Fatalf("close -o %s: %v", format, err)
					}

					got := decodeStructuredMap(t, format, out)
					if got["dseq"] != "42" || got["state"] != "closed" || got["already_closed"] != tc.alreadyClosed {
						t.Errorf("close acknowledgement = %#v, want dseq=42 state=closed already_closed=%v", got, tc.alreadyClosed)
					}
				})
			}
		})
	}
}

func TestDeploymentCloseConvergesUniqueLocalDeploymentAndLeases(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	ctx := context.Background()
	s, err := bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("OpenContext: %v", err)
	}
	owner := "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu"
	if err := s.PutDeployment(ctx, &sstore.DeploymentRecord{Owner: owner, DSeq: 42, State: "active"}); err != nil {
		t.Fatalf("PutDeployment: %v", err)
	}
	leaseID := sstore.LeaseID{Owner: owner, DSeq: 42, GSeq: 1, OSeq: 1, Provider: "akash1provider"}
	if err := s.PutLease(ctx, &sstore.LeaseRecord{ID: leaseID, State: "active"}); err != nil {
		t.Fatalf("PutLease: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, `{"data":{"success":true}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "deployment", "close", "42"); err != nil {
		t.Fatalf("deployment close: %v", err)
	}

	s, err = bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = s.Close() }()
	dep, err := s.GetDeployment(ctx, owner, 42)
	if err != nil || dep == nil || dep.State != "closed" || dep.ClosedAt == 0 {
		t.Fatalf("deployment after close = %+v, err %v", dep, err)
	}
	lease, err := s.GetLease(ctx, leaseID)
	if err != nil || lease == nil || lease.State != "closed" || lease.ClosedAt == 0 {
		t.Fatalf("lease after close = %+v, err %v", lease, err)
	}
}

func TestDeploymentCloseDoesNotGuessBetweenLocalOwners(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	ctx := context.Background()
	s, err := bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("OpenContext: %v", err)
	}
	owners := []string{
		"akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu",
		"akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
	}
	for _, owner := range owners {
		if err := s.PutDeployment(ctx, &sstore.DeploymentRecord{Owner: owner, DSeq: 42, State: "active"}); err != nil {
			t.Fatalf("PutDeployment: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, `{"data":{"success":true}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "deployment", "close", "42")
	if err != nil {
		t.Fatalf("deployment close: %v", err)
	}
	if !strings.Contains(out, "multiple local owners") {
		t.Errorf("ambiguous local close did not warn:\n%s", out)
	}

	s, err = bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = s.Close() }()
	for _, owner := range owners {
		dep, getErr := s.GetDeployment(ctx, owner, 42)
		if getErr != nil || dep == nil || dep.State != "active" {
			t.Fatalf("ambiguous close changed %s: %+v, err %v", owner, dep, getErr)
		}
	}
}

// TestDeploymentListZeroFlagValues pins that legitimately-zero flag values
// are not mis-reported as errors: --skip 0 is forwarded and --limit 0 falls
// back to the server default page size (the API requires limit >= 1).
func TestDeploymentListZeroFlagValues(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		writeJSON(t, w, `{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":20,"hasMore":false}}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "deployment", "list", "--skip", "0", "--limit", "0"); err != nil {
		t.Fatalf("deployment list with zero flag values must succeed, got %v", err)
	}

	if gotQuery.Get("skip") != "0" {
		t.Errorf("skip=0 must be forwarded, got query %v", gotQuery)
	}
	if _, present := gotQuery["limit"]; present {
		t.Errorf("limit=0 must be omitted so the server default applies, got query %v", gotQuery)
	}
}

func TestManifestFromFile(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}

		return p
	}

	t.Run("accepts a rendered manifest", func(t *testing.T) {
		p := write("manifest.json", `[{"name":"dcloud","services":[]}]`)

		got, err := manifestFromFile(p)
		if err != nil {
			t.Fatalf("expected the JSON manifest to be accepted, got %v", err)
		}
		if got == "" {
			t.Fatal("expected the file contents back")
		}
	})

	// The regression: passing the SDL got as far as Console, which replied
	// "invalid character '-' in numeric literal" -- its JSON parser hitting
	// the leading `---`. Nothing about that names the real mistake.
	t.Run("rejects an SDL with a message that names the cause", func(t *testing.T) {
		p := write("deploy.yaml", "---\nversion: \"2.0\"\nservices:\n  web:\n    image: nginx\n")

		_, err := manifestFromFile(p)
		if err == nil {
			t.Fatal("expected an SDL file to be rejected")
		}
		for _, want := range []string{"not JSON", "not the SDL"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should mention %q, got: %v", want, err)
			}
		}
	})

	t.Run("reports a missing file", func(t *testing.T) {
		if _, err := manifestFromFile(filepath.Join(dir, "absent.json")); err == nil {
			t.Fatal("expected a missing file to error")
		}
	})
}
