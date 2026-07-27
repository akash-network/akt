package console_test

// Contract tests: every request the console client produces is validated
// against the vendored Console API OpenAPI document
// (testdata/openapi.json). The httptest server used here matches each
// incoming request to a spec route, validates path/query/header/body against
// the schema, and records violations; canned responses are only rich enough
// for the client methods to decode. Response-schema validation is explicitly
// out of scope — only REQUEST validation matters, because that is the side
// our httptest-based unit tests cannot see (e.g. the empty startDate/endDate
// 400 that motivated this suite).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"pkg.akt.dev/akt/internal/console"
)

// loadContractRouter loads the vendored OpenAPI spec and builds a router for
// request matching. Servers are cleared so httptest URLs match.
func loadContractRouter(t *testing.T) routers.Router {
	t.Helper()

	data, err := os.ReadFile("testdata/openapi.json")
	if err != nil {
		t.Fatalf("read vendored spec: %v", err)
	}

	// Upstream escaping bug: 44 dseq patterns read "^d+$" (one or more
	// literal 'd' characters) where "^\d+$" (digits) is clearly intended —
	// the correctly escaped "^\\d+$" appears elsewhere in the same document.
	// Restore the intended pattern so numeric dseq values validate. The
	// vendored file itself is kept verbatim.
	data = bytes.ReplaceAll(data, []byte(`"^d+$"`), []byte(`"^\\d+$"`))

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	doc, err := loader.LoadFromData(data)
	if err != nil {
		t.Fatalf("load OpenAPI spec: %v", err)
	}

	// Ignore the production server URL so route matching is host-agnostic.
	doc.Servers = nil

	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("build OpenAPI router: %v", err)
	}

	return router
}

// contractRecorder collects validation failures observed by the test server.
type contractRecorder struct {
	mu   sync.Mutex
	errs []string
}

func (r *contractRecorder) add(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, msg)
}

// take returns the collected errors and resets the recorder.
func (r *contractRecorder) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	errs := r.errs
	r.errs = nil
	return errs
}

// cannedResponse is the minimal reply for one "METHOD /spec/path" so the
// client method under test can decode a success.
type cannedResponse struct {
	status int
	body   string
}

