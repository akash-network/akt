package main

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVerifyCovdataShardRequiresMetadataAndCounters(t *testing.T) {
	t.Parallel()
	validCovdata := buildValidCovdataShard(t)

	for _, tc := range []struct {
		name      string
		files     map[string]string
		wantMeta  int
		wantCount int
		wantError string
	}{
		{
			name:     "complete",
			files:    validCovdata,
			wantMeta: 1, wantCount: 1,
		},
		{
			name:      "unexpected artifact",
			files:     map[string]string{"unrelated-artifact.json": "must not be uploaded"},
			wantError: "unexpected coverage shard entry unrelated-artifact.json",
		},
		{name: "empty directory", wantError: "found 0 metadata and 0 counter"},
		{
			name:      "metadata only",
			files:     map[string]string{"covmeta.0123456789abcdef0123456789abcdef": "metadata"},
			wantError: "found 1 metadata and 0 counter",
		},
		{
			name:      "counter only",
			files:     map[string]string{"covcounters.0123456789abcdef0123456789abcdef.1.2": "counters"},
			wantError: "counter hash 0123456789abcdef0123456789abcdef has no matching covmeta file",
		},
		{
			name:      "empty relevant file",
			files:     map[string]string{"covmeta.0123456789abcdef0123456789abcdef": "", "covcounters.0123456789abcdef0123456789abcdef.1.2": "counters"},
			wantError: "metadata file covmeta.0123456789abcdef0123456789abcdef is empty",
		},
		{
			name: "mismatched hashes",
			files: map[string]string{
				"covmeta.0123456789abcdef0123456789abcdef":             "metadata",
				"covcounters.abcdef0123456789abcdef0123456789.123.456": "counters",
			},
			wantError: "counter hash abcdef0123456789abcdef0123456789 has no matching covmeta file",
		},
		{name: "corrupt matching payloads", files: map[string]string{
			"covmeta.0123456789abcdef0123456789abcdef":         "metadata",
			"covcounters.0123456789abcdef0123456789abcdef.1.2": "counters",
		}, wantError: "go toolchain rejected coverage data"},
		{
			name: "malformed counter suffix",
			files: map[string]string{
				"covmeta.0123456789abcdef0123456789abcdef":           "metadata",
				"covcounters.0123456789abcdef0123456789abcdef.bad.2": "counters",
			},
			wantError: "invalid Go covdata name",
		},
		{
			name: "malformed metadata hash",
			files: map[string]string{
				"covmeta.NOT-A-HASH": "metadata",
			},
			wantError: "invalid Go covdata name",
		},
		{
			name: "empty counter",
			files: map[string]string{
				"covmeta.0123456789abcdef0123456789abcdef":         "metadata",
				"covcounters.0123456789abcdef0123456789abcdef.1.2": "",
			},
			wantError: "counter file covcounters.0123456789abcdef0123456789abcdef.1.2 is empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			for name, contents := range tc.files {
				if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			metadata, counters, err := verifyCovdataShard(directory, false)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("verifyCovdataShard() error = %v, want containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if metadata != tc.wantMeta || counters != tc.wantCount {
				t.Fatalf("verifyCovdataShard() = %d/%d, want %d/%d", metadata, counters, tc.wantMeta, tc.wantCount)
			}
		})
	}
	if _, _, err := verifyCovdataShard(filepath.Join(t.TempDir(), "missing"), false); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("missing shard error = %v", err)
	}
}

func TestVerifyCovdataShardRejectsSymlinkedArtifacts(t *testing.T) {
	t.Parallel()

	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("artifact\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name          string
		artifact      string
		allowIdentity bool
		want          string
	}{
		{name: "source manifest", artifact: "source-manifest.tsv", want: "source manifest is not a regular file"},
		{name: "binary identity", artifact: binaryIdentityFilename, allowIdentity: true, want: "binary identity is not a regular file"},
		{name: "metadata", artifact: "covmeta.0123456789abcdef0123456789abcdef", want: "metadata file"},
		{name: "counter", artifact: "covcounters.0123456789abcdef0123456789abcdef.1.2", want: "counter file"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(directory, test.artifact)); err != nil {
				t.Fatal(err)
			}
			_, _, err := verifyCovdataShard(directory, test.allowIdentity)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "not a regular file") {
				t.Fatalf("verifyCovdataShard() error = %v, want %q regular-file rejection", err, test.want)
			}
		})
	}
}

