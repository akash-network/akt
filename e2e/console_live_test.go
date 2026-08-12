package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// envConsoleLiveKey opts the real Console read suite in. The sandbox URL is a
// separate required input so the same contracts never default to production.
const (
	envConsoleLiveKey         = "AKT_E2E_CONSOLE_API_KEY"
	envConsoleLiveAPIURL      = "AKT_E2E_CONSOLE_API_URL"
	consoleLiveCommandTimeout = 45 * time.Second
)

type consoleLiveShape string

const (
	consoleLiveObject consoleLiveShape = "object"
	consoleLiveArray  consoleLiveShape = "array"
	consoleLiveString consoleLiveShape = "string"
	consoleLiveNumber consoleLiveShape = "number"
	consoleLiveBool   consoleLiveShape = "boolean"
)

type consoleLiveFieldContract struct {
	name     string
	contract consoleLiveJSONContract
}

type consoleLiveJSONContract struct {
	shape         consoleLiveShape
	minItems      int
	minProperties int
	nonEmpty      bool
	nullable      bool
	fields        []consoleLiveFieldContract
	item          *consoleLiveJSONContract
	values        *consoleLiveJSONContract
}

type consoleLiveCase struct {
	name     string
	args     []string
	contract consoleLiveJSONContract
	validate func(json.RawMessage) error
}

