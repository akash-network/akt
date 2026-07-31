package sdl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"pkg.akt.dev/akt/internal/cliutil"
)

// runSDL executes the sdl command group with the given args, capturing
// stdout and stderr. stdin feeds commands that read from "-".
func runSDL(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	cmd := Commands()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)

	err = cmd.Execute()

	return out.String(), errBuf.String(), err
}

func writeFixture(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sdl.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// validSDL is a minimal on-rails SDL priced in uact.
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

func TestScaffoldsListsAll(t *testing.T) {
	stdout, _, err := runSDL(t, "", "scaffolds")
	require.NoError(t, err)

	for _, name := range []string{"web", "gpu", "multi-service", "ip-lease"} {
		require.Contains(t, stdout, name)
	}
	require.Contains(t, stdout, "akt sdl init")
}

func TestInitRoundTripAllScaffolds(t *testing.T) {
	for _, name := range ScaffoldNames() {
		t.Run(name, func(t *testing.T) {
			stdout, _, err := runSDL(t, "", "init", name)
			require.NoError(t, err)

			res := Validate([]byte(stdout))
			require.Truef(t, res.Valid, "generated %s SDL must validate, errors: %+v", name, res.Errors)
			require.Empty(t, res.Errors)
			require.Emptyf(t, res.Warnings, "generated %s SDL must be warning-free", name)

			// Every scaffold prices in uact and follows SDL section order.
			require.Contains(t, stdout, "denom: uact")
			vi := strings.Index(stdout, "version:")
			si := strings.Index(stdout, "services:")
			pi := strings.Index(stdout, "profiles:")
			di := strings.Index(stdout, "deployment:")
			require.True(t, vi >= 0 && vi < si && si < pi && pi < di,
				"expected version < services < profiles < deployment order in:\n%s", stdout)

			// The generated SDL must also pass through the CLI validator.
			valOut, _, valErr := runSDL(t, stdout, "validate", "-")
			require.NoError(t, valErr)
			require.Contains(t, valOut, "0 warning(s)")
		})
	}
}

func TestInitWebShape(t *testing.T) {
	stdout, _, err := runSDL(t, "", "init", "web")
	require.NoError(t, err)

	require.Contains(t, stdout, `version: "2.0"`)
	require.Contains(t, stdout, "image: nginx:1.27")
	require.Contains(t, stdout, "global: true")
	require.Contains(t, stdout, "port: 80")
	require.Contains(t, stdout, "amount: 10000")
	require.Contains(t, stdout, "profile: web")
	require.Contains(t, stdout, "count: 1")
	require.Contains(t, stdout, "units: 0.5")
	require.Contains(t, stdout, "size: 512Mi")
}

func TestInitGPUShape(t *testing.T) {
	stdout, _, err := runSDL(t, "", "init", "gpu")
	require.NoError(t, err)

	require.Contains(t, stdout, "image: pytorch/pytorch:2.2.0-cuda12.1-cudnn8-runtime")
	require.Contains(t, stdout, "gpu:")
	require.Contains(t, stdout, "nvidia:")
	require.Contains(t, stdout, "model: a100")
	require.Contains(t, stdout, "units: 4")       // cpu default
	require.Contains(t, stdout, "size: 16Gi")     // memory default
	require.Contains(t, stdout, "size: 50Gi")     // storage default
	require.Contains(t, stdout, "port: 8080")     // container port default
	require.Contains(t, stdout, "as: 80")         // external port default
	require.Contains(t, stdout, "amount: 100000") // GPU price ceiling
}

func TestInitMultiServiceShape(t *testing.T) {
	stdout, _, err := runSDL(t, "", "init", "multi-service")
	require.NoError(t, err)

	require.Contains(t, stdout, "image: postgres:16")
	require.Contains(t, stdout, "service: db")  // app depends on db
	require.Contains(t, stdout, "service: app") // db exposes 5432 to app only
	require.Contains(t, stdout, "port: 5432")
	require.Contains(t, stdout, "persistent: true")
	require.Contains(t, stdout, "class: beta2")
	require.Contains(t, stdout, "name: db-data")
	require.Contains(t, stdout, "size: 10Gi")
	require.Contains(t, stdout, "mount: /var/lib/postgresql/data")
	require.Contains(t, stdout, "readOnly: false")
	require.Contains(t, stdout, "POSTGRES_PASSWORD=changeme")
}

func TestInitIPLeaseShape(t *testing.T) {
	stdout, _, err := runSDL(t, "", "init", "ip-lease")
	require.NoError(t, err)

	require.Contains(t, stdout, `version: "2.1"`)
	require.Contains(t, stdout, "endpoints:")
	require.Contains(t, stdout, "appip:")
	require.Contains(t, stdout, "kind: ip")
	require.Contains(t, stdout, "ip: appip")
}

func TestInitFlagOverrides(t *testing.T) {
	stdout, _, err := runSDL(t, "", "init", "web",
		"--name", "svc",
		"--image", "myapp:2.1",
		"--port", "3000",
		"--as", "8080",
		"--cpu", "500m",
		"--memory", "1Gi",
		"--storage", "2Gi",
		"--count", "3",
		"--price", "500",
		"--env", "FOO=bar",
		"--env", "BAZ=qux",
	)
	require.NoError(t, err)

	require.Contains(t, stdout, "svc:")
	require.Contains(t, stdout, "image: myapp:2.1")
	require.Contains(t, stdout, "port: 3000")
	require.Contains(t, stdout, "as: 8080")
	require.Contains(t, stdout, "units: 500m")
	require.Contains(t, stdout, "size: 1Gi")
	require.Contains(t, stdout, "size: 2Gi")
	require.Contains(t, stdout, "count: 3")
	require.Contains(t, stdout, "amount: 500")
	require.Contains(t, stdout, "- FOO=bar")
	require.Contains(t, stdout, "- BAZ=qux")

	res := Validate([]byte(stdout))
	require.Truef(t, res.Valid, "overridden SDL must still validate, errors: %+v", res.Errors)
}

func TestInitDeterministicOutput(t *testing.T) {
	first, _, err := runSDL(t, "", "init", "multi-service")
	require.NoError(t, err)

	second, _, err := runSDL(t, "", "init", "multi-service")
	require.NoError(t, err)

	require.Equal(t, first, second, "scaffold output must be byte-for-byte deterministic")
}

func TestInitUnknownScaffold(t *testing.T) {
	_, _, err := runSDL(t, "", "init", "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown scaffold")
	require.Equal(t, cliutil.ExitUsage, cliutil.ExitCode(err))
}

func TestInitEnvRequiresKeyValue(t *testing.T) {
	_, _, err := runSDL(t, "", "init", "web", "--env", "MISSING_EQUALS")
	require.Error(t, err)
	require.Contains(t, err.Error(), "KEY=value")
}

// TestInitOutOfRangeIntFlagsAreUsageErrors pins the zero-vs-unset semantics:
// an unset int flag keeps the per-scaffold default, while an explicitly set
// out-of-range value — including an explicit 0 — is rejected as a usage
// error (exit 2), never misreported as an internal generation error.
func TestInitOutOfRangeIntFlagsAreUsageErrors(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{"count zero", []string{"init", "web", "--count", "0"}, "--count must be at least 1, got 0"},
		{"count negative", []string{"init", "web", "--count", "-3"}, "--count must be at least 1, got -3"},
		{"port zero", []string{"init", "web", "--port", "0"}, "--port must be between 1 and 65535, got 0"},
		{"port too large", []string{"init", "web", "--port", "70000"}, "--port must be between 1 and 65535, got 70000"},
		{"as zero", []string{"init", "web", "--as", "0"}, "--as must be between 1 and 65535, got 0"},
		{"price zero", []string{"init", "web", "--price", "0"}, "--price must be at least 1, got 0"},
		{"gpu zero", []string{"init", "gpu", "--gpu", "0"}, "--gpu must be at least 1, got 0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := runSDL(t, "", tc.args...)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantMsg)
			require.NotContains(t, err.Error(), "internal error")
			require.Equal(t, cliutil.ExitUsage, cliutil.ExitCode(err))
			require.Empty(t, stdout, "no SDL must be emitted for invalid input")
		})
	}
}

