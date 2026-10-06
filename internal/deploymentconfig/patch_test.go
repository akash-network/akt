package deploymentconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const deploymentSDL = `version: "2.0"
services:
  web:
    image: private.example/web:1
    command: [sh]
    args: [-c, start]
    env:
      - TOKEN=ac-secret://TOKEN
      - KEEP=plain
      - DROP=old
    credentials:
      host: private.example
      username: ac-secret://USER
      password: ac-secret://PASS
      email: owner@example.com
    expose:
      - port: 8080
        as: 80
        proto: tcp
        to: [{global: true}]
        accept: [old.example.com]
      - port: 9000
        as: 9000
        proto: tcp
        to: [{global: true}]
    params:
      storage:
        data: {mount: /old, readOnly: false}
profiles:
  compute:
    web:
      resources:
        cpu: {units: 1}
        memory: {size: 1Gi}
        storage: [{name: data, size: 1Gi}]
  placement:
    anywhere: {pricing: {web: {denom: uakt, amount: 1000}}}
deployment:
  web:
    anywhere: {profile: web, count: 1}
`

func TestParsePatchPreservesClearableFields(t *testing.T) {
	patch, err := ParsePatch([]byte(`services:
  web:
    command: null
    args: []
    credentials: null
    env: {DROP: null, ADD: value}
    expose:
      "8080": {accept: [], httpOptions: {nextCases: [off]}}
`))
	require.NoError(t, err)
	require.JSONEq(t, `{"services":{"web":{"command":null,"args":[],"credentials":null,"env":{"DROP":null,"ADD":"value"},"expose":{"8080":{"accept":[],"httpOptions":{"nextCases":["off"]}}}}}}`, mustJSON(t, patch))
	require.True(t, patch.HasChanges())
	require.NoError(t, Validate(Patch{}))
	require.False(t, Patch{}.HasChanges())
}

func TestParsePatchRejectsAmbiguousOrInvalidInputsWithoutValues(t *testing.T) {
	sentinel := "sensitive-value-do-not-echo"
	cases := map[string]string{
		"unknown field":          `{"services":{"web":{"unexpected":"` + sentinel + `"}}}`,
		"duplicate JSON":         `{"name":"` + sentinel + `","name":"again"}`,
		"duplicate YAML":         "name: " + sentinel + "\nname: again\n",
		"extra document":         "name: " + sentinel + "\n---\nname: again\n",
		"array root":             `["` + sentinel + `"]`,
		"numeric env":            `{"services":{"web":{"env":{"TOKEN":42}}}}`,
		"bad env key":            `{"services":{"web":{"env":{"1TOKEN":"` + sentinel + `"}}}}`,
		"credentials unknown":    `{"services":{"web":{"credentials":{"host":"h","username":"u","password":"` + sentinel + `","email":"x"}}}}`,
		"credentials missing":    `{"services":{"web":{"credentials":{"password":"` + sentinel + `"}}}}`,
		"null image":             `{"services":{"web":{"image":null}}}`,
		"null array element":     `{"services":{"web":{"command":[null]}}}`,
		"null accept":            `{"services":{"web":{"expose":{"8080":{"accept":null}}}}}`,
		"null services":          `{"services":null}`,
		"case variant":           `{"services":{"web":{"Image":"` + sentinel + `"}}}`,
		"service named env":      `{"services":{"env":{"image":null}}}`,
		"zero port":              `{"services":{"web":{"expose":{"8080":{"port":0}}}}}`,
		"large port":             `{"services":{"web":{"expose":{"8080":{"as":65536}}}}}`,
		"negative timeout":       `{"services":{"web":{"expose":{"8080":{"httpOptions":{"nextTimeout":-1}}}}}}`,
		"zero timeout":           `{"services":{"web":{"expose":{"8080":{"httpOptions":{"readTimeout":0}}}}}}`,
		"mixed off":              `{"services":{"web":{"expose":{"8080":{"httpOptions":{"nextCases":["off","timeout"]}}}}}}`,
		"empty retry conditions": `{"services":{"web":{"expose":{"8080":{"httpOptions":{"nextCases":[]}}}}}}`,
		"ciphertext":             `{"sealedSecrets":"` + sentinel + `"}`,
		"oversized":              "name: " + strings.Repeat("a", maxDocumentBytes),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParsePatch([]byte(input))
			require.Error(t, err)
			require.NotContains(t, err.Error(), sentinel)
		})
	}
}