// TestConsoleLive executes safe reads against the real Console API. Success
// means the command returned its documented JSON shape, not merely that the
// process exited zero or produced syntactically valid JSON.
func TestConsoleLive(t *testing.T) {
	key := os.Getenv(envConsoleLiveKey)
	if key == "" {
		t.Skipf("skipping live Console API smoke tests: %s is not set", envConsoleLiveKey)
	}
	t.Setenv(aktctx.EnvConsoleAPIKey, key)

	home, apiURL := setupConsoleLiveHome(t, key, false)
	providerRegionSchema, err := loadConsoleLiveProviderRegionSchema(t.Context(), apiURL)
	if err != nil {
		t.Fatalf("load independent Console provider-region schema: %v", err)
	}

	nonEmptyString := consoleLiveJSONContract{shape: consoleLiveString, nonEmpty: true}
	numberValue := consoleLiveJSONContract{shape: consoleLiveNumber}
	boolValue := consoleLiveJSONContract{shape: consoleLiveBool}
	requireProviderMembership := os.Getenv(envConsoleMutationOptIn) == consoleMutationOptInValue

	tests := []consoleLiveCase{
		{
			name: "whoami",
			args: []string{"console", "whoami"},
			contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
				{name: "username", contract: nonEmptyString},
				{name: "email", contract: nonEmptyString},
				{name: "emailVerified", contract: boolValue},
			}},
		},
		{
			name: "wallet balance",
			args: []string{"console", "wallet", "balance"},
			contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
				{name: "available", contract: nonEmptyString},
				{name: "inDeployments", contract: nonEmptyString},
				{name: "total", contract: nonEmptyString},
				{name: "allocationStatus", contract: nonEmptyString},
				{name: "allocationNote", contract: nonEmptyString},
			}},
		},
		{
			name: "wallet settings",
			args: []string{"console", "wallet", "settings"},
			contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
				{name: "autoReloadEnabled", contract: boolValue},
				{name: "configured", contract: boolValue},
			}},
		},
		{
			name: "wallet cost",
			args: []string{"console", "wallet", "cost"},
			contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
				{name: "weeklyCost", contract: nonEmptyString},
			}},
		},
		// No dates: this catches the historical regression where the client sent
		// empty startDate/endDate parameters and Console rejected the request.
		{
			name: "usage",
			args: []string{"console", "usage"},
			contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
				{name: "totalSpent", contract: nonEmptyString},
				{name: "days", contract: numberValue},
				{name: "history", contract: consoleLiveJSONContract{
					shape: consoleLiveArray,
					item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
						{name: "date", contract: nonEmptyString},
						{name: "deployments", contract: numberValue},
						{name: "spent", contract: nonEmptyString},
					}},
				}},
			}},
		},
		{
			name: "provider regions",
			args: []string{"console", "provider", "regions"},
			contract: consoleLiveJSONContract{
				shape:    consoleLiveArray,
				minItems: 1,
				item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "key", contract: nonEmptyString},
					{name: "description", contract: nonEmptyString},
					{name: "providers", contract: consoleLiveJSONContract{
						shape: consoleLiveArray,
						item:  &nonEmptyString,
					}},
				}},
			},
			validate: func(raw json.RawMessage) error {
				return validateConsoleLiveProviderRegions(raw, providerRegionSchema, requireProviderMembership)
			},
		},
		{
			name: "gpu",
			args: []string{"console", "gpu"},
			contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
				{name: "availability", contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "total", contract: numberValue},
					{name: "available", contract: numberValue},
				}}},
				{name: "models", contract: consoleLiveJSONContract{
					shape:    consoleLiveArray,
					minItems: 1,
					item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
						{name: "vendor", contract: nonEmptyString},
						{name: "model", contract: nonEmptyString},
						{name: "price", contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
							{name: "min", contract: numberValue},
							{name: "max", contract: numberValue},
							{name: "avg", contract: numberValue},
						}}},
					}},
				}},
			}},
		},
		// Console envelopes the catalog as data: Category[]. The client unwraps
		// data, so the public command returns the category array directly.
		{
			name: "template list",
			args: []string{"console", "template", "list"},
			contract: consoleLiveJSONContract{
				shape:    consoleLiveArray,
				minItems: 1,
				item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "title", contract: nonEmptyString},
					{name: "templates", contract: consoleLiveJSONContract{
						shape: consoleLiveArray,
						item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
							{name: "id", contract: nonEmptyString},
						}},
					}},
				}},
			},
			validate: validateConsoleLiveTemplateCatalog,
		},
		{
			name: "deployment list",
			args: []string{"console", "deployment", "list"},
			contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
				{name: "deployments", contract: consoleLiveJSONContract{
					shape: consoleLiveArray,
					item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
						{name: "deployment", contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
							{name: "id", contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
								{name: "owner", contract: nonEmptyString},
								{name: "dseq", contract: nonEmptyString},
							}}},
							{name: "state", contract: nonEmptyString},
							{name: "hash", contract: nonEmptyString},
						}}},
						{name: "leases", contract: consoleLiveJSONContract{shape: consoleLiveArray}},
					}},
				}},
				{name: "pagination", contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "total", contract: numberValue},
					{name: "skip", contract: numberValue},
					{name: "limit", contract: numberValue},
					{name: "hasMore", contract: boolValue},
				}}},
			}},
		},
		{
			name: "provider list",
			args: []string{"console", "provider", "list", "--limit", "0"},
			contract: consoleLiveJSONContract{
				shape:    consoleLiveArray,
				minItems: 1,
				item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "owner", contract: nonEmptyString},
				}},
			},
		},
		{
			name: "provider auditors",
			args: []string{"console", "provider", "auditors"},
			contract: consoleLiveJSONContract{
				shape:    consoleLiveArray,
				minItems: 1,
				item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "id", contract: nonEmptyString},
					{name: "name", contract: nonEmptyString},
					{name: "address", contract: nonEmptyString},
				}},
			},
		},
		{
			name: "wallet list",
			args: []string{"console", "wallet", "list"},
			contract: consoleLiveJSONContract{
				shape:    consoleLiveArray,
				minItems: 1,
				item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "address", contract: nonEmptyString},
					{name: "balance", contract: nonEmptyString},
					{name: "trialing", contract: boolValue},
				}},
			},
		},
		// Lists metadata only; key material is returned only by the mutating create.
		{
			name: "apikey list",
			args: []string{"console", "apikey", "list"},
			contract: consoleLiveJSONContract{
				shape: consoleLiveArray,
				item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "id", contract: nonEmptyString},
					{name: "name", contract: nonEmptyString},
				}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := "akt " + strings.Join(test.args, " ")
			result := runConsoleLiveAkt(t, home, test.args...)
			raw := requireConsoleLiveContract(t, command, result, test.contract)
			if test.validate != nil {
				if err := test.validate(raw); err != nil {
					t.Fatalf("%s response failed its live invariant: %v", command, err)
				}
			}
		})
	}
	assertConsoleActions(t, home, "prod", "")
}