// TestInitExplicitMinimumValuesAccepted proves that legitimate explicit
// values at the low end of each range still generate a valid SDL — the
// Changed() discrimination must not reject valid input.
func TestInitExplicitMinimumValuesAccepted(t *testing.T) {
	stdout, _, err := runSDL(t, "", "init", "gpu",
		"--count", "1", "--port", "1", "--as", "65535", "--gpu", "1", "--price", "1")
	require.NoError(t, err)

	res := Validate([]byte(stdout))
	require.Truef(t, res.Valid, "minimum-value SDL must validate, errors: %+v", res.Errors)
	require.Contains(t, stdout, "count: 1")
	require.Contains(t, stdout, "port: 1")
	require.Contains(t, stdout, "as: 65535")
	require.Contains(t, stdout, "amount: 1")
}

func TestInitInvalidExplicitStringFlagsAreUsageErrors(t *testing.T) {
	cases := []struct {
		name       string
		flag       string
		value      string
		wantReason string
	}{
		{name: "name", flag: "--name", value: "Bad_Name", wantReason: `service "Bad_Name" name is invalid`},
		{name: "image", flag: "--image", value: "nginx", wantReason: "has no tag"},
		{name: "cpu", flag: "--cpu", value: "nope", wantReason: `parsing "nope"`},
		{name: "memory", flag: "--memory", value: "nope", wantReason: `parsing "nope"`},
		{name: "storage", flag: "--storage", value: "nope", wantReason: `parsing "nope"`},
		{name: "env", flag: "--env", value: "=value", wantReason: `invalid name ""`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := runSDL(t, "", "init", "web", tc.flag, tc.value)
			require.Error(t, err)
			require.Equal(t, cliutil.ExitUsage, cliutil.ExitCode(err))
			require.Contains(t, err.Error(), tc.flag)
			require.Contains(t, err.Error(), tc.wantReason)
			require.NotContains(t, err.Error(), "internal error")
			require.Empty(t, stdout, "no SDL must be emitted for invalid input")
		})
	}
}

