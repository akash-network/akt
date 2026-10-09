package deploymentconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPatchBoundaryRejections(t *testing.T) {
	for name, input := range map[string]string{
		"empty name": `name: " "`, "long name": `name: ` + strings.Repeat("x", 257),
		"long version":   `ifManifestVersion: ` + strings.Repeat("x", 65),
		"service scalar": `services: {web: broken}`, "expose scalar": `services: {web: {expose: {8080: broken}}}`,
		"storage scalar": `services: {web: {storage: {data: broken}}}`, "storage unknown": `services: {web: {storage: {data: {size: 2Gi}}}}`,
		"options unknown": `services: {web: {expose: {8080: {httpOptions: {bad: 1}}}}}`,
		"bad array":       `services: {web: {command: 7}}`, "bad env": `services: {web: {env: {BAD/NAME: value}}}`,
		"large body":   `services: {web: {expose: {8080: {httpOptions: {maxBodySize: 104857601}}}}}`,
		"bad retry":    `services: {web: {expose: {8080: {httpOptions: {nextCases: [unknown]}}}}}`,
		"merge scalar": `services: {web: {<<: 1}}`, "merge recursive": `name: &x {<<: *x}`,
		"invalid key": `true: value`, "sequence key": `? [a,b]\n: value`,
		"unsupported tag": `name: !!binary eA==`, "invalid scalar": `name: !!int nope`,
		"infinite scalar": `name: .inf`, "bad sequence element": `services: [!!int nope]`,
		"huge expansion": "name: [" + strings.Repeat("x,", 100001) + "]",
	} {
		t.Run(name, func(t *testing.T) { _, err := ParsePatch([]byte(input)); require.Error(t, err) })
	}
	// Programmatic callers must obey the same clearable-field contract as files.
	for _, raw := range []string{`false`, `{}`, `[1]`, `[null]`, `["x"] {}`} {
		require.Error(t, Validate(Patch{Services: map[string]ServicePatch{"web": {Command: json.RawMessage(raw)}}}))
	}
	require.NoError(t, Validate(Patch{Services: map[string]ServicePatch{"web": {Command: json.RawMessage(`[]`), Args: json.RawMessage(`null`)}}}))
	require.ErrorContains(t, decodeStrict([]byte(`{} {}`), new(Patch)), "trailing")
}

func TestYAMLMergePrecedenceAndNumericKeys(t *testing.T) {
	patch, err := ParsePatch([]byte(`defaults: value`))
	require.Error(t, err)
	require.False(t, patch.HasChanges())
	doc, err := parseObject([]byte(`first: &a {x: first, y: first}
second: &b {x: second, z: second}
merged: {<<: [*a, *b], y: explicit}
ports: {0x10: value}
`))
	require.NoError(t, err)
	require.Equal(t, map[string]any{"x": "first", "y": "explicit", "z": "second"}, doc["merged"])
	require.Equal(t, map[string]any{"16": "value"}, doc["ports"])
	_, err = parseObject([]byte("!!int nope: value"))
	require.ErrorContains(t, err, "invalid scalar")
}

func TestApplyRejectsInvalidDocumentsAndPreservesUntouchedValues(t *testing.T) {
	invalid := Patch{Services: map[string]ServicePatch{"web": {Args: json.RawMessage(`false`)}}}
	_, err := Apply(deploymentSDL, invalid)
	require.Error(t, err)
	for _, sdl := range []string{"[", "version: 2", "services: []"} {
		_, err = Apply(sdl, Patch{})
		require.Error(t, err)
	}
	patch, err := ParsePatch([]byte(`services: {web: {env: {NEW: value}}}`))
	require.NoError(t, err)
	updated, err := Apply("services: {web: {env: [12, 'FLAG', 'SAME=yes']}}", patch)
	require.NoError(t, err)
	doc, err := parseObject([]byte(updated))
	require.NoError(t, err)
	require.Equal(t, []any{float64(12), "FLAG", "SAME=yes", "NEW=value"}, doc["services"].(map[string]any)["web"].(map[string]any)["env"])
	huge := strings.Repeat("x", maxDocumentBytes)
	_, err = Apply("services: {web: {image: old}}", Patch{Services: map[string]ServicePatch{"web": {Image: &huge}}})
	require.ErrorContains(t, err, "exceeds")
}