// cannedResponses maps "METHOD <spec path template>" to a response body just
// rich enough for the client to decode. These encode the documented response
// envelopes (data-enveloped vs top-level); response-schema validation is out
// of scope here.
var cannedResponses = map[string]cannedResponse{
	"GET /v1/user/me":  {http.StatusOK, `{"data":{"id":"123e4567-e89b-12d3-a456-426614174000","userId":"u1","username":"tester","email":"tester@example.com","emailVerified":true}}`},
	"GET /v1/balances": {http.StatusOK, `{"data":{"balance":1000000,"deployments":500000,"total":1500000}}`},
	"GET /v1/wallets":  {http.StatusOK, `{"data":[{"address":"akash1xtestwallet","creditAmount":10,"isTrialing":false,"denom":"uakt"}]}`},

	"GET /v1/wallet-settings": {http.StatusOK, `{"data":{"autoReloadEnabled":false}}`},
	"PUT /v1/wallet-settings": {http.StatusOK, `{"data":{"autoReloadEnabled":true}}`},
	"GET /v1/weekly-cost":     {http.StatusOK, `{"data":{"weeklyCost":1.25}}`},

	// Top-level array, NOT data-enveloped.
	"GET /v1/usage/history": {http.StatusOK, `[{"date":"2026-01-01","activeDeployments":1,"dailyUsdcSpent":0.5,"totalUsdcSpent":10.5}]`},

	"GET /v1/deployments":           {http.StatusOK, `{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":20,"hasMore":false}}}`},
	"POST /v1/deployments":          {http.StatusOK, `{"data":{"dseq":"1","manifest":"m"}}`},
	"GET /v1/deployments/{dseq}":    {http.StatusOK, `{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"1"},"state":"active"},"leases":[]}}`},
	"PUT /v1/deployments/{dseq}":    {http.StatusOK, `{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"1"},"state":"active"},"leases":[]}}`},
	"DELETE /v1/deployments/{dseq}": {http.StatusOK, `{"data":{}}`},
	"POST /v1/deposit-deployment":   {http.StatusOK, `{"data":{}}`},

	"GET /v1/bids":    {http.StatusOK, `{"data":[{"bid":{"id":{"owner":"akash1x","dseq":"1","gseq":1,"oseq":1,"provider":"akash1p"},"state":"open","price":{"denom":"uakt","amount":"100"}}}]}`},
	"POST /v1/leases": {http.StatusOK, `{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"1"},"state":"active"},"leases":[{"id":{"owner":"akash1x","dseq":"1","gseq":1,"oseq":1,"provider":"akash1p"},"state":"active"}]}}`},

	"GET /v2/deployment-settings/{dseq}":   {http.StatusOK, `{"data":{"dseq":"1","autoTopUpEnabled":true,"estimatedTopUpAmount":5,"topUpFrequencyMs":60000}}`},
	"PATCH /v2/deployment-settings/{dseq}": {http.StatusOK, `{"data":{"dseq":"1","autoTopUpEnabled":true,"estimatedTopUpAmount":5,"topUpFrequencyMs":60000}}`},
	"POST /v2/deployment-settings":         {http.StatusOK, `{"data":{"dseq":"404","autoTopUpEnabled":true,"estimatedTopUpAmount":5,"topUpFrequencyMs":60000}}`},

	"POST /v1/create-jwt-token": {http.StatusOK, `{"data":{"token":"jwt-token"}}`},

	"GET /v1/api-keys":         {http.StatusOK, `{"data":[{"id":"123e4567-e89b-12d3-a456-426614174000","name":"ci"}]}`},
	"POST /v1/api-keys":        {http.StatusOK, `{"data":{"id":"123e4567-e89b-12d3-a456-426614174000","name":"ci","apiKey":"secret"}}`},
	"DELETE /v1/api-keys/{id}": {http.StatusNoContent, ""},

	// NOT data-enveloped.
	"POST /v1/bid-screening": {http.StatusOK, `{"providers":[{"owner":"akash1p","hostUri":"https://p.example.com:8443","isAudited":true}]}`},

	// Top-level array / object catalog endpoints.
	"GET /v1/providers":           {http.StatusOK, `[{"owner":"akash1p","name":"provider","isOnline":true}]`},
	"GET /v1/providers/{address}": {http.StatusOK, `{"owner":"akash1p","name":"provider","hostUri":"https://p.example.com:8443"}`},
	"GET /v1/provider-regions":    {http.StatusOK, `[{"key":"us-west","description":"US West","providers":["akash1p"]}]`},
	"GET /v1/auditors":            {http.StatusOK, `[{"id":"a1","name":"auditor","address":"akash1aud"}]`},
	"GET /v1/gpu-prices":          {http.StatusOK, `{"availability":{"total":10,"available":4},"models":[{"vendor":"nvidia","model":"h100","ram":"80Gi","price":{"min":1,"max":3,"avg":2}}]}`},

	"GET /v1/templates-list": {http.StatusOK, `{"data":{"categories":[]}}`},
	"GET /v1/templates/{id}": {http.StatusOK, `{"data":{"id":"tpl-1","name":"template"}}`},
}

// newContractServer returns an httptest server that validates every incoming
// request against the OpenAPI router and then serves the canned response for
// the matched route.
func newContractServer(router routers.Router, rec *contractRecorder) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, pathParams, err := router.FindRoute(r)
		if err != nil {
			rec.add(r.Method + " " + r.URL.Path + ": no matching route in OpenAPI spec: " + err.Error())
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{}`)
			return
		}

		input := &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: pathParams,
			Route:      route,
			Options: &openapi3filter.Options{
				// Security schemes (x-api-key / bearer) are the transport's
				// concern; don't fail validation on them.
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
				// Validate what the client actually sends: without this,
				// the validator rewrites JSON null into schema defaults
				// before checking, masking null-vs-array violations.
				SkipSettingDefaults: true,
				// Enforce format assertions (date, date-time, uuid): the
				// motivating bug was an empty string sent for a
				// format=date query parameter.
				SchemaValidationOptions: []openapi3.SchemaValidationOption{
					openapi3.EnableFormatValidation(),
				},
			},
		}
		if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
			rec.add(r.Method + " " + route.Path + ": request violates OpenAPI contract: " + err.Error())
		}

		// Force the PATCH -> POST create fallback of SetDeploymentAutoTopUp:
		// settings for dseq 404 "do not exist yet".
		key := r.Method + " " + route.Path
		if key == "PATCH /v2/deployment-settings/{dseq}" && pathParams["dseq"] == "404" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found"}`)
			return
		}

		resp, ok := cannedResponses[key]
		if !ok {
			rec.add(key + ": no canned response registered; add one to cannedResponses")
			resp = cannedResponse{status: http.StatusOK, body: `{}`}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		_, _ = io.WriteString(w, resp.body)
	}))
}