func TestInitMalformedImagesAreUsageErrors(t *testing.T) {
	cases := []struct {
		name  string
		image string
	}{
		{name: "empty tag", image: "nginx:"},
		{name: "empty digest", image: "nginx@sha256:"},
		{name: "short digest", image: "nginx@sha256:abc"},
		{name: "non-hex digest", image: "nginx@sha256:" + strings.Repeat("g", 64)},
		{name: "unsupported digest", image: "nginx@sha512:" + strings.Repeat("a", 128)},
		{name: "double colon", image: "nginx::1"},
		{name: "URL scheme", image: "http://nginx:1"},
		{name: "space", image: "bad image:1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := runSDL(t, "", "init", "web", "--image", tc.image)
			require.Error(t, err)
			require.Equal(t, cliutil.ExitUsage, cliutil.ExitCode(err))
			require.Contains(t, err.Error(), "--image")
			require.Contains(t, err.Error(), "is not a valid container image reference")
			require.NotContains(t, err.Error(), "internal error")
			require.Empty(t, stdout, "no SDL must be emitted for invalid input")
		})
	}
}

func TestInitValidImageReferences(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []string{
		"nginx:1.27",
		"registry.example.com:5000/team/app:1.2.3",
		"nginx@" + digest,
		"registry.example.com/team/app:1.2.3@" + digest,
	}

	for _, image := range cases {
		t.Run(image, func(t *testing.T) {
			stdout, _, err := runSDL(t, "", "init", "web", "--image", image)
			require.NoError(t, err)
			require.Contains(t, stdout, image)
		})
	}
}