func TestPatchPortAndStorageOnlyChanges(t *testing.T) {
	for _, input := range []string{
		`services: {web: {expose: {9000: {as: 9001}}}}`,
		`services: {web: {expose: {8080: {httpOptions: {maxBodySize: 100, sendTimeout: 10, nextTimeout: 20}}}}}`,
		`services: {web: {storage: {data: {readOnly: true}}}}`,
	} {
		patch, err := ParsePatch([]byte(input))
		require.NoError(t, err)
		require.True(t, patch.HasChanges())
		_, err = Apply(deploymentSDL, patch)
		require.NoError(t, err)
	}
	patch, err := ParsePatch([]byte(`services: {web: {expose: {8080: {port: 8081}}}}`))
	require.NoError(t, err)
	_, err = Apply("services: {web: {expose: [broken]}}", patch)
	require.ErrorContains(t, err, "objects")
	_, err = Apply("services: {web: {expose: [{port: missing}]}}", patch)
	require.ErrorContains(t, err, "not declared")
	updated, err := Apply("services: {web: {expose: [{port: 8080, to: [{service: other}]}]}}", patch)
	require.NoError(t, err)
	require.Contains(t, updated, "8081")
	require.Equal(t, "internal", endpointKind(map[string]any{}))
	require.Equal(t, float64(8080), externalPort(map[string]any{"port": float64(8080)}))
	require.False(t, hasLeasedIP(map[string]any{}))
}

func TestApplyRefusesSharedNestedTargets(t *testing.T) {
	for _, tc := range []struct{ name, sdl, patch, want string }{
		{"expose list", "endpoints: &x [{port: 8080}]\nservices: {web: {expose: *x}}", `services: {web: {expose: {8080: {port: 8081}}}}`, "patched endpoints"},
		{"endpoint", "endpoint: &x {port: 8080}\nservices: {web: {expose: [*x]}}", `services: {web: {expose: {8080: {port: 8081}}}}`, "patched endpoint"},
		{"http options", "options: &x {read_timeout: 10}\nservices: {web: {expose: [{port: 8080, http_options: *x}]}}", `services: {web: {expose: {8080: {httpOptions: {readTimeout: 20}}}}}`, "patched endpoint"},
		{"storage list", "volumes: &x {data: {mount: /data}}\nservices: {web: {params: {storage: *x}}}", `services: {web: {storage: {data: {mount: /new}}}}`, "patched storage"},
		{"volume", "volume: &x {mount: /data}\nservices: {web: {params: {storage: {data: *x}}}}", `services: {web: {storage: {data: {mount: /new}}}}`, "patched volume"},
		{"credentials", "login: &x {host: registry, username: u, password: p}\nservices: {web: {credentials: *x}}", `services: {web: {credentials: null}}`, "credentials"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patch, err := ParsePatch([]byte(tc.patch))
			require.NoError(t, err)
			_, err = Apply(tc.sdl, patch)
			require.ErrorContains(t, err, tc.want)
		})
	}
	// Shared endpoints outside the selected port remain untouched.
	patch, err := ParsePatch([]byte(`services: {web: {expose: {9000: {port: 9001}}}}`))
	require.NoError(t, err)
	_, err = Apply("services: {web: {expose: [{as: 80}, {port: 8080}, {port: 9000}]}}", patch)
	require.NoError(t, err)
}