// TestClientRequestsMatchOpenAPIContract calls every public client method
// with happy-path arguments and fails if any produced request violates the
// vendored Console API contract. A failing subtest names the offending
// client method.
func TestClientRequestsMatchOpenAPIContract(t *testing.T) {
	// The spec uses format=uuid on request parameters (wallets userId,
	// api-key id); kin-openapi only validates uuid when registered.
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC4122))

	router := loadContractRouter(t)
	rec := &contractRecorder{}
	srv := newContractServer(router, rec)
	defer srv.Close()

	c := console.New(srv.URL, "test-key")

	const (
		address  = "akash1xf29ao02u23nippl3ddjrec9dkjzwmz4t7g8dl"
		provider = "akash1p"
		uuid     = "123e4567-e89b-12d3-a456-426614174000"
		sdl      = "version: \"2.0\"\nservices: {}\n"
	)

	// Minimal valid /v1/bid-screening resources per the vendored schema:
	// resource{id,cpu,memory,gpu,storage} + count + price are all required.
	screeningResources := json.RawMessage(`[{
		"resource": {
			"id": 1,
			"cpu": {"units": {"val": "1000"}},
			"memory": {"quantity": {"val": "536870912"}},
			"gpu": {"units": {"val": "0"}},
			"storage": [{"name": "default", "quantity": {"val": "1073741824"}}],
			"endpoints": []
		},
		"count": 1,
		"price": {"denom": "uakt", "amount": "10000"}
	}]`)

	tests := []struct {
		name string
		call func(ctx context.Context) error
	}{
		{"GetUser", func(ctx context.Context) error {
			_, err := c.GetUser(ctx)
			return err
		}},
		{"GetBalances", func(ctx context.Context) error {
			_, err := c.GetBalances(ctx)
			return err
		}},
		{"ListWallets", func(ctx context.Context) error {
			_, err := c.ListWallets(ctx, uuid)
			return err
		}},
		{"GetWalletSettings", func(ctx context.Context) error {
			_, err := c.GetWalletSettings(ctx)
			return err
		}},
		{"UpdateWalletSettings", func(ctx context.Context) error {
			_, err := c.UpdateWalletSettings(ctx, true)
			return err
		}},
		{"GetWeeklyCost", func(ctx context.Context) error {
			_, err := c.GetWeeklyCost(ctx)
			return err
		}},
		// The exact repro of the production 400: no dates supplied. The
		// client must omit startDate/endDate rather than send them empty
		// (format=date rejects "").
		{"GetUsageHistory/no-dates", func(ctx context.Context) error {
			_, err := c.GetUsageHistory(ctx, address, "", "")
			return err
		}},
		{"GetUsageHistory/date-range", func(ctx context.Context) error {
			_, err := c.GetUsageHistory(ctx, address, "2026-01-01", "2026-01-31")
			return err
		}},
		{"ListDeployments", func(ctx context.Context) error {
			_, err := c.ListDeployments(ctx, 0, 20)
			return err
		}},
		{"GetDeployment", func(ctx context.Context) error {
			_, err := c.GetDeployment(ctx, "1")
			return err
		}},
		{"CreateDeployment", func(ctx context.Context) error {
			_, err := c.CreateDeployment(ctx, sdl, 5)
			return err
		}},
		{"UpdateDeployment", func(ctx context.Context) error {
			_, err := c.UpdateDeployment(ctx, "1", sdl)
			return err
		}},
		{"CloseDeployment", func(ctx context.Context) error {
			return c.CloseDeployment(ctx, "1")
		}},
		{"Deposit", func(ctx context.Context) error {
			return c.Deposit(ctx, "1", 5)
		}},
		{"FetchBids", func(ctx context.Context) error {
			_, err := c.FetchBids(ctx, "1")
			return err
		}},
		{"CreateLease", func(ctx context.Context) error {
			_, err := c.CreateLease(ctx, "m", []console.LeaseRequest{
				{DSeq: "1", GSeq: 1, OSeq: 1, Provider: provider},
			})
			return err
		}},
		{"GetDeploymentSettings", func(ctx context.Context) error {
			_, err := c.GetDeploymentSettings(ctx, "1")
			return err
		}},
		{"SetDeploymentAutoTopUp/patch", func(ctx context.Context) error {
			_, err := c.SetDeploymentAutoTopUp(ctx, "1", true)
			return err
		}},
		// dseq 404 makes the server answer the PATCH with 404, forcing the
		// POST /v2/deployment-settings create fallback through validation.
		{"SetDeploymentAutoTopUp/create-fallback", func(ctx context.Context) error {
			_, err := c.SetDeploymentAutoTopUp(ctx, "404", true)
			return err
		}},
		{"CreateJWTToken", func(ctx context.Context) error {
			_, err := c.CreateJWTToken(ctx, 300, []string{"logs"})
			return err
		}},
		{"ListAPIKeys", func(ctx context.Context) error {
			_, err := c.ListAPIKeys(ctx)
			return err
		}},
		{"CreateAPIKey/no-expiry", func(ctx context.Context) error {
			_, err := c.CreateAPIKey(ctx, "n", "")
			return err
		}},
		// The schema declares expiresAt as format=date-time (RFC 3339).
		{"CreateAPIKey/expiry", func(ctx context.Context) error {
			_, err := c.CreateAPIKey(ctx, "n", "2027-01-01T00:00:00Z")
			return err
		}},
		// The spec requires a UUID key id (format=uuid path parameter).
		{"DeleteAPIKey", func(ctx context.Context) error {
			return c.DeleteAPIKey(ctx, uuid)
		}},
		{"ScreenBids/minimal", func(ctx context.Context) error {
			_, err := c.ScreenBids(ctx, &console.BidScreeningRequest{
				Resources: screeningResources,
				Timezone:  "America/Chicago",
			})
			return err
		}},
		{"ScreenBids/with-requirements", func(ctx context.Context) error {
			_, err := c.ScreenBids(ctx, &console.BidScreeningRequest{
				Requirements: console.BidScreeningRequirements{
					SignedBy:   console.SignedBy{AnyOf: []string{"akash1aud"}},
					Attributes: []console.Attribute{{Key: "region", Value: "us-west"}},
				},
				Resources: screeningResources,
				Timezone:  "America/Chicago",
			})
			return err
		}},
		{"ListProviders/all", func(ctx context.Context) error {
			_, err := c.ListProviders(ctx, "", nil)
			return err
		}},
		{"ListProviders/trial-with-addresses", func(ctx context.Context) error {
			_, err := c.ListProviders(ctx, "trial", []string{provider})
			return err
		}},
		{"GetProvider", func(ctx context.Context) error {
			_, err := c.GetProvider(ctx, provider)
			return err
		}},
		{"ListProviderRegions", func(ctx context.Context) error {
			_, err := c.ListProviderRegions(ctx)
			return err
		}},
		{"ListAuditors", func(ctx context.Context) error {
			_, err := c.ListAuditors(ctx)
			return err
		}},
		{"GetGPUPrices", func(ctx context.Context) error {
			_, err := c.GetGPUPrices(ctx)
			return err
		}},
		{"ListTemplates", func(ctx context.Context) error {
			_, err := c.ListTemplates(ctx)
			return err
		}},
		{"GetTemplate", func(ctx context.Context) error {
			_, err := c.GetTemplate(ctx, "tpl-1")
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec.take() // drop anything left over from a previous subtest

			if err := tc.call(context.Background()); err != nil {
				t.Errorf("client call failed: %v", err)
			}

			for _, violation := range rec.take() {
				t.Errorf("contract violation: %s", violation)
			}
		})
	}
}

// TestContractHarnessDetectsViolations is the negative control: it replays
// the exact request shape that broke `akt console usage` in production (an
// empty startDate against a format=date query parameter) and asserts the
// harness flags it. If this fails, the contract suite has silently stopped
// validating and TestClientRequestsMatchOpenAPIContract proves nothing.
func TestContractHarnessDetectsViolations(t *testing.T) {
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC4122))

	router := loadContractRouter(t)
	rec := &contractRecorder{}
	srv := newContractServer(router, rec)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/usage/history?address=akash1x&startDate=&endDate=")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	violations := rec.take()
	if len(violations) == 0 {
		t.Fatal("expected the harness to flag empty startDate/endDate (format=date), but no violation was recorded")
	}
}