func TestConsoleLiveUsesConfiguredAPIURL(t *testing.T) {
	const key = "sandbox-routing-key"
	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != key {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		requestPath <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"sandbox-user","username":"sandbox","email":"sandbox@example.test","emailVerified":true}}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv(aktctx.EnvConsoleAPIKey, key)
	t.Setenv(envConsoleLiveAPIURL, server.URL)

	home, _ := setupConsoleLiveHome(t, key, true)
	result := runConsoleLiveAkt(t, home, "console", "whoami")
	requireConsoleSuccess(t, result, "akt console whoami")

	select {
	case got := <-requestPath:
		if got != "/v1/user/me" {
			t.Fatalf("Console read reached %q, want /v1/user/me", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Console read did not reach the configured API URL")
	}
}

// activeConsoleDSeq returns an exact active deployment identity that also has
// an active lease. It uses the public CLI response because this suite tests
// read compatibility only; mutation post-state uses the independent observer.
func activeConsoleDSeq(t *testing.T, home string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), consoleLiveCommandTimeout)
	defer cancel()
	deployments, err := listAllConsoleDeployments(ctx, t, home)
	if err != nil {
		t.Fatalf("list every Console deployment page: %v", err)
	}
	for _, deployment := range deployments {
		if !strings.EqualFold(deployment.Deployment.State, "active") || deployment.Deployment.ID.DSeq.String() == "" {
			continue
		}
		for _, lease := range deployment.Leases {
			if strings.EqualFold(lease.State, "active") {
				return deployment.Deployment.ID.DSeq.String()
			}
		}
	}
	return ""
}

// TestConsoleLiveDeploymentReads covers safe reads that need existing state.
func TestConsoleLiveDeploymentReads(t *testing.T) {
	key := os.Getenv(envConsoleLiveKey)
	if key == "" {
		t.Skipf("skipping live Console API smoke tests: %s is not set", envConsoleLiveKey)
	}

	t.Setenv(aktctx.EnvConsoleAPIKey, key)
	home, _ := setupConsoleLiveHome(t, key, false)

	dseq := activeConsoleDSeq(t, home)
	if dseq == "" {
		t.Skip("account has no active deployment with an active lease")
	}

	exerciseConsoleLiveDeploymentReads(t.Context(), t, home, dseq)
	assertConsoleActions(t, home, "prod", "")
}

func exerciseConsoleLiveDeploymentReads(parent context.Context, t *testing.T, home, dseq string) {
	t.Helper()

	t.Run("deployment get", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(parent, consoleLiveCommandTimeout)
		defer cancel()
		detail, _, err := getConsoleDeployment(ctx, t, home, dseq)
		if err != nil {
			t.Fatal(err)
		}
		if detail.Deployment.ID.DSeq.String() != dseq {
			t.Fatalf("akt console deployment get returned dseq %s, want %s", detail.Deployment.ID.DSeq, dseq)
		}
	})

	t.Run("status", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(parent, consoleLiveCommandTimeout)
		defer cancel()
		result := runConsoleAkt(ctx, t, home, "console", "status", dseq)
		requireConsoleLiveContract(t, "akt console status "+dseq, result, consoleProviderStatusContract())
	})

	t.Run("bid list", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(parent, consoleLiveCommandTimeout)
		defer cancel()
		result := runConsoleAkt(ctx, t, home, "console", "bid", "list", dseq)
		// A deployment selected above already has a lease, so its bid list must
		// contain at least the accepted bid rather than the command's prose
		// no-bids acknowledgement.
		nonEmptyString := consoleLiveJSONContract{shape: consoleLiveString, nonEmpty: true}
		numberValue := consoleLiveJSONContract{shape: consoleLiveNumber}
		raw := requireConsoleLiveContract(t, "akt console bid list "+dseq, result, consoleLiveJSONContract{
			shape:    consoleLiveArray,
			minItems: 1,
			item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
				{name: "id", contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "owner", contract: nonEmptyString},
					{name: "dseq", contract: nonEmptyString},
					{name: "gseq", contract: numberValue},
					{name: "oseq", contract: numberValue},
					{name: "provider", contract: nonEmptyString},
				}}},
				{name: "state", contract: nonEmptyString},
				{name: "price", contract: consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "denom", contract: nonEmptyString},
					{name: "amount", contract: nonEmptyString},
				}}},
			}},
		})
		var bids []struct {
			ID struct {
				DSeq consoleFlexibleID `json:"dseq"`
			} `json:"id"`
		}
		if err := json.Unmarshal(raw, &bids); err != nil {
			t.Fatalf("decode validated bid list for deployment identity: %v", err)
		}
		for index, bid := range bids {
			if bid.ID.DSeq.String() != dseq {
				t.Fatalf("akt console bid list item %d returned dseq %s, want %s", index, bid.ID.DSeq, dseq)
			}
		}
	})

	// A one-shot log fetch ends normally when the provider closes the stream.
	t.Run("logs exit status", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(parent, consoleLiveCommandTimeout)
		defer cancel()
		result := runConsoleAkt(ctx, t, home, "console", "logs", dseq)
		requireConsoleSuccess(t, result, "akt console logs "+dseq)
	})
}