func TestGeneratedSDLDefaultValidationFailureIsInternal(t *testing.T) {
	cmd := initCmd()
	out, err := Marshal(webScaffold.Build(Options{Image: "nginx"}))
	require.NoError(t, err)

	err = validateGeneratedSDL(cmd, &webScaffold, out)
	require.Error(t, err)
	require.Equal(t, cliutil.ExitGeneral, cliutil.ExitCode(err))
	require.Contains(t, err.Error(), "internal error")
	require.Contains(t, err.Error(), `scaffold "web" default output failed validation`)
}

func TestGeneratedSDLBrokenDefaultWithOverrideIsInternal(t *testing.T) {
	cmd := initCmd()
	require.NoError(t, cmd.Flags().Set("image", "also-untagged"))

	broken := webScaffold
	broken.Name = "broken"
	broken.Build = func(Options) *yaml.Node {
		return webScaffold.Build(Options{Image: "untagged"})
	}

	out, err := Marshal(broken.Build(Options{Image: "also-untagged"}))
	require.NoError(t, err)

	err = validateGeneratedSDL(cmd, &broken, out)
	require.Error(t, err)
	require.Equal(t, cliutil.ExitGeneral, cliutil.ExitCode(err))
	require.Contains(t, err.Error(), "internal error")
	require.Contains(t, err.Error(), `scaffold "broken" default output failed validation`)
}

func TestValidateValidFile(t *testing.T) {
	path := writeFixture(t, validSDL)

	stdout, _, err := runSDL(t, "", "validate", path)
	require.NoError(t, err)
	require.Contains(t, stdout, "valid: 1 service(s), 1 group(s), 0 warning(s)")
}

func TestValidateUnpinnedImage(t *testing.T) {
	path := writeFixture(t, strings.Replace(validSDL, "image: nginx:1.27", "image: nginx", 1))

	_, stderr, err := runSDL(t, "", "validate", path)
	require.Error(t, err)
	require.Equal(t, cliutil.ExitGeneral, cliutil.ExitCode(err))
	require.Contains(t, stderr, "has no tag")
	require.Contains(t, stderr, "services/web/image")
}

func TestValidateLatestTag(t *testing.T) {
	path := writeFixture(t, strings.Replace(validSDL, "image: nginx:1.27", "image: nginx:latest", 1))

	_, stderr, err := runSDL(t, "", "validate", path)
	require.Error(t, err)
	require.Equal(t, cliutil.ExitGeneral, cliutil.ExitCode(err))
	require.Contains(t, stderr, ":latest")
}

func TestValidateUaktWarnsButPasses(t *testing.T) {
	path := writeFixture(t, strings.Replace(validSDL, "denom: uact", "denom: uakt", 1))

	stdout, _, err := runSDL(t, "", "validate", path)
	require.NoError(t, err, "uakt is valid on-chain and must only warn")
	require.Contains(t, stdout, "valid: 1 service(s), 1 group(s), 1 warning(s)")
	require.Contains(t, stdout, `warning:`)
	require.Contains(t, stdout, "uakt")
}

func TestValidateStdin(t *testing.T) {
	stdout, _, err := runSDL(t, validSDL, "validate", "-")
	require.NoError(t, err)
	require.Contains(t, stdout, "valid: 1 service(s), 1 group(s), 0 warning(s)")
}

func TestValidateGarbageYAML(t *testing.T) {
	path := writeFixture(t, "{{{ this is not yaml: [")

	_, stderr, err := runSDL(t, "", "validate", path)
	require.Error(t, err)
	require.Equal(t, cliutil.ExitGeneral, cliutil.ExitCode(err))
	require.Contains(t, stderr, "invalid: 1 error(s)")
}

func TestValidateMissingFile(t *testing.T) {
	_, _, err := runSDL(t, "", "validate", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	require.Error(t, err)
	require.Equal(t, cliutil.ExitUsage, cliutil.ExitCode(err))
}

func TestDigestPinnedImagePasses(t *testing.T) {
	pinned := strings.Replace(validSDL, "image: nginx:1.27",
		"image: nginx@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", 1)

	stdout, _, err := runSDL(t, pinned, "validate", "-")
	require.NoError(t, err)
	require.Contains(t, stdout, "0 warning(s)")
}