func TestApplyEditsPreserveUnmentionedFieldsAndSecretReferences(t *testing.T) {
	patch, err := ParsePatch([]byte(`services:
  web:
    image: private.example/web:2
    command: null
    args: []
    env: {DROP: null, ADD: new, KEEP: changed}
    credentials: {host: private.example, username: "ac-secret://USER", password: "ac-secret://NEW_PASS"}
    expose:
      "8080": {port: 8081, accept: [], httpOptions: {readTimeout: 1000, nextTries: 0, nextCases: [timeout]}}
    storage:
      data: {mount: /new, readOnly: true}
`))
	require.NoError(t, err)
	updated, err := Apply(deploymentSDL, patch)
	require.NoError(t, err)
	doc, err := parseObject([]byte(updated))
	require.NoError(t, err)
	web := doc["services"].(map[string]any)["web"].(map[string]any)
	require.Equal(t, "private.example/web:2", web["image"])
	require.NotContains(t, web, "command")
	require.Equal(t, []any{}, web["args"])
	require.ElementsMatch(t, []any{"TOKEN=ac-secret://TOKEN", "ADD=new", "KEEP=changed"}, web["env"])
	credentials := web["credentials"].(map[string]any)
	require.Equal(t, "owner@example.com", credentials["email"])
	require.Equal(t, "ac-secret://NEW_PASS", credentials["password"])
	expose := web["expose"].([]any)[0].(map[string]any)
	require.Equal(t, float64(8081), expose["port"])
	require.Equal(t, float64(80), expose["as"])
	require.Equal(t, []any{}, expose["accept"])
	require.Equal(t, map[string]any{"read_timeout": float64(1000), "next_tries": float64(0), "next_cases": []any{"timeout"}}, expose["http_options"])
	require.Equal(t, map[string]any{"mount": "/new", "readOnly": true}, web["params"].(map[string]any)["storage"].(map[string]any)["data"])
	original, err := parseObject([]byte(deploymentSDL))
	require.NoError(t, err)
	require.Equal(t, original["profiles"], doc["profiles"])
	require.Equal(t, original["deployment"], doc["deployment"])
}

func TestApplyRejectsInvalidTargetAndPortChanges(t *testing.T) {
	cases := map[string]string{
		"missing service":     `{"services":{"absent":{"image":"changed"}}}`,
		"missing port":        `{"services":{"web":{"expose":{"1234":{"port":8081}}}}}`,
		"missing volume":      `{"services":{"web":{"storage":{"absent":{"mount":"/new"}}}}}`,
		"endpoint kind":       `{"services":{"web":{"expose":{"8080":{"as":81}}}}}`,
		"container collision": `{"services":{"web":{"expose":{"8080":{"port":9000}}}}}`,
		"external collision":  `{"services":{"web":{"expose":{"9000":{"as":80}}}}}`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			patch, err := ParsePatch([]byte(input))
			require.NoError(t, err)
			_, err = Apply(deploymentSDL, patch)
			require.Error(t, err)
		})
	}
	patch, err := ParsePatch([]byte(`{"services":{"web":{"expose":{"8080":{"port":8081}}}}}`))
	require.NoError(t, err)
	_, err = Apply(strings.Replace(deploymentSDL, "to: [{global: true}]", "to: [{global: true, ip: leased}]", 1), patch)
	require.ErrorContains(t, err, "leased IP")
	ambiguous := strings.Replace(deploymentSDL, "port: 9000", "port: 8080", 1)
	_, err = Apply(ambiguous, patch)
	require.ErrorContains(t, err, "multiple endpoints")
}

func TestApplyAllowsPortSwapsWhenFinalPortsAreDistinct(t *testing.T) {
	raw := strings.Replace(deploymentSDL, "as: 80", "as: 8080", 1)
	patch, err := ParsePatch([]byte(`{"services":{"web":{"expose":{"8080":{"port":9000,"as":9000},"9000":{"port":8080,"as":8080}}}}}`))
	require.NoError(t, err)
	_, err = Apply(raw, patch)
	require.NoError(t, err)
}

func TestYAMLNumericPortKeys(t *testing.T) {
	patch, err := ParsePatch([]byte("services:\n  web:\n    expose:\n      8080: {port: 8081}\n"))
	require.NoError(t, err)
	require.Contains(t, patch.Services["web"].Expose, "8080")
	_, err = ParsePatch([]byte("services:\n  web:\n    expose:\n      8080: {}\n      '8080': {}\n"))
	require.ErrorContains(t, err, "duplicate")
}