func consoleProviderStatusContract() consoleLiveJSONContract {
	nonEmptyString := consoleLiveJSONContract{shape: consoleLiveString, nonEmpty: true}
	numberValue := consoleLiveJSONContract{shape: consoleLiveNumber}
	service := consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
		{name: "name", contract: nonEmptyString},
		{name: "available", contract: numberValue},
		{name: "total", contract: numberValue},
		{name: "uris", contract: consoleLiveJSONContract{
			shape: consoleLiveArray,
			item:  &nonEmptyString,
		}},
		{name: "observed_generation", contract: numberValue},
		{name: "replicas", contract: numberValue},
		{name: "updated_replicas", contract: numberValue},
		{name: "ready_replicas", contract: numberValue},
		{name: "available_replicas", contract: numberValue},
	}}
	return consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
		{name: "services", contract: consoleLiveJSONContract{
			shape:         consoleLiveObject,
			minProperties: 1,
			values:        &service,
		}},
		{name: "forwarded_ports", contract: consoleLiveJSONContract{shape: consoleLiveObject, nullable: true}},
		{name: "ips", contract: consoleLiveJSONContract{shape: consoleLiveObject, nullable: true}},
	}}
}

func setupConsoleLiveHome(t *testing.T, key string, allowLoopback bool) (string, string) {
	t.Helper()
	apiURL, err := validateConsoleMutationEndpoint(os.Getenv(envConsoleLiveAPIURL), allowLoopback)
	if err != nil {
		t.Fatalf("unsafe live Console endpoint: %v", err)
	}
	if err := validateConsoleCredentialEndpoint(apiURL, key, allowLoopback); err != nil {
		t.Fatalf("invalid live Console credential/endpoint pairing: %v", err)
	}

	home := t.TempDir()
	initHome(t, home)
	t.Cleanup(func() { assertConsoleSecretAbsent(t, home, "prod", key) })
	args := []string{
		"context", "create", "prod",
		"--auth-method", "console-api",
		"--set-current",
	}
	args = append(args, "--console-api-url", apiURL)

	result := runConsoleLiveAkt(t, home, args...)
	requireConsoleSuccess(t, result, "akt context create for live Console tests")
	return home, apiURL
}

func runConsoleLiveAkt(t *testing.T, home string, args ...string) consoleCommandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), consoleLiveCommandTimeout)
	defer cancel()
	return runConsoleAkt(ctx, t, home, args...)
}