func TestVerifyCovdataShardRejectsOrphanMetadata(t *testing.T) {
	complete := buildValidCovdataShard(t)
	orphan := buildIndependentCovdataShard(t)
	directory := t.TempDir()
	for name, contents := range complete {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	orphanName := ""
	for name, contents := range orphan {
		if !strings.HasPrefix(name, "covmeta.") {
			continue
		}
		if _, duplicate := complete[name]; duplicate {
			t.Fatalf("independent coverage fixture reused metadata hash %s", name)
		}
		orphanName = name
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		break
	}
	if orphanName == "" {
		t.Fatal("independent coverage fixture produced no metadata")
	}

	metadata, counters, err := verifyCovdataShard(directory, false)
	if err == nil || !strings.Contains(err.Error(), "has no matching covcounters file") {
		t.Fatalf("verifyCovdataShard() = %d/%d, error %v; want orphan metadata rejection", metadata, counters, err)
	}
}

func TestValidateWorkflowActionPins(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"

	for _, test := range []struct {
		name      string
		workflow  string
		wantError string
	}{
		{
			name: "immutable and local actions",
			workflow: "name: ci\non: push\njobs:\n  test:\n    uses: owner/repository/.github/workflows/test.yml@" + commit +
				"\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@" + commit +
				"\n      - uses: ./.github/actions/setup\n",
		},
		{
			name:      "floating tag",
			workflow:  "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v7\n",
			wantError: "40-character commit SHA",
		},
		{
			name:      "abbreviated hash",
			workflow:  "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@0123456789ab\n",
			wantError: "40-character commit SHA",
		},
		{
			name:      "non-hex commit",
			workflow:  "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@0123456789abcdef0123456789abcdef0123456z\n",
			wantError: "commit must be hexadecimal",
		},
		{
			name:      "missing commit separator",
			workflow:  "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout\n",
			wantError: "external action must end in @",
		},
		{
			name:      "malformed workflow",
			workflow:  "jobs: [\n",
			wantError: "parse workflow",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, ".github", "workflows")
			if err := os.MkdirAll(directory, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(directory, "nested"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "README.md"), []byte("not a workflow\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "ci.yml"), []byte(test.workflow), 0o600); err != nil {
				t.Fatal(err)
			}

			err := validateWorkflowActionPins(root)
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateWorkflowActionPins() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
	if err := validateWorkflowNodeActionPins(nil, "nil.yml"); err != nil {
		t.Fatalf("nil workflow node: %v", err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".github"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".github", "workflows"), []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateWorkflowActionPins(root); err == nil || !strings.Contains(err.Error(), "read workflow directory") {
		t.Fatalf("workflow directory boundary error = %v", err)
	}
}

func buildValidCovdataShard(t *testing.T) map[string]string {
	t.Helper()
	root := t.TempDir()
	output := filepath.Join(root, "coverage-tool")
	command := exec.Command("go", "build", "-cover", "-o", output, ".")
	command.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build coverage fixture: %v\n%s", err, out)
	}
	raw := filepath.Join(root, "raw")
	if err := os.Mkdir(raw, 0o700); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(output)
	command.Env = replaceEnv(replaceEnv(os.Environ(), "GOWORK", "off"), "GOCOVERDIR", raw)
	_ = command.Run()
	files := map[string]string{"source-manifest.tsv": "manifest"}
	entries, err := os.ReadDir(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		contents, err := os.ReadFile(filepath.Join(raw, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = string(contents)
	}
	return files
}

func buildIndependentCovdataShard(t *testing.T) map[string]string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/coverageorphan\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { println(\"independent fixture\") }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "fixture")
	command := exec.Command("go", "build", "-cover", "-o", output, ".")
	command.Dir = root
	command.Env = replaceEnv(os.Environ(), "GOWORK", "off")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build independent coverage fixture: %v\n%s", err, out)
	}
	raw := filepath.Join(root, "raw")
	if err := os.Mkdir(raw, 0o700); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(output)
	command.Env = replaceEnv(replaceEnv(os.Environ(), "GOWORK", "off"), "GOCOVERDIR", raw)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run independent coverage fixture: %v\n%s", err, out)
	}
	files := make(map[string]string)
	entries, err := os.ReadDir(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		contents, err := os.ReadFile(filepath.Join(raw, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = string(contents)
	}
	return files
}

func TestBuildSourceManifestTracksCoverageInputsOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, directory := range []string{"internal/example/testdata", "make", "coverage", ".cache/generated", ".github/workflows"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	inputs := map[string]string{
		".env":                                "CGO_ENABLED=1\n",
		".envrc":                              "dotenv\n",
		".goreleaser.yaml":                    "builds: []\n",
		".goreleaser.yml":                     "builds: []\n",
		".github/workflows/ci.yml":            "name: ci\n",
		".github/workflows/release.yaml":      "name: release-yaml\n",
		".github/workflows/release.yml":       "name: release\n",
		"internal/example/example.go":         "package example\n\nimport _ \"embed\"\n\n//go:embed workflow.yaml\nvar workflow string\n",
		"internal/example/example_test.go":    "package example\n",
		"internal/example/workflow.yaml":      "name: embedded-workflow\n",
		"internal/example/testdata/case.json": `{"expected":"fixture"}`,
		"go.mod":                              "module example.test/repo\n",
		"go.work":                             "go 1.25\n",
		"make/testing.mk":                     "test:\n\tgo test ./...\n",
		"coverage/packages.tsv":               "package\tclass\tcritical\n",
		"coverage/baseline-active.tsv":        "package\tcovered\ttotal\n",
		"codecov.yml":                         "coverage:\n  precision: 2\n",
		"README.md":                           "documentation does not alter collection\n",
		".cache/generated/stale.go":           "package stale\n",
	}
	for filename, contents := range inputs {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(filename)), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	first, err := buildSourceManifest(root, "ledger,netgo", "static-link")
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(first)
	for _, expected := range []string{
		"@build-options\t", "@build-tags\t", "@go-version\t", "@platform\t", "@go-env-CGO_ENABLED\t",
		"@go-env-AR\t", "@go-env-GOAMD64\t", "@go-env-GOEXPERIMENT\t", "@go-env-GOFIPS140\t", "@go-env-GOFLAGS\t",
		"@go-env-GOTOOLCHAIN\t", "@go-env-GOWORK\t", ".env\t", ".envrc\t",
		".goreleaser.yaml\t", ".goreleaser.yml\t", ".github/workflows/ci.yml\t",
		".github/workflows/release.yaml\t", ".github/workflows/release.yml\t", "internal/example/example.go\t",
		"internal/example/example_test.go\t", "internal/example/workflow.yaml\t",
		"internal/example/testdata/case.json\t", "go.mod\t", "make/testing.mk\t",
		"coverage/packages.tsv\t",
	} {
		if !strings.Contains(manifest, expected) {
			t.Errorf("source manifest missing %q:\n%s", expected, manifest)
		}
	}
	for _, excluded := range []string{
		"README.md\t", ".cache/generated/stale.go\t", "go.work\t",
		"coverage/baseline-active.tsv\t", "codecov.yml\t",
	} {
		if strings.Contains(manifest, excluded) {
			t.Errorf("source manifest unexpectedly contains %q", excluded)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("changed documentation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterDocs, err := buildSourceManifest(root, "ledger,netgo", "static-link")
	if err != nil {
		t.Fatal(err)
	}
	if string(afterDocs) != string(first) {
		t.Fatal("documentation-only edit changed source manifest")
	}

	if err := os.WriteFile(filepath.Join(root, "coverage/baseline-active.tsv"), []byte("package\tcovered\ttotal\n@total\t1\t1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterBaseline, err := buildSourceManifest(root, "ledger,netgo", "static-link")
	if err != nil {
		t.Fatal(err)
	}
	if string(afterBaseline) != string(first) {
		t.Fatal("reporting-only baseline edit changed source manifest")
	}

	if err := os.WriteFile(filepath.Join(root, "internal/example/workflow.yaml"), []byte("name: changed-embedded-workflow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterEmbedded, err := buildSourceManifest(root, "ledger,netgo", "static-link")
	if err != nil {
		t.Fatal(err)
	}
	if string(afterEmbedded) == string(first) {
		t.Fatal("embedded workflow edit did not change source manifest")
	}

	if err := os.WriteFile(filepath.Join(root, "internal/example/testdata/case.json"), []byte(`{"expected":"changed-fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	afterFixture, err := buildSourceManifest(root, "ledger,netgo", "static-link")
	if err != nil {
		t.Fatal(err)
	}
	if string(afterFixture) == string(afterEmbedded) {
		t.Fatal("testdata fixture edit did not change source manifest")
	}

	if err := os.WriteFile(filepath.Join(root, "internal/example/example.go"), []byte("package example\n\nconst changed = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterSource, err := buildSourceManifest(root, "ledger,netgo", "static-link")
	if err != nil {
		t.Fatal(err)
	}
	if string(afterSource) == string(first) {
		t.Fatal("Go source edit did not change source manifest")
	}

	afterTags, err := buildSourceManifest(root, "cgotrace,ledger,netgo", "cgotrace static-link")
	if err != nil {
		t.Fatal(err)
	}
	if string(afterTags) == string(afterSource) {
		t.Fatal("evaluated build tags and options did not change source manifest")
	}

	reorderedTags, err := buildSourceManifest(root, "netgo,ledger,cgotrace", "cgotrace static-link")
	if err != nil {
		t.Fatal(err)
	}
	if string(reorderedTags) != string(afterTags) {
		t.Fatal("semantically equivalent reordered build tags changed source manifest")
	}
}

func TestBuildSourceManifestEscapesUnusualSourcePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/unusualpaths\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "testdata"), 0o700); err != nil {
		t.Fatal(err)
	}
	unusual := "testdata/case\twith-tab.txt"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(unusual)), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	manifest, err := buildSourceManifest(root, "netgo", "static")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "manifest.tsv")
	if err := os.WriteFile(filename, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSourceManifest(filename); err != nil {
		t.Fatalf("generated manifest is not valid TSV: %v\n%s", err, manifest)
	}
	if !strings.Contains(string(manifest), "\""+unusual+"\"") {
		t.Fatalf("manifest did not quote unusual path %q:\n%s", unusual, manifest)
	}
}

func TestSourceManifestFailsClosedAtProcessAndFilesystemBoundaries(t *testing.T) {
	t.Run("missing root", func(t *testing.T) {
		_, err := buildSourceManifest(filepath.Join(t.TempDir(), "missing"), "netgo", "")
		if err == nil || !strings.Contains(err.Error(), "effective Go build environment") {
			t.Fatalf("buildSourceManifest() error = %v, want missing-root rejection", err)
		}
	})

	t.Run("active workspace", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/workspace\n\ngo 1.23\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		workspace := filepath.Join(root, "go.work")
		if err := os.WriteFile(workspace, []byte("go 1.23\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GOWORK", workspace)
		_, err := buildSourceManifest(root, "netgo", "")
		if err == nil || !strings.Contains(err.Error(), "requires GOWORK=off") {
			t.Fatalf("buildSourceManifest() error = %v, want active-workspace rejection", err)
		}
	})

	t.Run("symlinked source input", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/symlink\n\ngo 1.23\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "testdata"), 0o700); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), "fixture.txt")
		if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "testdata", "linked.txt")); err != nil {
			t.Fatal(err)
		}
		_, err := buildSourceManifest(root, "netgo", "")
		if err == nil || !strings.Contains(err.Error(), "is not a regular file") {
			t.Fatalf("buildSourceManifest() error = %v, want symlink rejection", err)
		}
	})

	t.Run("embedded discovery input validation", func(t *testing.T) {
		root := t.TempDir()
		if _, err := discoverEmbeddedInputs(filepath.Join(root, "missing"), ""); err == nil || !strings.Contains(err.Error(), "root symlinks") {
			t.Fatalf("missing embedded root error = %v", err)
		}
		if _, err := discoverEmbeddedInputs(root, "-bad"); err == nil || !strings.Contains(err.Error(), "invalid build tag") {
			t.Fatalf("invalid embedded build tags error = %v", err)
		}
	})
}

func TestGoCommandConsumersRejectMissingOrMalformedOutput(t *testing.T) {
	installFakeGo := func(t *testing.T, script string) {
		t.Helper()
		root := t.TempDir()
		filename := filepath.Join(root, "go")
		if err := os.WriteFile(filename, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", root)
	}

	t.Run("command failure", func(t *testing.T) {
		installFakeGo(t, "printf 'unavailable\\n' >&2\nexit 1\n")
		if _, err := moduleName(); err == nil || !strings.Contains(err.Error(), "go list -m") {
			t.Fatalf("moduleName() error = %v", err)
		}
		if _, err := moduleRoot(); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("moduleRoot() error = %v", err)
		}
		if _, err := effectiveGoBuildEnvironment("."); err == nil || !strings.Contains(err.Error(), "inspect effective Go build environment") {
			t.Fatalf("effectiveGoBuildEnvironment() error = %v", err)
		}
		if _, err := discoverEmbeddedInputs(".", ""); err == nil || !strings.Contains(err.Error(), "discover go:embed inputs") {
			t.Fatalf("discoverEmbeddedInputs() error = %v", err)
		}
		if _, err := listPackageDependencies("netgo", "./cmd/akt"); err == nil || !strings.Contains(err.Error(), "go list dependencies") {
			t.Fatalf("listPackageDependencies() error = %v", err)
		}
		if _, err := listRepositoryImports("netgo"); err == nil || !strings.Contains(err.Error(), "go list repository imports") {
			t.Fatalf("listRepositoryImports() error = %v", err)
		}
		if _, err := listReleaseDependencyLocations("netgo"); err == nil || !strings.Contains(err.Error(), "go list release dependency locations") {
			t.Fatalf("listReleaseDependencyLocations() error = %v", err)
		}
	})

	t.Run("empty module paths", func(t *testing.T) {
		installFakeGo(t, "exit 0\n")
		if _, err := moduleName(); err == nil || !strings.Contains(err.Error(), "empty module name") {
			t.Fatalf("moduleName() error = %v", err)
		}
		if _, err := moduleRoot(); err == nil || !strings.Contains(err.Error(), "empty module root") {
			t.Fatalf("moduleRoot() error = %v", err)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		installFakeGo(t, "printf 'not-json\\n'\n")
		if _, err := effectiveGoBuildEnvironment("."); err == nil || !strings.Contains(err.Error(), "decode effective Go build environment") {
			t.Fatalf("effectiveGoBuildEnvironment() error = %v", err)
		}
		if _, err := discoverEmbeddedInputs(".", ""); err == nil || !strings.Contains(err.Error(), "decode go:embed package inventory") {
			t.Fatalf("discoverEmbeddedInputs() error = %v", err)
		}
		if _, err := listRepositoryImports("netgo"); err == nil || !strings.Contains(err.Error(), "malformed go list import row") {
			t.Fatalf("listRepositoryImports() error = %v", err)
		}
		if _, err := listReleaseDependencyLocations("netgo"); err == nil || !strings.Contains(err.Error(), "decode release dependency locations") {
			t.Fatalf("listReleaseDependencyLocations() error = %v", err)
		}
	})

	t.Run("incomplete environment", func(t *testing.T) {
		installFakeGo(t, "printf '{}\\n'\n")
		if _, err := effectiveGoBuildEnvironment("."); err == nil || !strings.Contains(err.Error(), "omitted AR") {
			t.Fatalf("effectiveGoBuildEnvironment() error = %v", err)
		}
	})

	t.Run("embedded input escapes root", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.ToSlash(t.TempDir())
		installFakeGo(t, "printf '%s\\n' '{\"Dir\":\""+outside+"\",\"EmbedFiles\":[\"outside.txt\"]}'\n")
		if _, err := discoverEmbeddedInputs(root, ""); err == nil || !strings.Contains(err.Error(), "escapes repository root") {
			t.Fatalf("discoverEmbeddedInputs() error = %v", err)
		}
	})
}

func TestGoBuildSourceInputIncludesNativeCompilerInputs(t *testing.T) {
	t.Parallel()

	for _, extension := range []string{
		".go", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx",
		".m", ".mm", ".f", ".F", ".f90", ".for", ".s", ".S", ".sx", ".swig", ".swigcxx", ".syso",
	} {
		if !goBuildSourceInput("internal/native/input" + extension) {
			t.Errorf("native build input %s was excluded", extension)
		}
	}
	for _, filename := range []string{"README.md", "script.sh", "image.png"} {
		if goBuildSourceInput(filename) {
			t.Errorf("non-Go build input %s was included", filename)
		}
	}
}

func TestVerifySourceManifestRejectsMissingMalformedAndStaleData(t *testing.T) {
	t.Parallel()

	const valid = "path\tsha256\n@go-version\t0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n@platform\tabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789\n"
	expected := writeTempFile(t, "expected.tsv", valid)
	matching := writeTempFile(t, "matching.tsv", valid)
	if err := verifySourceManifest(matching, expected); err != nil {
		t.Fatalf("matching manifests: %v", err)
	}

	stale := writeTempFile(t, "stale.tsv", strings.Replace(valid, "abcdef012345", "bbcdef012345", 1))
	if err := verifySourceManifest(stale, expected); err == nil || !strings.Contains(err.Error(), "recollect this shard") {
		t.Fatalf("stale manifest error = %v, want recollection failure", err)
	}

	malformed := writeTempFile(t, "malformed.tsv", "path\tsha256\nsource.go\tnot-a-digest\n")
	if err := verifySourceManifest(malformed, expected); err == nil || !strings.Contains(err.Error(), "invalid header or no source entries") {
		t.Fatalf("malformed manifest error = %v, want structural rejection", err)
	}

	missing := filepath.Join(t.TempDir(), "missing.tsv")
	if err := verifySourceManifest(missing, expected); err == nil || !strings.Contains(err.Error(), "load collected source manifest") {
		t.Fatalf("missing manifest error = %v, want missing collected manifest", err)
	}

	invalidRow := writeTempFile(t, "invalid-row.tsv", "path\tsha256\n@meta\t"+strings.Repeat("0", 64)+"\nsource.go\tnot-a-digest\n")
	if _, err := loadSourceManifest(invalidRow); err == nil || !strings.Contains(err.Error(), "row 3 is invalid") {
		t.Fatalf("invalid manifest row error = %v", err)
	}
	unsorted := writeTempFile(t, "unsorted.tsv", "path\tsha256\nz.go\t"+strings.Repeat("0", 64)+"\na.go\t"+strings.Repeat("1", 64)+"\n")
	if _, err := loadSourceManifest(unsorted); err == nil || !strings.Contains(err.Error(), "not strictly sorted") {
		t.Fatalf("unsorted manifest error = %v", err)
	}
	if err := verifySourceManifest(matching, filepath.Join(t.TempDir(), "missing.tsv")); err == nil || !strings.Contains(err.Error(), "load current source manifest") {
		t.Fatalf("missing expected manifest error = %v", err)
	}
}

func TestLoadProfileRejectsDuplicateStatementRanges(t *testing.T) {
	t.Parallel()

	filename := writeTempFile(t, "profile.out", `mode: atomic
pkg.akt.dev/akt/internal/example/example.go:1.1,2.2 1 1
pkg.akt.dev/akt/internal/example/example.go:1.1,2.2 1 0
`)
	_, err := loadProfile(filename)
	if err == nil || !strings.Contains(err.Error(), "duplicate statement range") {
		t.Fatalf("loadProfile() error = %v, want duplicate range error", err)
	}
}

func TestCountStatementsRejectsOverflow(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/example"
	packages := packageSet{
		Module:  "pkg.akt.dev/akt",
		Entries: []packageEntry{{Name: packageName, Class: classActive}},
		ByName: map[string]packageEntry{
			packageName: {Name: packageName, Class: classActive},
		},
	}
	profile := coverProfile{Mode: "atomic", Blocks: []coverBlock{
		{File: packageName + "/example.go", Statements: math.MaxInt64, Count: 1},
		{File: packageName + "/example.go", Statements: 1, Count: 1},
	}}

	_, err := countStatements(packages, profile, classActive)
	if err == nil || !strings.Contains(err.Error(), "statement count overflow") {
		t.Fatalf("countStatements() error = %v, want statement count overflow", err)
	}

	const otherPackage = "pkg.akt.dev/akt/internal/other"
	packages.Entries = append(packages.Entries, packageEntry{Name: otherPackage, Class: classActive})
	packages.ByName[otherPackage] = packageEntry{Name: otherPackage, Class: classActive}
	profile.Blocks[1].File = otherPackage + "/other.go"
	if _, err := countStatements(packages, profile, classActive); err == nil || !strings.Contains(err.Error(), "statement count overflow") {
		t.Fatalf("cross-package countStatements() error = %v, want aggregate overflow", err)
	}

	profile.Blocks = []coverBlock{{File: packageName + "/example.go", Statements: -1, Count: 1}}
	if _, err := countStatements(packages, profile, classActive); err == nil || !strings.Contains(err.Error(), "negative statement count") {
		t.Fatalf("negative countStatements() error = %v, want negative-count rejection", err)
	}
}

func TestCountStatementsKeepsDenominatorsSeparate(t *testing.T) {
	t.Parallel()

	packages := packageSet{
		Module: "pkg.akt.dev/akt",
		Entries: []packageEntry{
			{Name: "pkg.akt.dev/akt/internal/active", Class: classActive},
			{Name: "pkg.akt.dev/akt/internal/disabled", Class: classExperimentalTUI},
			{Name: "pkg.akt.dev/akt/internal/helper", Class: classSupport},
			{Name: "pkg.akt.dev/akt/tools/coverage", Class: classTooling},
		},
		ByName: map[string]packageEntry{
			"pkg.akt.dev/akt/internal/active":   {Name: "pkg.akt.dev/akt/internal/active", Class: classActive},
			"pkg.akt.dev/akt/internal/disabled": {Name: "pkg.akt.dev/akt/internal/disabled", Class: classExperimentalTUI},
			"pkg.akt.dev/akt/internal/helper":   {Name: "pkg.akt.dev/akt/internal/helper", Class: classSupport},
			"pkg.akt.dev/akt/tools/coverage":    {Name: "pkg.akt.dev/akt/tools/coverage", Class: classTooling},
		},
	}
	profile := coverProfile{Mode: "atomic", Blocks: []coverBlock{
		{File: "pkg.akt.dev/akt/internal/active/a.go", Statements: 3, Count: 1},
		{File: "pkg.akt.dev/akt/internal/active/a.go", Statements: 2, Count: 0},
		{File: "pkg.akt.dev/akt/internal/disabled/tui.go", Statements: 7, Count: 1},
		{File: "pkg.akt.dev/akt/internal/helper/helper.go", Statements: 11, Count: 0},
		{File: "pkg.akt.dev/akt/tools/coverage/main.go", Statements: 13, Count: 1},
	}}

	active, err := countStatements(packages, profile, classActive)
	if err != nil {
		t.Fatal(err)
	}
	if got := active["@total"]; got != (statementCount{Covered: 3, Total: 5}) {
		t.Fatalf("active total = %+v, want 3/5", got)
	}
	repository, err := countStatements(packages, profile, "repository")
	if err != nil {
		t.Fatal(err)
	}
	if got := repository["@total"]; got != (statementCount{Covered: 23, Total: 25}) {
		t.Fatalf("repository total = %+v, want 23/25; tooling is included and support is excluded", got)
	}
	tooling, err := countStatements(packages, profile, classTooling)
	if err != nil {
		t.Fatal(err)
	}
	if got := tooling["@total"]; got != (statementCount{Covered: 13, Total: 13}) {
		t.Fatalf("tooling total = %+v, want 13/13", got)
	}
}

func TestDiscoverSourcePackagesFindsBuildConstrainedPackages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSource := func(name, contents string) {
		t.Helper()
		filename := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeSource("internal/hidden/only.go", "//go:build a_tag_that_is_never_selected\n\npackage hidden\n")
	writeSource("internal/tests/only_test.go", "package tests\n")
	writeSource("testdata/fixture/fixture.go", "package fixture\n")
	writeSource(".cache/generated/generated.go", "package generated\n")
	writeSource("_hidden/generated.go", "package hidden\n")
	writeSource("vendor/example.test/dependency/dependency.go", "package dependency\n")
	writeSource("nested/go.mod", "module example.test/nested\n")
	writeSource("nested/package/package.go", "package nested\n")

	actual, err := discoverSourcePackages(root, "example.test/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !actual["example.test/repo/internal/hidden"] {
		t.Fatalf("discovered packages = %#v, want build-constrained package", actual)
	}
	for _, excluded := range []string{
		"example.test/repo/internal/tests",
		"example.test/repo/testdata/fixture",
		"example.test/repo/.cache/generated",
		"example.test/repo/_hidden",
		"example.test/repo/vendor/example.test/dependency",
		"example.test/repo/nested/package",
	} {
		if actual[excluded] {
			t.Fatalf("discovered packages includes excluded directory %q: %#v", excluded, actual)
		}
	}
}

func TestSourceDiscoveryFailsClosedOnFilesystemErrors(t *testing.T) {
	t.Parallel()

	if _, err := discoverSourcePackages(filepath.Join(t.TempDir(), "missing"), "example.test/repo"); err == nil || !strings.Contains(err.Error(), "discover non-test Go source") {
		t.Fatalf("missing discoverSourcePackages() error = %v", err)
	}
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("go.mod", filepath.Join(nested, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverSourcePackages(root, "example.test/repo"); err == nil || !strings.Contains(err.Error(), "inspect nested module") {
		t.Fatalf("looping nested module error = %v", err)
	}
}

func TestValidateTaxonomyRejectsSupportReleaseDependency(t *testing.T) {
	t.Parallel()

	const helper = "pkg.akt.dev/akt/internal/helper"
	packages := packageSet{
		Module:  "pkg.akt.dev/akt",
		Entries: []packageEntry{{Name: helper, Class: classSupport}},
		ByName: map[string]packageEntry{
			helper: {Name: helper, Class: classSupport},
		},
	}
	exceptions := map[string]exception{
		exceptionKey(helper, "internal/helper", 0): {Package: helper},
	}

	err := validateTaxonomy(packages, exceptions, map[string]bool{helper: true}, map[string]bool{helper: true})
	if err == nil || !strings.Contains(err.Error(), "support package is linked into the release binary") {
		t.Fatalf("validateTaxonomy() error = %v, want release-dependency rejection", err)
	}
}

func TestValidateTaxonomyRejectsInventoryDriftAndUnreviewedSupport(t *testing.T) {
	t.Parallel()

	const (
		module  = "pkg.akt.dev/akt"
		stale   = module + "/internal/stale"
		support = module + "/internal/support"
		newPkg  = module + "/internal/new"
	)
	packages := packageSet{
		Module: module,
		Entries: []packageEntry{
			{Name: stale, Class: classActive},
			{Name: support, Class: classSupport},
		},
		ByName: map[string]packageEntry{
			stale:   {Name: stale, Class: classActive},
			support: {Name: support, Class: classSupport},
		},
	}
	err := validateTaxonomy(packages, nil, map[string]bool{support: true, newPkg: true}, map[string]bool{stale: true})
	if err == nil {
		t.Fatal("validateTaxonomy() accepted inventory drift")
	}
	for _, want := range []string{
		"unclassified package: " + newPkg,
		"stale package entry: " + stale,
		"support package lacks a reviewed package exception: " + support,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validateTaxonomy() error = %q, want %q", err, want)
		}
	}
}

func TestValidateTaxonomyRejectsSpoofedSpecialClasses(t *testing.T) {
	t.Parallel()

	const (
		module        = "pkg.akt.dev/akt"
		fakeTooling   = module + "/internal/hidden"
		fakeTUI       = module + "/internal/also-hidden"
		invalidActive = module + "/tools/release-helper"
	)
	packages := packageSet{
		Module: module,
		Entries: []packageEntry{
			{Name: fakeTUI, Class: classExperimentalTUI},
			{Name: fakeTooling, Class: classTooling},
			{Name: invalidActive, Class: classActive},
		},
		ByName: map[string]packageEntry{
			fakeTUI:       {Name: fakeTUI, Class: classExperimentalTUI},
			fakeTooling:   {Name: fakeTooling, Class: classTooling},
			invalidActive: {Name: invalidActive, Class: classActive},
		},
	}
	actual := map[string]bool{fakeTUI: true, fakeTooling: true, invalidActive: true}

	err := validateTaxonomy(packages, nil, actual, map[string]bool{fakeTUI: true, fakeTooling: true})
	if err == nil {
		t.Fatal("validateTaxonomy() succeeded, want special-class path failures")
	}
	for _, want := range []string{
		"experimental-tui package is outside internal/tui",
		"tooling package is outside tools",
		"tooling package is linked into the release binary",
		"package under tools must use tooling class",
		"active package is not linked into the release binary",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validateTaxonomy() error = %q, want containing %q", err, want)
		}
	}
}

func TestValidateActiveDependencyClosureRejectsNonActiveDependency(t *testing.T) {
	t.Parallel()

	const (
		module       = "pkg.akt.dev/akt"
		monitor      = module + "/internal/monitor/runtime"
		experimental = module + "/internal/tui/views"
	)
	packages := packageSet{
		Module: module,
		Entries: []packageEntry{
			{Name: monitor, Class: classActive},
			{Name: experimental, Class: classExperimentalTUI},
		},
		ByName: map[string]packageEntry{
			monitor:      {Name: monitor, Class: classActive},
			experimental: {Name: experimental, Class: classExperimentalTUI},
		},
	}
	dependencies := map[string]bool{
		monitor:                  true,
		experimental:             true,
		"github.com/example/lib": true,
	}

	err := validateActiveDependencyClosure(packages, monitor, dependencies)
	if err == nil || !strings.Contains(err.Error(), "experimental-tui") {
		t.Fatalf("validateActiveDependencyClosure() error = %v, want experimental dependency rejection", err)
	}

	delete(dependencies, experimental)
	if err := validateActiveDependencyClosure(packages, monitor, dependencies); err != nil {
		t.Fatalf("active-only dependency closure: %v", err)
	}
	if err := validateActiveDependencyClosure(packages, module+"/internal/missing", dependencies); err == nil || !strings.Contains(err.Error(), "not classified active") {
		t.Fatalf("missing active root error = %v", err)
	}
}

func TestValidateExperimentalImportBoundaryAllowsOnlyGatedRootBridge(t *testing.T) {
	t.Parallel()

	const module = "pkg.akt.dev/akt"
	packages := packageSet{
		Module: module,
		ByName: map[string]packageEntry{
			module + "/internal/cli":       {Name: module + "/internal/cli", Class: classActive},
			module + "/internal/provider":  {Name: module + "/internal/provider", Class: classActive},
			module + "/internal/tui":       {Name: module + "/internal/tui", Class: classExperimentalTUI},
			module + "/internal/tui/views": {Name: module + "/internal/tui/views", Class: classExperimentalTUI},
		},
	}
	allowed := map[string][]string{
		module + "/internal/cli": {module + "/internal/tui"},
	}
	if err := validateExperimentalImportBoundary(packages, allowed); err != nil {
		t.Fatalf("reviewed root bridge was rejected: %v", err)
	}
	allowed[module+"/internal/provider"] = []string{module + "/internal/tui/views"}
	if err := validateExperimentalImportBoundary(packages, allowed); err == nil ||
		!strings.Contains(err.Error(), "active package "+module+"/internal/provider imports experimental package "+module+"/internal/tui/views") {
		t.Fatalf("unreviewed active-to-experimental import error = %v", err)
	}
}

func TestValidateLocalReleaseDependenciesRejectsNestedModuleEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mainDir := filepath.Join(root, "internal", "main")
	nestedDir := filepath.Join(root, "nested")
	for _, directory := range []string{mainDir, nestedDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	packages := packageSet{ByName: map[string]packageEntry{
		"example.test/repo/internal/main": {Name: "example.test/repo/internal/main", Class: classActive},
	}}
	err := validateLocalReleaseDependencies(root, packages, []packageLocation{
		{ImportPath: "builtin"},
		{ImportPath: "example.test/repo/internal/main", Dir: mainDir},
		{ImportPath: "example.test/nested", Dir: nestedDir},
		{ImportPath: "example.test/external", Dir: filepath.Dir(root)},
	})
	if err == nil || !strings.Contains(err.Error(), "repository-local release dependency is unclassified: example.test/nested (nested)") {
		t.Fatalf("validateLocalReleaseDependencies() error = %v", err)
	}
	if err := validateLocalReleaseDependencies(filepath.Join(root, "missing"), packages, nil); err == nil || !strings.Contains(err.Error(), "repository root symlinks") {
		t.Fatalf("missing repository root error = %v", err)
	}
	if err := validateLocalReleaseDependencies(root, packages, []packageLocation{{ImportPath: "example.test/missing", Dir: filepath.Join(root, "missing")}}); err == nil || !strings.Contains(err.Error(), "resolve release dependency") {
		t.Fatalf("missing dependency directory error = %v", err)
	}
}

func TestValidateReleaseTagsRequiresEveryGoreleaserBuildToMatch(t *testing.T) {
	t.Parallel()

	matching := writeTempFile(t, ".goreleaser.yaml", `builds:
  - main: ./cmd/akt
    flags:
      - -tags=netgo,ledger
  - main: ./cmd/akt
    flags:
      - -tags=netgo,ledger
`)
	if err := validateGoreleaserReleaseTags(matching, "ledger,netgo"); err != nil {
		t.Fatalf("matching release tags: %v", err)
	}
	splitFlag := writeTempFile(t, ".goreleaser.yaml", `builds:
  - main: ./cmd/akt
    flags:
      - -tags
      - netgo,ledger
`)
	if err := validateGoreleaserReleaseTags(splitFlag, "ledger,netgo"); err != nil {
		t.Fatalf("split release tag flag: %v", err)
	}

	drifted := writeTempFile(t, ".goreleaser.yaml", `builds:
  - main: ./cmd/akt
    flags:
      - -tags=netgo,ledger
  - main: ./cmd/akt
    flags:
      - -tags=netgo,cgo
`)
	err := validateGoreleaserReleaseTags(drifted, "ledger,netgo")
	if err == nil || !strings.Contains(err.Error(), `build "2" -tags`) {
		t.Fatalf("drifted release tags error = %v, want per-build rejection", err)
	}

	err = validateGoreleaserReleaseTags(matching, "netgo,cgo")
	if err == nil || !strings.Contains(err.Error(), "do not match -release-tags") {
		t.Fatalf("mismatched Make release tags error = %v, want parity rejection", err)
	}

	missing := writeTempFile(t, ".goreleaser.yaml", `builds:
  - id: tagged
    main: ./cmd/akt
    flags:
      - -tags=netgo,ledger
  - id: untagged
    main: ./cmd/akt
    flags:
      - -trimpath
# A comment is not a build flag: -tags=netgo,ledger
`)
	err = validateGoreleaserReleaseTags(missing, "ledger,netgo")
	if err == nil || !strings.Contains(err.Error(), `build "untagged" must have exactly one -tags flag`) {
		t.Fatalf("missing release tags error = %v, want untagged-build rejection", err)
	}

	wrongMain := writeTempFile(t, ".goreleaser.yaml", `builds:
  - id: auxiliary
    main: ./cmd/helper
    flags:
      - -tags=netgo,ledger
`)
	err = validateGoreleaserReleaseTags(wrongMain, "ledger,netgo")
	if err == nil || !strings.Contains(err.Error(), `build "auxiliary" main package must be ./cmd/akt`) {
		t.Fatalf("wrong main package error = %v, want shipped-binary rejection", err)
	}

	for _, test := range []struct {
		name     string
		filename string
		contents string
		want     string
	}{
		{name: "missing file", filename: filepath.Join(t.TempDir(), "missing.yaml"), want: "read goreleaser configuration"},
		{name: "malformed yaml", contents: "builds: [\n", want: "parse goreleaser configuration"},
		{name: "no builds", contents: "builds: []\n", want: "has no builds"},
		{name: "split flag missing value", contents: "builds:\n  - main: ./cmd/akt\n    flags:\n      - -tags\n", want: "-tags without a value"},
		{name: "invalid tag value", contents: "builds:\n  - main: ./cmd/akt\n    flags:\n      - -tags=-bad\n", want: "-tags value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			filename := test.filename
			if filename == "" {
				filename = writeTempFile(t, ".goreleaser.yaml", test.contents)
			}
			if err := validateGoreleaserReleaseTags(filename, "ledger,netgo"); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateGoreleaserReleaseTags() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestUncoveredChangedActiveLinesEnforcesActiveProduction(t *testing.T) {
	t.Parallel()

	const (
		activePackage = "pkg.akt.dev/akt/internal/active"
		tuiPackage    = "pkg.akt.dev/akt/internal/tui"
	)
	packages := packageSet{
		Module: "pkg.akt.dev/akt",
		Entries: []packageEntry{
			{Name: activePackage, Class: classActive},
			{Name: tuiPackage, Class: classExperimentalTUI},
		},
		ByName: map[string]packageEntry{
			activePackage: {Name: activePackage, Class: classActive},
			tuiPackage:    {Name: tuiPackage, Class: classExperimentalTUI},
		},
	}
	profile := coverProfile{Mode: "atomic", Blocks: []coverBlock{
		{File: activePackage + "/active.go", StartLine: 4, EndLine: 4, Count: 0},
		{File: activePackage + "/active.go", StartLine: 5, EndLine: 5, Count: 1},
		{File: tuiPackage + "/tui.go", StartLine: 8, EndLine: 8, Count: 0},
	}}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "active"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "internal", "active", "active.go"),
		[]byte("package active\n\nfunc run() {\n\tprintln(\"uncovered\")\n\tprintln(\"covered\")\n}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	changed := map[string]map[int]bool{
		"internal/active/active.go":      {4: true, 5: true},
		"internal/active/active_test.go": {9: true},
		"internal/tui/tui.go":            {8: true},
	}

	uncovered, checked, exempted, err := uncoveredChangedActiveLines(packages, profile, nil, changed, root)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 2 || exempted != 0 {
		t.Fatalf("checked/exempted = %d/%d, want 2/0", checked, exempted)
	}
	if len(uncovered) != 1 || !strings.Contains(uncovered[0], "internal/active/active.go:4") {
		t.Fatalf("uncovered = %q, want active.go:4", uncovered)
	}

	uncovered, checked, exempted, err = uncoveredChangedActiveLines(
		packages,
		profile,
		map[string]exception{
			exceptionKey(activePackage, "internal/active/active.go", 4): {
				Package: activePackage,
				File:    "internal/active/active.go",
				Line:    4,
			},
		},
		changed,
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncovered) != 0 || checked != 2 || exempted != 1 {
		t.Fatalf("reviewed exception result = uncovered %q, checked/exempted %d/%d", uncovered, checked, exempted)
	}
}

func TestUncoveredChangedActiveLinesRejectsExecutableFileMissingFromProfile(t *testing.T) {
	const packageName = "pkg.akt.dev/akt/internal/active"
	packages := packageSet{
		Module:  "pkg.akt.dev/akt",
		Entries: []packageEntry{{Name: packageName, Class: classActive}},
		ByName: map[string]packageEntry{
			packageName: {Name: packageName, Class: classActive},
		},
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "active"), 0o755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "internal", "active", "hidden.go")
	if err := os.WriteFile(filename, []byte("package active\n\nfunc hidden() { println(\"executed\") }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uncovered, checked, exempted, err := uncoveredChangedActiveLines(
		packages,
		coverProfile{Mode: "atomic"},
		nil,
		map[string]map[int]bool{"internal/active/hidden.go": {1: true}},
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 0 || exempted != 0 {
		t.Fatalf("checked/exempted = %d/%d, want 0/0", checked, exempted)
	}
	if len(uncovered) != 1 || !strings.Contains(uncovered[0], "absent from the coverage profile") {
		t.Fatalf("uncovered = %q, want missing-file failure", uncovered)
	}
}

func TestUncoveredChangedActiveLinesChecksModuleRootPackage(t *testing.T) {
	t.Parallel()

	const module = "pkg.akt.dev/akt"
	packages := packageSet{
		Module:  module,
		Entries: []packageEntry{{Name: module, Class: classActive}},
		ByName: map[string]packageEntry{
			module: {Name: module, Class: classActive},
		},
	}
	profile := coverProfile{Mode: "atomic", Blocks: []coverBlock{{
		File: module + "/root.go", StartLine: 3, EndLine: 3, Statements: 1, Count: 0,
	}}}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "root.go"), []byte("package akt\n\nvar value = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	uncovered, checked, exempted, err := uncoveredChangedActiveLines(
		packages,
		profile,
		nil,
		map[string]map[int]bool{"root.go": {3: true}},
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 1 || exempted != 0 {
		t.Fatalf("checked/exempted = %d/%d, want 1/0", checked, exempted)
	}
	if len(uncovered) != 1 || !strings.Contains(uncovered[0], "root.go:3") {
		t.Fatalf("uncovered = %q, want root.go:3", uncovered)
	}
}

func TestFileHasExecutableStatementsIgnoresDeclarationOnlyFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	filename := filepath.Join(root, "types.go")
	if err := os.WriteFile(filename, []byte("package example\n\ntype Value struct { Name string }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hasStatements, err := fileHasExecutableStatements(root, "types.go")
	if err != nil {
		t.Fatal(err)
	}
	if hasStatements {
		t.Fatal("declaration-only source was classified as executable")
	}
	if _, err := fileHasExecutableStatements(root, "missing.go"); err == nil || !strings.Contains(err.Error(), "inspect changed active source") {
		t.Fatalf("missing fileHasExecutableStatements() error = %v", err)
	}
}

func TestFileExecutableLinesIgnoresBlankAndClosingDelimiterLines(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const source = `package example

func choose(ok bool) {
	if ok {
		println("yes")
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "example.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := fileExecutableLines(root, "example.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []int{4, 5} {
		if !lines[line] {
			t.Errorf("line %d was not classified as executable", line)
		}
	}
	for _, line := range []int{2, 3, 6, 7} {
		if lines[line] {
			t.Errorf("blank/closing-delimiter line %d was classified as executable", line)
		}
	}
}

func TestUncoveredChangedActiveLinesRejectsInitializerWithoutCoverageBlock(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/active"
	packages := packageSet{
		Module:  "pkg.akt.dev/akt",
		Entries: []packageEntry{{Name: packageName, Class: classActive}},
		ByName: map[string]packageEntry{
			packageName: {Name: packageName, Class: classActive},
		},
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "active"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "internal", "active", "active.go"),
		[]byte("package active\n\nfunc covered() { println(\"covered\") }\n\nvar initialized = buildValue()\n\nfunc buildValue() int { return 1 }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	profile := coverProfile{Mode: "atomic", Blocks: []coverBlock{{
		File: packageName + "/active.go", StartLine: 3, EndLine: 3, Statements: 1, Count: 1,
	}}}

	uncovered, checked, exempted, err := uncoveredChangedActiveLines(
		packages,
		profile,
		nil,
		map[string]map[int]bool{"internal/active/active.go": {5: true}},
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 1 || exempted != 0 {
		t.Fatalf("checked/exempted = %d/%d, want 1/0", checked, exempted)
	}
	if len(uncovered) != 1 || !strings.Contains(uncovered[0], "no coverage block intersects executable syntax") {
		t.Fatalf("uncovered = %q, want uncovered initializer", uncovered)
	}

	uncovered, checked, exempted, err = uncoveredChangedActiveLines(
		packages,
		profile,
		map[string]exception{
			exceptionKey(packageName, "internal/active/active.go", 5): {
				Package: packageName,
				File:    "internal/active/active.go",
				Line:    5,
			},
		},
		map[string]map[int]bool{"internal/active/active.go": {5: true}},
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncovered) != 0 || checked != 1 || exempted != 1 {
		t.Fatalf("reviewed initializer exception result = uncovered %q, checked/exempted %d/%d", uncovered, checked, exempted)
	}
}

func TestUncoveredChangedActiveLinesUsesEnclosingMultilineStatement(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/active"
	packages := packageSet{
		Module:  "pkg.akt.dev/akt",
		Entries: []packageEntry{{Name: packageName, Class: classActive}},
		ByName: map[string]packageEntry{
			packageName: {Name: packageName, Class: classActive},
		},
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "active"), 0o755); err != nil {
		t.Fatal(err)
	}
	const source = `package active

func launch() {
	go func() {
		println("covered")
	}()
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "active", "active.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name          string
		count         uint64
		wantUncovered bool
	}{
		{name: "statement executed", count: 1},
		{name: "statement not executed", count: 0, wantUncovered: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := coverProfile{Mode: "atomic", Blocks: []coverBlock{{
				File: packageName + "/active.go", StartLine: 4, EndLine: 5,
				Statements: 1, Count: test.count,
			}}}
			uncovered, checked, exempted, err := uncoveredChangedActiveLines(
				packages,
				profile,
				nil,
				map[string]map[int]bool{"internal/active/active.go": {6: true}},
				root,
			)
			if err != nil {
				t.Fatal(err)
			}
			if checked != 1 || exempted != 0 {
				t.Fatalf("checked/exempted = %d/%d, want 1/0", checked, exempted)
			}
			if got := len(uncovered) > 0; got != test.wantUncovered {
				t.Fatalf("uncovered = %q, want failure %t", uncovered, test.wantUncovered)
			}
		})
	}
}

func TestUncoveredChangedActiveLinesUsesOnlyExactSyntheticEdgeEvidence(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/active"
	packages := packageSet{
		Module:  "pkg.akt.dev/akt",
		Entries: []packageEntry{{Name: packageName, Class: classActive}},
		ByName: map[string]packageEntry{
			packageName: {Name: packageName, Class: classActive},
		},
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "active"), 0o755); err != nil {
		t.Fatal(err)
	}
	const source = `package active

func send(ch chan<- int, done <-chan struct{}) {
	select {
	case ch <- 1:
	case <-done:
	}
}

func covered() { println("covered") }
`
	if err := os.WriteFile(filepath.Join(root, "internal", "active", "active.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	profile := coverProfile{
		Mode: "atomic",
		Blocks: []coverBlock{
			{
				File: packageName + "/active.go", StartLine: 5, StartColumn: 1,
				EndLine: 6, EndColumn: 20, Statements: 1, Count: 1,
			},
			{
				File: packageName + "/active.go", StartLine: 10, StartColumn: 1,
				EndLine: 10, EndColumn: 38, Statements: 1, Count: 1,
			},
		},
		EdgeBlocks: []coverBlock{
			{File: packageName + "/active.go", StartLine: 5, StartColumn: 15, EndLine: 5, EndColumn: 15, Count: 1},
			{File: packageName + "/active.go", StartLine: 6, StartColumn: 14, EndLine: 6, EndColumn: 14, Count: 0},
		},
	}

	uncovered, checked, exempted, err := uncoveredChangedActiveLines(
		packages,
		profile,
		nil,
		map[string]map[int]bool{"internal/active/active.go": {5: true, 6: true}},
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 2 || exempted != 0 {
		t.Fatalf("checked/exempted = %d/%d, want 2/0", checked, exempted)
	}
	if len(uncovered) != 1 || !strings.Contains(uncovered[0], "active.go:6") {
		t.Fatalf("uncovered = %q, want only the unexecuted exact edge", uncovered)
	}

	profile.EdgeBlocks[0].StartColumn++
	profile.EdgeBlocks[0].EndColumn++
	uncovered, _, _, err = uncoveredChangedActiveLines(
		packages,
		profile,
		nil,
		map[string]map[int]bool{"internal/active/active.go": {5: true}},
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncovered) != 1 || !strings.Contains(uncovered[0], "no coverage block intersects executable syntax") {
		t.Fatalf("uncovered = %q, want mismatched edge position to fail closed", uncovered)
	}
}

func TestParseChangedGoLinesTracksOnlyNewSideOfHunks(t *testing.T) {
	t.Parallel()

	changed, err := parseChangedGoLines([]byte(`diff --git a/internal/example/a.go b/internal/example/a.go
--- a/internal/example/a.go
+++ b/internal/example/a.go
@@ -3,2 +3,3 @@
-old
+first
+second
 context
diff --git a/internal/example/deleted.go b/internal/example/deleted.go
--- a/internal/example/deleted.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package example
-func deleted() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	lines := changed["internal/example/a.go"]
	if len(lines) != 3 || !lines[3] || !lines[4] || !lines[5] {
		t.Fatalf("changed lines = %#v, want 3-5", lines)
	}
	if _, exists := changed["internal/example/deleted.go"]; exists {
		t.Fatalf("deleted-only file was classified as added: %#v", changed)
	}
}

func TestParseChangedGoLinesDoesNotTreatSourceAsAFileHeader(t *testing.T) {
	t.Parallel()

	changed, err := parseChangedGoLines([]byte(`diff --git a/internal/example/a.go b/internal/example/a.go
--- a/internal/example/a.go
+++ b/internal/example/a.go
@@ -3,0 +4,2 @@
+const message = ` + "`" + `
+++ b/internal/spoofed/spoofed.go
@@ -9 +10 @@
-old
+new
`))
	if err != nil {
		t.Fatal(err)
	}
	lines := changed["internal/example/a.go"]
	if len(lines) != 3 || !lines[4] || !lines[5] || !lines[10] {
		t.Fatalf("real file changed lines = %#v, want 4, 5, and 10", lines)
	}
	if _, exists := changed["internal/spoofed/spoofed.go"]; exists {
		t.Fatalf("source text spoofed a diff file header: %#v", changed)
	}
	if _, err := parseChangedGoLines([]byte(strings.Repeat("x", 70*1024))); err == nil {
		t.Fatal("parseChangedGoLines() accepted an oversized diff line")
	}
}

func TestAddWholeFileLinesIncludesEveryUntrackedLine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "example"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "internal", "example", "new.go"),
		[]byte("package example\n\nfunc added() {}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	changed := make(map[string]map[int]bool)
	if err := addWholeFileLines(changed, root, "internal/example/new.go"); err != nil {
		t.Fatal(err)
	}
	lines := changed["internal/example/new.go"]
	if len(lines) != 3 || !lines[1] || !lines[2] || !lines[3] {
		t.Fatalf("untracked lines = %#v, want 1-3", lines)
	}
}

func TestGoSourceReadersRejectSymlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n\nfunc execute() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.go")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	changed := make(map[string]map[int]bool)
	if err := addWholeFileLines(changed, root, "linked.go"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("addWholeFileLines() error = %v, want symlink rejection", err)
	}
	if _, _, err := fileExecutableSyntax(root, "linked.go"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("fileExecutableSyntax() error = %v, want symlink rejection", err)
	}
	if err := addWholeFileLines(changed, root, "../outside.go"); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unsafe addWholeFileLines() error = %v", err)
	}
	if err := addWholeFileLines(changed, root, "missing.go"); err == nil || !strings.Contains(err.Error(), "inspect untracked Go file") {
		t.Fatalf("missing addWholeFileLines() error = %v", err)
	}
	if _, _, err := fileExecutableSyntax(root, "../outside.go"); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unsafe fileExecutableSyntax() error = %v", err)
	}
	if _, _, err := fileExecutableSyntax(root, "missing.go"); err == nil || !strings.Contains(err.Error(), "inspect changed active source") {
		t.Fatalf("missing fileExecutableSyntax() error = %v", err)
	}
	malformed := filepath.Join(root, "malformed.go")
	if err := os.WriteFile(malformed, []byte("package malformed\nfunc {\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fileExecutableSyntax(root, "malformed.go"); err == nil || !strings.Contains(err.Error(), "parse changed active source") {
		t.Fatalf("malformed fileExecutableSyntax() error = %v", err)
	}
	empty := filepath.Join(root, "empty.go")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := addWholeFileLines(changed, root, "empty.go"); err != nil {
		t.Fatal(err)
	}
	if lines := changed["empty.go"]; len(lines) != 1 || !lines[1] {
		t.Fatalf("empty untracked file lines = %#v, want synthetic line 1", lines)
	}
	oversized := filepath.Join(root, "oversized.go")
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", 4*1024*1024+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := addWholeFileLines(changed, root, "oversized.go"); err == nil || !strings.Contains(err.Error(), "read untracked Go file") {
		t.Fatalf("oversized addWholeFileLines() error = %v", err)
	}
}

func TestEnclosingRegionBlocksSelectsSmallestStatement(t *testing.T) {
	t.Parallel()

	regions := []sourceRegion{{StartLine: 2, EndLine: 10}, {StartLine: 5, EndLine: 7}}
	blocks := []coverBlock{
		{StartLine: 2, EndLine: 2, Count: 1},
		{StartLine: 5, EndLine: 5, Count: 0},
	}
	selected := enclosingRegionBlocks(regions, blocks, 6)
	if len(selected) != 1 || selected[0].StartLine != 5 {
		t.Fatalf("enclosingRegionBlocks() = %+v, want inner statement block", selected)
	}
	if selected := enclosingRegionBlocks(regions, blocks, 20); len(selected) != 0 {
		t.Fatalf("enclosingRegionBlocks() outside any statement = %+v", selected)
	}
}

func TestValidateActiveProfileRejectsEmptyProfile(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/active"
	packages := packageSet{
		Entries: []packageEntry{{Name: packageName, Class: classActive}},
		ByName: map[string]packageEntry{
			packageName: {Name: packageName, Class: classActive},
		},
	}

	err := validateActiveProfile(packages, coverProfile{Mode: "atomic"})
	if err == nil || !strings.Contains(err.Error(), "no executable statements") {
		t.Fatalf("validateActiveProfile() error = %v, want empty-profile rejection", err)
	}
}

func TestValidateActiveProfileRejectsMissingActivePackage(t *testing.T) {
	t.Parallel()

	const (
		present = "pkg.akt.dev/akt/internal/present"
		missing = "pkg.akt.dev/akt/internal/missing"
	)
	packages := packageSet{
		Entries: []packageEntry{
			{Name: missing, Class: classActive},
			{Name: present, Class: classActive},
		},
		ByName: map[string]packageEntry{
			missing: {Name: missing, Class: classActive},
			present: {Name: present, Class: classActive},
		},
	}
	profile := coverProfile{Mode: "atomic", Blocks: []coverBlock{{
		File: present + "/present.go", Statements: 1, Count: 1,
	}}}

	err := validateActiveProfile(packages, profile)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("validateActiveProfile() error = %v, want missing-package rejection", err)
	}
}

func TestRatioLessUsesExactCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  statementCount
		want statementCount
		less bool
	}{
		{name: "same ratio different denominator", got: statementCount{9, 10}, want: statementCount{90, 100}, less: false},
		{name: "one statement regression", got: statementCount{89, 100}, want: statementCount{9, 10}, less: true},
		{name: "improvement", got: statementCount{91, 100}, want: statementCount{9, 10}, less: false},
		{name: "missing uncovered denominator", got: statementCount{}, want: statementCount{0, 80}, less: true},
		{name: "empty baseline", got: statementCount{}, want: statementCount{}, less: false},
		{name: "empty baseline to uncovered code", got: statementCount{0, 1}, want: statementCount{}, less: true},
		{name: "empty baseline to fully covered code", got: statementCount{1, 1}, want: statementCount{}, less: false},
		{
			name: "large counters cannot overflow into a false non-regression",
			got: statementCount{
				Covered: math.MaxInt64/3 - 1,
				Total:   math.MaxInt64 / 3,
			},
			want: statementCount{
				Covered: math.MaxInt64 - 2,
				Total:   math.MaxInt64 - 1,
			},
			less: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ratioLess(test.got, test.want); got != test.less {
				t.Fatalf("ratioLess(%+v, %+v) = %t, want %t", test.got, test.want, got, test.less)
			}
		})
	}
}

func TestPercentFloorUsesExactCounts(t *testing.T) {
	t.Parallel()

	if !belowPercentFloor(statementCount{Covered: 1, Total: math.MaxInt64}, 80) {
		t.Fatal("belowPercentFloor() accepted near-zero large-count coverage")
	}
	if belowPercentFloor(statementCount{Covered: math.MaxInt64 - 1, Total: math.MaxInt64}, 95) {
		t.Fatal("belowPercentFloor() rejected near-complete large-count coverage")
	}
}

func TestIntersectingBlocksRequiresEveryRangeOnChangedLine(t *testing.T) {
	t.Parallel()

	blocks := []coverBlock{
		{StartLine: 10, EndLine: 12, Count: 1},
		{StartLine: 11, EndLine: 11, Count: 0},
		{StartLine: 20, EndLine: 21, Count: 1},
	}
	got := intersectingBlocks(blocks, 11)
	if len(got) != 2 {
		t.Fatalf("intersectingBlocks() returned %d blocks, want 2", len(got))
	}
	if got[0].Count != 1 || got[1].Count != 0 {
		t.Fatalf("intersectingBlocks() = %+v, want covered and uncovered ranges", got)
	}
}

func TestGuardBaselineRejectsLoweredAndUndercoveredNewPackages(t *testing.T) {
	t.Parallel()

	packages := packageSet{
		Entries: []packageEntry{
			{Name: "pkg.akt.dev/akt/internal/existing", Class: classActive},
			{Name: "pkg.akt.dev/akt/internal/new", Class: classActive, Critical: true},
		},
		ByName: map[string]packageEntry{
			"pkg.akt.dev/akt/internal/existing": {Name: "pkg.akt.dev/akt/internal/existing", Class: classActive},
			"pkg.akt.dev/akt/internal/new":      {Name: "pkg.akt.dev/akt/internal/new", Class: classActive, Critical: true},
		},
	}
	current := map[string]statementCount{
		"pkg.akt.dev/akt/internal/existing": {Covered: 89, Total: 100},
		"pkg.akt.dev/akt/internal/new":      {Covered: 94, Total: 100},
	}
	head := map[string]statementCount{
		"pkg.akt.dev/akt/internal/existing": {Covered: 89, Total: 100},
		"pkg.akt.dev/akt/internal/new":      {Covered: 94, Total: 100},
		"@total":                            {Covered: 183, Total: 200},
	}
	prior := map[string]statementCount{
		"pkg.akt.dev/akt/internal/existing": {Covered: 90, Total: 100},
		"@total":                            {Covered: 90, Total: 100},
	}

	err := guardBaseline(packages, current, head, prior, nil, classActive)
	if err == nil {
		t.Fatal("guardBaseline() succeeded, want failures")
	}
	if !strings.Contains(err.Error(), "baseline for pkg.akt.dev/akt/internal/existing was lowered") {
		t.Fatalf("guardBaseline() error %q does not identify lowered baseline", err)
	}
	if !strings.Contains(err.Error(), "new package pkg.akt.dev/akt/internal/new is 94.00%; minimum is 95%") {
		t.Fatalf("guardBaseline() error %q does not enforce critical-package floor", err)
	}
}

func TestGuardInitialBaselineEnforcesOnlyPackagesAddedByBootstrap(t *testing.T) {
	t.Parallel()

	const (
		legacy   = "pkg.akt.dev/akt/internal/legacy"
		ordinary = "pkg.akt.dev/akt/internal/ordinary"
		critical = "pkg.akt.dev/akt/internal/critical"
		tooling  = "pkg.akt.dev/akt/tools/new-tool"
	)
	packages := packageSet{
		Entries: []packageEntry{
			{Name: legacy, Class: classActive},
			{Name: ordinary, Class: classActive},
			{Name: critical, Class: classActive, Critical: true},
			{Name: tooling, Class: classTooling},
		},
		ByName: map[string]packageEntry{
			legacy:   {Name: legacy, Class: classActive},
			ordinary: {Name: ordinary, Class: classActive},
			critical: {Name: critical, Class: classActive, Critical: true},
			tooling:  {Name: tooling, Class: classTooling},
		},
	}
	current := map[string]statementCount{
		legacy:   {Covered: 1, Total: 100},
		ordinary: {Covered: 79, Total: 100},
		critical: {Covered: 94, Total: 100},
		tooling:  {Covered: 1, Total: 100},
	}

	err := guardInitialBaseline(packages, current, map[string]bool{legacy: true}, classActive)
	if err == nil {
		t.Fatal("guardInitialBaseline() succeeded, want minimum failures")
	}
	for _, want := range []string{
		"initial package " + ordinary + " is 79.00%; minimum is 80%",
		"initial package " + critical + " is 94.00%; minimum is 95%",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("guardInitialBaseline() error %q does not contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), legacy) {
		t.Fatalf("guardInitialBaseline() rejected legacy package: %v", err)
	}
	if err := guardInitialBaseline(packages, current, map[string]bool{}, classTooling); err != nil {
		t.Fatalf("tooling bootstrap should ratchet from its measured value: %v", err)
	}
}

func TestSourcePackagesFromTreeUsesNULTerminatedGitPaths(t *testing.T) {
	t.Parallel()

	tree := []byte("cmd/akt/main.go\x00internal/example/example.go\x00internal/example/example_test.go\x00nested/go.mod\x00nested/hidden/hidden.go\x00vendor/vendor.go\x00_hidden/hidden.go\x00README.md\x00")
	got, err := sourcePackagesFromTree("example.test/repo", tree)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"example.test/repo/cmd/akt", "example.test/repo/internal/example"} {
		if !got[want] {
			t.Errorf("sourcePackagesFromTree() omitted %s: %#v", want, got)
		}
	}
	if len(got) != 2 {
		t.Fatalf("sourcePackagesFromTree() = %#v, want two production packages", got)
	}
	if _, err := sourcePackagesFromTree("example.test/repo", []byte("../escape.go\x00")); err == nil {
		t.Fatal("sourcePackagesFromTree() accepted an unsafe path")
	}
	if _, err := sourcePackagesAtRevision("definitely-missing-ref", "example.test/repo"); err == nil || !strings.Contains(err.Error(), "inspect source packages") {
		t.Fatalf("sourcePackagesAtRevision() error = %v, want missing-revision rejection", err)
	}
}

func TestGuardBaselineAllowsRemovedPackageButProtectsTotal(t *testing.T) {
	t.Parallel()

	packages := packageSet{ByName: map[string]packageEntry{}}
	prior := map[string]statementCount{
		"pkg.akt.dev/akt/internal/removed": {Covered: 5, Total: 10},
		"@total":                           {Covered: 5, Total: 10},
	}
	head := map[string]statementCount{"@total": {Covered: 5, Total: 10}}
	if err := guardBaseline(packages, nil, head, prior, nil, classActive); err != nil {
		t.Fatalf("guardBaseline() rejected a package removed from taxonomy: %v", err)
	}
	delete(head, "@total")
	if err := guardBaseline(packages, nil, head, prior, nil, classActive); err == nil || !strings.Contains(err.Error(), "baseline removed protected entry: @total") {
		t.Fatalf("guardBaseline() missing-total error = %v", err)
	}
}

func TestGuardBaselineCarriesHistoryAcrossClassMigration(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/migrated"
	packages := packageSet{ByName: map[string]packageEntry{
		packageName: {Name: packageName, Class: classActive},
	}}
	current := map[string]statementCount{packageName: {Covered: 89, Total: 100}}
	head := map[string]statementCount{
		packageName: {Covered: 89, Total: 100},
		"@total":    {Covered: 89, Total: 100},
	}
	prior := map[string]statementCount{"@total": {Covered: 89, Total: 100}}
	migrationPrior := map[string]statementCount{packageName: {Covered: 90, Total: 100}}

	err := guardBaseline(packages, current, head, prior, migrationPrior, classActive)
	if err == nil || !strings.Contains(err.Error(), "baseline for migrated package "+packageName+" was lowered") {
		t.Fatalf("guardBaseline() migration error = %v", err)
	}

	// Removing the row from its old denominator is legal once taxonomy moved it.
	oldPrior := map[string]statementCount{
		packageName: {Covered: 90, Total: 100},
		"@total":    {Covered: 90, Total: 100},
	}
	oldHead := map[string]statementCount{"@total": {Covered: 90, Total: 100}}
	if err := guardBaseline(packages, nil, oldHead, oldPrior, nil, classExperimentalTUI); err != nil {
		t.Fatalf("guardBaseline() rejected old-denominator migration: %v", err)
	}
}

func TestLoadMigratedPackageBaselinesAtRevision(t *testing.T) {
	repository := t.TempDir()
	runTestGit(t, repository, "init", "--quiet")
	runTestGit(t, repository, "config", "user.email", "coverage-test@example.invalid")
	runTestGit(t, repository, "config", "user.name", "coverage test")

	baselineDirectory := filepath.Join(repository, "coverage")
	if err := os.MkdirAll(baselineDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	baselines := map[string]string{
		activeBaselinePath:          "package\tcovered\ttotal\npkg.akt.dev/akt/internal/current\t1\t2\n@total\t1\t2\n",
		experimentalTUIBaselinePath: "package\tcovered\ttotal\npkg.akt.dev/akt/internal/migrated-tui\t5\t10\n@total\t5\t10\n",
		toolingBaselinePath:         "package\tcovered\ttotal\npkg.akt.dev/akt/internal/migrated-tool\t7\t8\n@total\t7\t8\n",
	}
	for filename, contents := range baselines {
		if err := os.WriteFile(filepath.Join(repository, filename), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runTestGit(t, repository, "add", "coverage")
	runTestGit(t, repository, "commit", "--quiet", "-m", "test baselines")
	revision := strings.TrimSpace(runTestGit(t, repository, "rev-parse", "HEAD"))

	t.Chdir(repository)
	got, err := loadMigratedPackageBaselinesAtRevision(revision, activeBaselinePath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]statementCount{
		"pkg.akt.dev/akt/internal/migrated-tui":  {Covered: 5, Total: 10},
		"pkg.akt.dev/akt/internal/migrated-tool": {Covered: 7, Total: 8},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadMigratedPackageBaselinesAtRevision() = %#v, want %#v", got, want)
	}

	const conflict = "package\tcovered\ttotal\npkg.akt.dev/akt/internal/migrated-tui\t6\t10\n@total\t6\t10\n"
	if err := os.WriteFile(filepath.Join(repository, toolingBaselinePath), []byte(conflict), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository, "add", toolingBaselinePath)
	runTestGit(t, repository, "commit", "--quiet", "-m", "conflicting baseline")
	revision = strings.TrimSpace(runTestGit(t, repository, "rev-parse", "HEAD"))
	_, err = loadMigratedPackageBaselinesAtRevision(revision, activeBaselinePath)
	if err == nil || !strings.Contains(err.Error(), "conflicting prior baselines") {
		t.Fatalf("conflicting baselines error = %v", err)
	}
}

func TestCompareRatchetRequiresEveryPackageInCheckedInBaseline(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/new"
	packages := packageSet{
		Entries: []packageEntry{{Name: packageName, Class: classActive}},
		ByName: map[string]packageEntry{
			packageName: {Name: packageName, Class: classActive},
		},
	}
	current := map[string]statementCount{
		packageName: {Covered: 10, Total: 10},
		"@total":    {Covered: 10, Total: 10},
	}
	baseline := map[string]statementCount{
		"@total": {Covered: 10, Total: 10},
	}

	failures := compareRatchet(packages, current, baseline, classActive)
	if len(failures) != 1 || failures[0] != "checked-in baseline is missing package: "+packageName {
		t.Fatalf("compareRatchet() failures = %q", failures)
	}

	current[packageName] = statementCount{Covered: 1, Total: 10}
	current["@total"] = statementCount{Covered: 1, Total: 10}
	delete(baseline, "@total")
	baseline["pkg.akt.dev/akt/internal/stale"] = statementCount{Covered: 1, Total: 1}
	joined := strings.Join(compareRatchet(packages, current, baseline, classActive), "\n")
	for _, want := range []string{
		"new package " + packageName + " is 10.00%; minimum is 80%",
		"baseline is missing @total",
		"baseline has stale package: pkg.akt.dev/akt/internal/stale",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("compareRatchet() failures = %q, want %q", joined, want)
		}
	}
}

func TestCompareRatchetRequiresImprovementsInCheckedInBaseline(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/example"
	packages := packageSet{
		Entries: []packageEntry{{Name: packageName, Class: classActive}},
		ByName: map[string]packageEntry{
			packageName: {Name: packageName, Class: classActive},
		},
	}
	baseline := map[string]statementCount{
		packageName: {Covered: 50, Total: 100},
		"@total":    {Covered: 50, Total: 100},
	}
	improved := map[string]statementCount{
		packageName: {Covered: 80, Total: 100},
		"@total":    {Covered: 80, Total: 100},
	}

	failures := compareRatchet(packages, improved, baseline, classActive)
	joined := strings.Join(failures, "\n")
	if !strings.Contains(joined, packageName+" baseline is stale at 50.00%; raise it to the current 80.00%") ||
		!strings.Contains(joined, "aggregate baseline is stale at 50.00%; raise it to the current 80.00%") {
		t.Fatalf("compareRatchet() failures = %q, want package and aggregate stale-baseline failures", failures)
	}

	// Once the improvement is recorded, a later deletion of the tests that
	// produced it cannot fall back to the old floor.
	regressed := map[string]statementCount{
		packageName: {Covered: 50, Total: 100},
		"@total":    {Covered: 50, Total: 100},
	}
	failures = compareRatchet(packages, regressed, improved, classActive)
	joined = strings.Join(failures, "\n")
	if !strings.Contains(joined, packageName+" regressed from 80.00% to 50.00%") ||
		!strings.Contains(joined, "aggregate regressed from 80.00% to 50.00%") {
		t.Fatalf("second compareRatchet() failures = %q, want recorded-improvement regression", failures)
	}
}

func TestCompareRatchetRequiresExactCountsForEquivalentRatios(t *testing.T) {
	t.Parallel()

	const packageName = "pkg.akt.dev/akt/internal/example"
	packages := packageSet{Entries: []packageEntry{{Name: packageName, Class: classActive}}}
	current := map[string]statementCount{
		packageName: {Covered: 9, Total: 10},
		"@total":    {Covered: 9, Total: 10},
	}
	baseline := map[string]statementCount{
		packageName: {Covered: 90, Total: 100},
		"@total":    {Covered: 90, Total: 100},
	}

	joined := strings.Join(compareRatchet(packages, current, baseline, classActive), "\n")
	for _, want := range []string{
		packageName + " baseline counts are stale at 90/100; record the current 9/10",
		"aggregate baseline counts are stale at 90/100; record the current 9/10",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("compareRatchet() failures = %q, want %q", joined, want)
		}
	}
}

func TestRequireCanonicalBaselinePathRejectsRedirects(t *testing.T) {
	t.Parallel()

	got, err := requireCanonicalBaselinePath(classActive, "./coverage/baseline-active-union.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if got != activeBaselinePath {
		t.Fatalf("canonical path = %q, want %q", got, activeBaselinePath)
	}

	_, err = requireCanonicalBaselinePath(classActive, "coverage/renamed-baseline.tsv")
	if err == nil || !strings.Contains(err.Error(), activeBaselinePath) {
		t.Fatalf("redirected baseline error = %v, want canonical-path rejection", err)
	}

	got, err = requireCanonicalBaselinePath(classTooling, "./coverage/baseline-tooling-unit.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if got != toolingBaselinePath {
		t.Fatalf("tooling canonical path = %q, want %q", got, toolingBaselinePath)
	}
	got, err = requireCanonicalBaselinePath(classExperimentalTUI, "./coverage/baseline-experimental-tui-union.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if got != experimentalTUIBaselinePath {
		t.Fatalf("experimental TUI canonical path = %q, want %q", got, experimentalTUIBaselinePath)
	}
}

func TestLoadBaselineAtRevisionFailsClosedOnGitLookupErrors(t *testing.T) {
	repository := t.TempDir()
	runTestGit(t, repository, "init", "--quiet")
	runTestGit(t, repository, "config", "user.email", "coverage-test@example.invalid")
	runTestGit(t, repository, "config", "user.name", "coverage test")

	baselineDirectory := filepath.Join(repository, "coverage")
	if err := os.MkdirAll(baselineDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	const baseline = "package\tcovered\ttotal\npkg.akt.dev/akt/internal/example\t1\t2\n@total\t1\t2\n"
	if err := os.WriteFile(filepath.Join(baselineDirectory, "baseline.tsv"), []byte(baseline), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository, "add", "coverage/baseline.tsv")
	runTestGit(t, repository, "commit", "--quiet", "-m", "test baseline")
	revision := strings.TrimSpace(runTestGit(t, repository, "rev-parse", "HEAD"))

	t.Chdir(repository)
	got, exists, err := loadBaselineAtRevision(revision, "coverage/baseline.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || got["@total"] != (statementCount{Covered: 1, Total: 2}) {
		t.Fatalf("loadBaselineAtRevision() = (%v, %t), want committed baseline", got, exists)
	}
	if _, exists, err := loadBaselineAtRevision(revision, "coverage/absent.tsv"); err != nil || exists {
		t.Fatalf("absent baseline = (exists %t, error %v), want false and nil", exists, err)
	}
	if _, exists, err := loadBaselineAtRevision("missing-ref", "coverage/baseline.tsv"); err == nil || exists || !strings.Contains(err.Error(), "verify base revision") {
		t.Fatalf("missing revision = (exists %t, error %v), want verification failure", exists, err)
	}

	if err := os.WriteFile(filepath.Join(baselineDirectory, "baseline.tsv"), []byte("not a baseline\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository, "add", "coverage/baseline.tsv")
	runTestGit(t, repository, "commit", "--quiet", "-m", "malformed baseline")
	malformedRevision := strings.TrimSpace(runTestGit(t, repository, "rev-parse", "HEAD"))
	if _, exists, err := loadBaselineAtRevision(malformedRevision, "coverage/baseline.tsv"); err == nil || exists || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("malformed committed baseline = (exists %t, error %v), want parse failure", exists, err)
	}

	tree := strings.TrimSpace(runTestGit(t, repository, "rev-parse", revision+"^{tree}"))
	treeObject := filepath.Join(repository, ".git", "objects", tree[:2], tree[2:])
	if err := os.Remove(treeObject); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := loadBaselineAtRevision(revision, "coverage/baseline.tsv"); err == nil || exists || !strings.Contains(err.Error(), "inspect baseline") {
		t.Fatalf("corrupt tree = (exists %t, error %v), want fail-closed lookup error", exists, err)
	}
}

func TestProfileFileRelativeSupportsImportAndAbsolutePaths(t *testing.T) {
	t.Parallel()

	const module = "pkg.akt.dev/akt"
	for _, filename := range []string{
		"pkg.akt.dev/akt/internal/store/store.go",
		"/tmp/build/pkg.akt.dev/akt/internal/store/store.go",
	} {
		got, err := profileFileRelative(module, filename)
		if err != nil {
			t.Fatal(err)
		}
		if got != "internal/store/store.go" {
			t.Fatalf("profileFileRelative(%q) = %q", filename, got)
		}
	}
}

func writeTempFile(t *testing.T, name, contents string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func runTestGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output)
}