func TestDiffReproducesAllSupportedChanges(t *testing.T) {
	requested := strings.ReplaceAll(deploymentSDL, "private.example/web:1", "private.example/web:2")
	requested = strings.Replace(requested, "    command: [sh]\n", "", 1)
	requested = strings.Replace(requested, "    args: [-c, start]", "    args: []", 1)
	requested = strings.Replace(requested, "      - DROP=old", "      - ADDED=new", 1)
	requested = strings.Replace(requested, "password: ac-secret://PASS", "password: ac-secret://NEW_PASS", 1)
	requested = strings.Replace(requested, "port: 8080", "port: 8081", 1)
	requested = strings.Replace(requested, "accept: [old.example.com]", "accept: [new.example.com]\n        http_options: {next_tries: 0, read_timeout: 1000}", 1)
	requested = strings.Replace(requested, "mount: /old, readOnly: false", "mount: /new, readOnly: true", 1)
	patch, err := Diff(deploymentSDL, requested)
	require.NoError(t, err)
	require.True(t, patch.HasChanges())
	require.NotContains(t, patch.Services["web"].Env, "TOKEN")
	require.Nil(t, patch.Services["web"].Env["DROP"])
	require.Equal(t, "new", *patch.Services["web"].Env["ADDED"])
	updated, err := Apply(deploymentSDL, patch)
	require.NoError(t, err)
	actual, err := parseObject([]byte(updated))
	require.NoError(t, err)
	expected, err := parseObject([]byte(requested))
	require.NoError(t, err)
	normalizeSDL(actual)
	normalizeSDL(expected)
	require.Equal(t, expected, actual)
}

func TestDiffRejectsEveryUnrepresentableChange(t *testing.T) {
	cases := map[string]string{
		"resources":             strings.Replace(deploymentSDL, "units: 1", "units: 2", 1),
		"replicas":              strings.Replace(deploymentSDL, "count: 1", "count: 2", 1),
		"protocol":              strings.Replace(deploymentSDL, "proto: tcp", "proto: udp", 1),
		"routing":               strings.Replace(deploymentSDL, "global: true", "global: false", 1),
		"credentials metadata":  strings.Replace(deploymentSDL, "owner@example.com", "other@example.com", 1),
		"unknown service field": strings.Replace(deploymentSDL, "    image:", "    unexpected: true\n    image:", 1),
		"removed service":       strings.Replace(deploymentSDL, "  web:\n    image:", "  renamed:\n    image:", 1),
		"volume size":           strings.Replace(deploymentSDL, "size: 1Gi", "size: 2Gi", 1),
	}
	for name, requested := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Diff(deploymentSDL, requested)
			require.Error(t, err)
			require.Contains(t, err.Error(), "redeploy")
		})
	}
}

func TestNoopDiffAndSecretOnlyPatch(t *testing.T) {
	patch, err := Diff(deploymentSDL, deploymentSDL)
	require.NoError(t, err)
	require.False(t, patch.HasChanges())
	patch, err = ParsePatch([]byte(`{"services":{"web":{"env":{},"expose":{"8080":{"httpOptions":{}}}}}}`))
	require.NoError(t, err)
	require.False(t, patch.HasChanges())
	updated, err := Apply(deploymentSDL, patch)
	require.NoError(t, err)
	require.NotContains(t, updated, "http_options")
}

func TestReferencesAndAnchors(t *testing.T) {
	references, err := HasReferences(deploymentSDL)
	require.NoError(t, err)
	require.True(t, references)
	plain := `services: {web: {image: alpine}}
profiles:
  compute:
    web: &resources {cpu: {units: 1}}
    other: {<<: *resources}
`
	references, err = HasReferences(plain)
	require.NoError(t, err)
	require.False(t, references)
	patch, err := ParsePatch([]byte(`{"services":{"web":{"image":"alpine:latest"}}}`))
	require.NoError(t, err)
	_, err = Apply(plain, patch)
	require.NoError(t, err)
	_, err = Apply("services:\n  web: &shared {image: alpine}\n  other: *shared\n", patch)
	require.ErrorContains(t, err, "anchors")
	sharedEnv := "defaults: &env [TOKEN=ac-secret://TOKEN]\nservices:\n  web:\n    image: alpine\n    env: *env\n"
	_, err = Apply(sharedEnv, patch)
	require.NoError(t, err)
	envPatch, err := ParsePatch([]byte(`{"services":{"web":{"env":{"TOKEN":null}}}}`))
	require.NoError(t, err)
	_, err = Apply(sharedEnv, envPatch)
	require.ErrorContains(t, err, "anchors")
	_, err = HasReferences("services: &recursive {web: *recursive}")
	require.ErrorContains(t, err, "nesting")
	_, err = HasReferences("services: [")
	require.Error(t, err)
}

func TestApplyRemovesAllVariablesAndCredentials(t *testing.T) {
	patch, err := ParsePatch([]byte(`{"services":{"web":{"env":{"TOKEN":null,"KEEP":null,"DROP":null},"credentials":null}}}`))
	require.NoError(t, err)
	updated, err := Apply(deploymentSDL, patch)
	require.NoError(t, err)
	doc, err := parseObject([]byte(updated))
	require.NoError(t, err)
	web := doc["services"].(map[string]any)["web"].(map[string]any)
	require.NotContains(t, web, "env")
	require.NotContains(t, web, "credentials")
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}