func requireConsoleLiveContract(t *testing.T, command string, result consoleCommandResult, contract consoleLiveJSONContract) json.RawMessage {
	t.Helper()
	requireConsoleSuccess(t, result, command)

	var raw json.RawMessage
	if err := decodeConsoleJSONDocument([]byte(result.Stdout), &raw); err != nil {
		t.Fatalf("%s did not return one JSON document (%d bytes): %v", command, result.StdoutBytes, err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		t.Fatalf("%s returned an empty JSON document", command)
	}
	requireConsoleLiveJSONContract(t, command+" response", raw, contract)
	return raw
}

func loadConsoleLiveProviderRegionSchema(ctx context.Context, apiURL string) (map[string]string, error) {
	var schema struct {
		LocationRegion struct {
			Key    string `json:"key"`
			Type   string `json:"type"`
			Values []struct {
				Key         string `json:"key"`
				Description string `json:"description"`
			} `json:"values"`
		} `json:"location-region"`
	}
	observer := newConsoleAPIObserver(apiURL, "")
	if err := observer.getJSON(ctx, "/v1/provider-attributes-schema", &schema); err != nil {
		return nil, err
	}
	if schema.LocationRegion.Key != "location-region" || schema.LocationRegion.Type != "option" {
		return nil, errors.New("provider attributes schema has an invalid location-region definition")
	}
	if len(schema.LocationRegion.Values) == 0 {
		return nil, errors.New("provider attributes schema has no location-region values")
	}

	regions := make(map[string]string, len(schema.LocationRegion.Values))
	for index, value := range schema.LocationRegion.Values {
		if strings.TrimSpace(value.Key) == "" || strings.TrimSpace(value.Description) == "" {
			return nil, fmt.Errorf("provider attributes schema location-region value %d has an empty identity", index)
		}
		if _, exists := regions[value.Key]; exists {
			return nil, fmt.Errorf("provider attributes schema repeats location-region key %q", value.Key)
		}
		regions[value.Key] = value.Description
	}
	return regions, nil
}

func validateConsoleLiveProviderRegions(raw json.RawMessage, expected map[string]string, requireMembership bool) error {
	var regions []struct {
		Key         string   `json:"key"`
		Description string   `json:"description"`
		Providers   []string `json:"providers"`
	}
	if err := json.Unmarshal(raw, &regions); err != nil {
		return fmt.Errorf("decode provider-region catalog: %w", err)
	}
	if len(regions) != len(expected) {
		return fmt.Errorf("provider-region catalog has %d regions, attributes schema has %d", len(regions), len(expected))
	}

	seen := make(map[string]struct{}, len(regions))
	memberships := 0
	for index, region := range regions {
		if _, exists := seen[region.Key]; exists {
			return fmt.Errorf("provider-region catalog repeats key %q at index %d", region.Key, index)
		}
		seen[region.Key] = struct{}{}
		expectedDescription, exists := expected[region.Key]
		if !exists {
			return fmt.Errorf("provider-region catalog contains key %q absent from the attributes schema", region.Key)
		}
		if region.Description != expectedDescription {
			return fmt.Errorf("provider-region catalog description for %q differs from the attributes schema", region.Key)
		}

		providers := make(map[string]struct{}, len(region.Providers))
		for _, provider := range region.Providers {
			if err := localnetSemanticValidateAddress(provider); err != nil {
				return fmt.Errorf("provider-region catalog %q membership: %w", region.Key, err)
			}
			if _, exists := providers[provider]; exists {
				return fmt.Errorf("provider-region catalog %q repeats provider %q", region.Key, provider)
			}
			providers[provider] = struct{}{}
			memberships++
		}
	}
	if requireMembership && memberships == 0 {
		return errors.New("provider-region catalog has no provider memberships; sandbox is not deployment-ready")
	}

	return nil
}

func validateConsoleLiveTemplateCatalog(raw json.RawMessage) error {
	var categories []struct {
		Title     string `json:"title"`
		Templates []struct {
			ID string `json:"id"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(raw, &categories); err != nil {
		return fmt.Errorf("decode template catalog: %w", err)
	}

	seenCategories := make(map[string]struct{}, len(categories))
	templateCount := 0
	for _, category := range categories {
		if strings.TrimSpace(category.Title) == "" {
			return errors.New("template catalog contains an empty category title")
		}
		if _, exists := seenCategories[category.Title]; exists {
			return fmt.Errorf("template catalog repeats category title %q", category.Title)
		}
		seenCategories[category.Title] = struct{}{}
		seenTemplates := make(map[string]struct{}, len(category.Templates))
		for _, template := range category.Templates {
			if strings.TrimSpace(template.ID) == "" {
				return fmt.Errorf("template catalog category %q contains an empty template ID", category.Title)
			}
			if _, exists := seenTemplates[template.ID]; exists {
				return fmt.Errorf("template catalog repeats template ID %q", template.ID)
			}
			seenTemplates[template.ID] = struct{}{}
			templateCount++
		}
	}
	if templateCount == 0 {
		return errors.New("template catalog has no templates")
	}
	return nil
}

func requireConsoleLiveJSONContract(t *testing.T, path string, raw json.RawMessage, contract consoleLiveJSONContract) {
	t.Helper()
	if err := validateConsoleLiveJSONContract(path, raw, contract); err != nil {
		t.Fatal(err)
	}
}

func validateConsoleLiveJSONContract(path string, raw json.RawMessage, contract consoleLiveJSONContract) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" && contract.nullable {
		return nil
	}
	if trimmed == "" || trimmed == "null" {
		return fmt.Errorf("%s is empty or null", path)
	}

	switch contract.shape {
	case consoleLiveArray:
		if trimmed[0] != '[' {
			return fmt.Errorf("%s is %s, want a JSON array", path, jsonKind(trimmed[0]))
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return fmt.Errorf("%s array decode failed: %w", path, err)
		}
		if len(items) < contract.minItems {
			return fmt.Errorf("%s has %d items, want at least %d", path, len(items), contract.minItems)
		}
		if contract.item != nil {
			for index, item := range items {
				if err := validateConsoleLiveJSONContract(fmt.Sprintf("%s[%d]", path, index), item, *contract.item); err != nil {
					return err
				}
			}
		}
	case consoleLiveObject:
		if trimmed[0] != '{' {
			return fmt.Errorf("%s is %s, want a JSON object", path, jsonKind(trimmed[0]))
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return fmt.Errorf("%s object decode failed: %w", path, err)
		}
		if len(object) < contract.minProperties {
			return fmt.Errorf("%s has %d properties, want at least %d", path, len(object), contract.minProperties)
		}
		for _, field := range contract.fields {
			value, exists := object[field.name]
			if !exists {
				return fmt.Errorf("%s omitted required field %q", path, field.name)
			}
			if err := validateConsoleLiveJSONContract(path+"."+field.name, value, field.contract); err != nil {
				return err
			}
		}
		if contract.values != nil {
			for name, value := range object {
				if err := validateConsoleLiveJSONContract(path+"."+name, value, *contract.values); err != nil {
					return err
				}
			}
		}
	case consoleLiveString:
		if trimmed[0] != '"' {
			return fmt.Errorf("%s is %s, want a JSON string", path, jsonKind(trimmed[0]))
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%s string decode failed: %w", path, err)
		}
		if contract.nonEmpty && strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is an empty string", path)
		}
	case consoleLiveNumber:
		if (trimmed[0] < '0' || trimmed[0] > '9') && trimmed[0] != '-' {
			return fmt.Errorf("%s is %s, want a JSON number", path, jsonKind(trimmed[0]))
		}
		var value json.Number
		if err := json.Unmarshal(raw, &value); err != nil || value.String() == "" {
			return fmt.Errorf("%s is not a JSON number", path)
		}
	case consoleLiveBool:
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%s is not a JSON boolean", path)
		}
	default:
		return fmt.Errorf("%s has unsupported expected JSON shape %q", path, contract.shape)
	}
	return nil
}

func jsonKind(first byte) string {
	switch first {
	case '{':
		return "a JSON object"
	case '[':
		return "a JSON array"
	case '"':
		return "a JSON string"
	case 't', 'f':
		return "a JSON boolean"
	case 'n':
		return "JSON null"
	default:
		return fmt.Sprintf("a JSON scalar beginning with %q", first)
	}
}