func TestDiffRejectsMalformedAndUnrepresentableSDL(t *testing.T) {
	for _, tc := range []struct{ name, current, desired string }{
		{"current syntax", "[", deploymentSDL}, {"desired syntax", deploymentSDL, "["}, {"missing services", "version: 2", "version: 2"},
		{"remove image", "services: {web: {image: old}}", "services: {web: {}}"},
		{"bad credentials", "services: {web: {image: old}}", "services: {web: {image: old, credentials: bad}}"},
		{"missing credentials fields", "services: {web: {}}", "services: {web: {credentials: {host: registry}}}"},
		{"old bad env", "services: {web: {env: bad}}", "services: {web: {env: []}}"},
		{"new bad env", "services: {web: {env: []}}", "services: {web: {env: bad}}"},
		{"env scalar", "services: {web: {env: [1]}}", "services: {web: {env: []}}"},
		{"env duplicates", "services: {web: {env: ['X=1','X=2']}}", "services: {web: {env: []}}"},
		{"bare env added", "services: {web: {}}", "services: {web: {env: [BARE]}}"},
		{"bad env name", "services: {web: {}}", "services: {web: {env: ['1X=v']}}"},
		{"expose count", "services: {web: {expose: []}}", "services: {web: {expose: [{port: 80}]}}"},
		{"expose scalar", "services: {web: {expose: [bad]}}", "services: {web: {expose: [{port: 80}]}}"},
		{"missing old port", "services: {web: {expose: [{as: 90}]}}", "services: {web: {expose: [{port: 8080, as: 90}]}}"},
		{"duplicate port", "services: {web: {expose: [{port: 9000},{port: 9000}]}}", "services: {web: {expose: [{port: 9001},{port: 9002}]}}"},
		{"kind changed", "services: {web: {expose: [{port: 80,to: [{global: true}]}]}}", "services: {web: {expose: [{port: 81,to: [{global: true}]}]}}"},
		{"storage add", "services: {web: {}}", "services: {web: {params: {storage: {data: {mount: /new}}}}}"},
		{"invalid image type", "services: {web: {image: old}}", "services: {web: {image: 3}}"},
	} {
		t.Run(tc.name, func(t *testing.T) { _, err := Diff(tc.current, tc.desired); require.Error(t, err) })
	}
	patch, err := Diff("services: {web: {env: [BARE]}}", "services: {web: {env: ['BARE=value']}}")
	require.NoError(t, err)
	require.Equal(t, "value", *patch.Services["web"].Env["BARE"])
	_, err = Diff(deploymentSDL, strings.Replace(deploymentSDL, "    command: [sh]", "    command: [sh]\n    extra: unsupported", 1))
	require.ErrorContains(t, err, "redeploy")
	doc := map[string]any{"services": map[string]any{"invalid": false, "web": map[string]any{"env": []any{}}}}
	normalizeSDL(doc)
	require.NotContains(t, doc["services"].(map[string]any)["web"], "env")
}

func TestApplyRefusesAliasedServiceCollections(t *testing.T) {
	patch, err := ParsePatch([]byte(`services: {web: {image: changed}}`))
	require.NoError(t, err)
	for _, sdl := range []string{
		"shared: &x {web: {image: old}}\nservices: *x",
		"shared: &x {services: {web: {image: old}}}\n<<: *x",
		"services: &x {web: {image: old}}\nshared: *x",
	} {
		_, err = Apply(sdl, patch)
		require.ErrorContains(t, err, "anchors")
	}
}

func TestRemainingConfigurationBoundaries(t *testing.T) {
	patch, err := ParsePatch([]byte(`services: {web: {credentials: {host: registry, username: user, password: pass}}}`))
	require.NoError(t, err)
	updated, err := Apply("services: {web: {image: app}}", patch)
	require.NoError(t, err)
	require.Contains(t, updated, "password: pass")
	patch, err = ParsePatch([]byte(`services: {web: {expose: {9000: {as: 9001}}}}`))
	require.NoError(t, err)
	_, err = Apply("services: {web: {expose: [{port: 9000, to: [{global: true}]}, {port: 9001, to: [{global: true}]}]}}", patch)
	require.ErrorContains(t, err, "external port collision")
	name := "renamed"
	require.True(t, Patch{Name: &name}.HasChanges())
	_, err = ParsePatch([]byte(`services: {web: {env: {"": value}}}`))
	require.ErrorContains(t, err, "variable name")
	_, err = Apply("defaults: &x {cpu: 1}\nprofiles: {a: *x, b: *x}\nservices: {web: {image: app}}", Patch{})
	require.NoError(t, err)
	doc := map[string]any{"services": map[string]any{"web": map[string]any{"env": "invalid"}}}
	normalizeSDL(doc)
	require.Equal(t, "invalid", doc["services"].(map[string]any)["web"].(map[string]any)["env"])
	old := "unused: [" + strings.Repeat("x,", 80000) + "]\nservices: {web: {image: old}}"
	desired := "services: {web: {image: old, command: [" + strings.Repeat("'',", 22000) + "]}}"
	_, err = Diff(old, desired)
	require.ErrorContains(t, err, "cannot verify updated SDL")
}

func TestResourceOnlyRootMergeRemainsEditable(t *testing.T) {
	patch, err := ParsePatch([]byte(`services: {web: {image: changed}}`))
	require.NoError(t, err)
	updated, err := Apply("defaults: &x {profiles: {compute: {web: {cpu: 1}}}}\n<<: *x\nservices: {web: {image: old}}", patch)
	require.NoError(t, err)
	require.Contains(t, updated, "image: changed")
	require.Contains(t, updated, "cpu: 1")
	patch, err = ParsePatch([]byte(`services: {web: {expose: {8080: {port: 8081}}}}`))
	require.NoError(t, err)
	_, err = Apply("number: &p 8080\nendpoint: &x {port: *p}\nservices: {web: {expose: [*x]}}", patch)
	require.ErrorContains(t, err, "patched endpoint")
}
