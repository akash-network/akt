package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFinalCoverageReportsRequireCompleteProfiles(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("..", "..", "make", "testing.mk")
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	recipe := makeTargetRecipe(t, string(contents), "test-coverage-report")
	logicalLines := strings.ReplaceAll(recipe, "\\\n", " ")
	normalized := strings.Join(strings.Fields(logicalLines), " ")
	wants := []string{
		"-profile $(COVERAGE_REPOSITORY_PROFILE) -class repository -require-complete -out $(COVERAGE_REPORT_ROOT)/repository-union.tsv",
		"-profile $(COVERAGE_ACTIVE_PROFILE) -class active -require-complete -out $(COVERAGE_REPORT_ROOT)/active-union.tsv",
		"-profile $(COVERAGE_TUI_PROFILE) -class experimental-tui -require-complete -out $(COVERAGE_REPORT_ROOT)/experimental-tui-union.tsv",
		"-profile $(COVERAGE_TOOLING_PROFILE) -class tooling -require-complete -out $(COVERAGE_REPORT_ROOT)/tooling-unit.tsv",
	}
	for _, want := range wants {
		if !strings.Contains(normalized, want) {
			t.Errorf("final coverage recipe is missing completeness contract %q", want)
		}
	}
}

func makeTargetRecipe(t *testing.T, contents, target string) string {
	t.Helper()

	lines := strings.Split(contents, "\n")
	found := false
	var recipe []string
	for _, line := range lines {
		if !found {
			if strings.HasPrefix(line, target+":") {
				found = true
			}
			continue
		}
		if strings.HasPrefix(line, "\t") {
			recipe = append(recipe, line)
			continue
		}
		if len(recipe) > 0 {
			break
		}
	}
	if !found {
		t.Fatalf("make target %s is missing", target)
	}
	if len(recipe) == 0 {
		t.Fatalf("make target %s has no recipe", target)
	}
	return strings.Join(recipe, "\n")
}

func TestCoverageUploadsPreserveExactBaseChain(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	normalized := strings.Join(strings.Fields(string(contents)), " ")
	wants := []string{
		"github.event_name == 'push' && github.sha || github.ref",
		"cancel-in-progress: false",
		"codecov-upload: name: coverage-upload-main",
		"name: coverage-comparison-profiles",
		".cache/coverage/reports/active-union.out",
		".cache/coverage/reports/experimental-tui-union.out",
		".cache/coverage/reports/tooling-unit.out",
		".cache/coverage/reports/codecov-pr-metadata.tsv",
		"name: Wait for exact predecessor coverage uploads",
		"git cat-file -e \"$TESTED_SHA:codecov.yml\"",
		"git cat-file -e \"$CODECOV_PARENT_SHA:codecov.yml\"",
		"required_flags=(active-union experimental-tui tooling)",
		"/repos/${REPOSITORY#*/}/commits/$CODECOV_PARENT_SHA/uploads/",
		"--connect-timeout 5 --max-time 20",
		".state == \"merged\"",
	}
	for _, want := range wants {
		if !strings.Contains(normalized, want) {
			t.Errorf("coverage workflow is missing exact-base contract %q", want)
		}
	}
	if got := strings.Count(normalized, "name: Wait for exact predecessor coverage uploads"); got != 1 {
		t.Errorf("main exact-predecessor readiness checks = %d, want 1", got)
	}
	if got := strings.Count(normalized, "--connect-timeout 5 --max-time 20"); got != 1 {
		t.Errorf("bounded main Codecov readiness requests = %d, want 1", got)
	}
	if strings.Contains(normalized, "codecov-pr-upload:") || strings.Contains(normalized, "CODECOV_PR_RESULT") {
		t.Error("pull-request Codecov authentication remains inside the source CI workflow")
	}
	uploadCount := strings.Count(normalized, "uses: actions/upload-artifact@")
	if overwriteCount := strings.Count(normalized, "overwrite: true"); overwriteCount != uploadCount {
		t.Errorf("rerunnable CI artifacts with overwrite enabled = %d, want %d", overwriteCount, uploadCount)
	}
	if strings.Contains(normalized, "github.head_ref") {
		t.Error("contributor-controlled head_ref crosses the Codecov CLI boundary")
	}
	firstWait := strings.Index(normalized, "name: Wait for exact predecessor coverage uploads")
	firstUpload := strings.Index(normalized, "uses: codecov/codecov-action@")
	if firstWait < 0 || firstUpload < 0 || firstWait >= firstUpload {
		t.Error("main exact-predecessor wait must run before its first Codecov upload")
	}
	if got := strings.Count(normalized, "commit_parent: ${{ env.CODECOV_PARENT_SHA }}"); got != 8 {
		t.Errorf("main Codecov uploads with the exact predecessor = %d, want 8", got)
	}
}

func TestPullRequestCodecovUploadsUseTrustedWorkflowRun(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("..", "..", ".github", "workflows", "codecov-pr.yml")
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	normalized := strings.Join(strings.Fields(string(contents)), " ")
	wants := []string{
		"workflow_run: workflows: [ci] types: [completed]",
		"github.event.workflow_run.event == 'pull_request'",
		"github.event.workflow_run.conclusion == 'success'",
		"github.event.workflow_run.pull_requests[0].base.ref == 'main'",
		"id-token: write",
		"WORKFLOW_PATH: ${{ github.event.workflow_run.path }}",
		"test \"$WORKFLOW_PATH\" = .github/workflows/ci.yml",
		"select(.name == \"coverage-report\" and .conclusion == \"success\")",
		"select(.name == \"required-ci\" and .conclusion == \"success\")",
		"read -r first_parent second_parent extra_parent < <(git show -s --format=%P \"$tested_sha\")",
		"test \"$first_parent\" = \"$base_sha\"",
		"test \"$second_parent\" = \"$head_sha\"",
		"name: coverage-comparison-profiles",
		"github-token: ${{ github.token }}",
		"run-id: ${{ github.event.workflow_run.id }}",
		"expected=(active-union.out codecov-pr-metadata.tsv experimental-tui-union.out tooling-unit.out)",
		"name: Wait for exact base coverage uploads",
		"required_flags=(active-union experimental-tui tooling)",
		"--connect-timeout 5 --max-time 20",
		"override_commit: ${{ steps.metadata.outputs.tested_sha }}",
		"override_pr: ${{ steps.metadata.outputs.pr_number }}",
		"override_branch: pr-${{ steps.metadata.outputs.pr_number }}",
		"commit_parent: ${{ steps.metadata.outputs.base_sha }}",
		"use_oidc: true",
	}
	for _, want := range wants {
		if !strings.Contains(normalized, want) {
			t.Errorf("trusted pull-request upload workflow is missing %q", want)
		}
	}
	if got := strings.Count(normalized, "uses: codecov/codecov-action@"); got != 3 {
		t.Errorf("pull-request comparison uploads = %d, want 3", got)
	}
	if got := strings.Count(normalized, "use_oidc: true"); got != 3 {
		t.Errorf("OIDC-authenticated pull-request uploads = %d, want 3", got)
	}
	if got := strings.Count(normalized, "override_branch: pr-${{ steps.metadata.outputs.pr_number }}"); got != 3 {
		t.Errorf("validated synthetic pull-request branch overrides = %d, want 3", got)
	}
	if strings.Contains(normalized, "actions/checkout@") || strings.Contains(normalized, "github.head_ref") ||
		strings.Contains(normalized, "CODECOV_TOKEN") {
		t.Error("trusted pull-request uploader checks out code, accepts head_ref, or uses a reusable token")
	}
	wait := strings.Index(normalized, "name: Wait for exact base coverage uploads")
	upload := strings.Index(normalized, "uses: codecov/codecov-action@")
	if wait < 0 || upload < 0 || wait >= upload {
		t.Error("pull-request exact-base wait must run before its first Codecov upload")
	}
}

// fakeBinst records its arguments and writes the files the real binst would,
// so the release-installer script can be exercised without the network.
const fakeBinst = `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$BINST_LOG"
command=$1
shift
if [[ "${BINST_FAIL:-}" == "$command" ]]; then
  exit 3
fi
config='' output='' version=''
while [[ $# -gt 0 ]]; do
  case "$1" in
    --config) config=$2; shift 2 ;;
    -o) output=$2; shift 2 ;;
    --version|--target-version) version=$2; shift 2 ;;
    *) shift ;;
  esac
done
case "$command" in
  embed-checksums) printf 'embedded: %s\n' "$version" >> "$config" ;;
  gen) { printf '#!/bin/sh\n# pinned %s\n' "$version"; cat "$config"; } > "$output" ;;
esac
`

func TestReleaseInstallerScript(t *testing.T) {
	t.Parallel()

	script, err := filepath.Abs(filepath.Join("..", "..", "script", "release-installer.sh"))
	if err != nil {
		t.Fatal(err)
	}
	const config = "schema: v1\nname: akt\n"
	const manifest = "1111111111111111111111111111111111111111111111111111111111111111  akt_1.2.3_darwin_all.zip\n" +
		"2222222222222222222222222222222222222222222222222222222222222222  akt_1.2.3_linux_amd64.zip\n"
	sum := func(contents string) string {
		digest := sha256.Sum256([]byte(contents))
		return hex.EncodeToString(digest[:])
	}

	for _, tc := range []struct {
		name      string
		manifests map[string]string
		args      []string
		fail      string
		wantError string
	}{
		{name: "pins the only manifest", manifests: map[string]string{"akt_1.2.3_checksums.txt": manifest}},
		{name: "accepts the event tag", manifests: map[string]string{"akt_1.2.3_checksums.txt": manifest}, args: []string{"v1.2.3"}},
		{name: "terminates an unterminated manifest", manifests: map[string]string{"akt_1.2.3_checksums.txt": strings.TrimSuffix(manifest, "\n")}},
		{name: "rejects another tag", manifests: map[string]string{"akt_1.2.3_checksums.txt": manifest}, args: []string{"v1.2.4"}, wantError: "belongs to v1.2.3, not v1.2.4"},
		{name: "requires a manifest", wantError: "found 0"},
		{name: "rejects ambiguous manifests", manifests: map[string]string{"akt_1.2.3_checksums.txt": manifest, "akt_1.2.4_checksums.txt": manifest}, wantError: "found 2"},
		{name: "propagates embed failure", manifests: map[string]string{"akt_1.2.3_checksums.txt": manifest}, fail: "embed-checksums", wantError: "exit status 3"},
		{name: "propagates generation failure", manifests: map[string]string{"akt_1.2.3_checksums.txt": manifest}, fail: "gen", wantError: "exit status 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			binst := filepath.Join(root, "binst")
			logFile := filepath.Join(root, "binst.log")
			configFile := filepath.Join(root, "binstaller.yml")
			dist := filepath.Join(root, "dist")
			out := filepath.Join(root, "installer")
			commandTestWriteFile(t, root, "binst", fakeBinst)
			if err := os.Chmod(binst, 0o755); err != nil {
				t.Fatal(err)
			}
			commandTestWriteFile(t, root, "binstaller.yml", config)
			if err := os.MkdirAll(dist, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, contents := range tc.manifests {
				commandTestWriteFile(t, dist, name, contents)
			}
			// Leftovers from an earlier run must never survive a failed run.
			commandTestWriteFile(t, out, "install.sh", "stale\n")
			commandTestWriteFile(t, out, "attestation-subjects.txt", "stale\n")

			// Executed directly, as make/releasing.mk does, so a script
			// committed without its executable bit fails here, not at release.
			command := exec.Command(script, append([]string{binst, configFile, dist, out}, tc.args...)...)
			command.Env = append(os.Environ(), "BINST_LOG="+logFile, "BINST_FAIL="+tc.fail)
			output, err := command.CombinedOutput()

			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("release-installer succeeded, want error containing %q:\n%s", tc.wantError, output)
				}
				if !strings.Contains(err.Error()+"\n"+string(output), tc.wantError) {
					t.Fatalf("release-installer error = %v, want %q:\n%s", err, tc.wantError, output)
				}
				for _, name := range []string{"install.sh", "attestation-subjects.txt"} {
					if _, statErr := os.Stat(filepath.Join(out, name)); !os.IsNotExist(statErr) {
						t.Errorf("failed run left %s behind (stat error %v)", name, statErr)
					}
				}
				if tc.fail == "" {
					if _, statErr := os.Stat(logFile); !os.IsNotExist(statErr) {
						t.Errorf("binst ran before the manifest was validated:\n%s", commandTestReadFile(t, logFile))
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("release-installer: %v\n%s", err, output)
			}

			manifestFile := filepath.Join(dist, "akt_1.2.3_checksums.txt")
			copied := filepath.Join(out, "binstaller.yml")
			installer := filepath.Join(out, "install.sh")
			wantLog := "embed-checksums --config " + copied + " --mode checksum-file --file " + manifestFile + " --version v1.2.3 --quiet\n" +
				"gen --config " + copied + " --target-version v1.2.3 -o " + installer + "\n"
			if got := commandTestReadFile(t, logFile); got != wantLog {
				t.Errorf("binst invocations:\n%s\nwant:\n%s", got, wantLog)
			}
			if got := commandTestReadFile(t, configFile); got != config {
				t.Errorf("committed config was modified:\n%s", got)
			}
			generated := commandTestReadFile(t, installer)
			if !strings.Contains(generated, "# pinned v1.2.3\n") || !strings.Contains(generated, "embedded: v1.2.3\n") {
				t.Errorf("installer was not generated from the checksum-embedded config:\n%s", generated)
			}
			wantSubjects := strings.TrimSuffix(commandTestReadFile(t, manifestFile), "\n") + "\n" +
				sum(commandTestReadFile(t, manifestFile)) + "  akt_1.2.3_checksums.txt\n" +
				sum(generated) + "  install.sh\n"
			if got := commandTestReadFile(t, filepath.Join(out, "attestation-subjects.txt")); got != wantSubjects {
				t.Errorf("attestation subjects:\n%s\nwant:\n%s", got, wantSubjects)
			}
		})
	}
}

// fakeReleaseInstaller stands in for the binst-generated install.sh: it
// checks the environment the dry-run script promises, optionally fetches
// FAKE_URL with curl the way install.sh does, and reports embedded-checksum
// verification unless FAKE_NO_EMBEDDED is set.
const fakeReleaseInstaller = `#!/bin/sh
set -e
[ "$1" = -n ] || { echo "not a dry run: $*" >&2; exit 9; }
[ -z "${GITHUB_TOKEN:-}" ] || { echo "GITHUB_TOKEN reached the installer" >&2; exit 9; }
[ "${BINSTALLER_NO_PROGRESS:-}" = 1 ] || { echo "progress output is enabled" >&2; exit 9; }
echo "platform $BINSTALLER_OS/$BINSTALLER_ARCH"
if [ -n "${FAKE_URL:-}" ]; then
  tmp=$(mktemp -d)
  curl -fsSL -o "$tmp/asset" "$FAKE_URL"
  echo "fetched $(cat "$tmp/asset")"
  rm -rf "$tmp"
fi
[ -n "${FAKE_NO_EMBEDDED:-}" ] || echo "Using embedded checksum for verification"
# install.sh logs the marker before comparing digests, then fails.
[ -z "${FAKE_FAIL_AFTER_EMBEDDED:-}" ] || exit 3
`

// fakePublicCurl stands in for the system curl that public dry runs use.
const fakePublicCurl = `#!/bin/sh
out=''
while [ $# -gt 0 ]; do
  case "$1" in
  -o) out=$2; shift 2 ;;
  *) shift ;;
  esac
done
printf 'public-download' > "$out"
`

func TestReleaseInstallerDryRunScript(t *testing.T) {
	t.Parallel()

	script, err := filepath.Abs(filepath.Join("..", "..", "script", "release-installer-dry-run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	const release = "https://github.com/akash-network/akt/releases/download/v1.2.3/"
	local := func(platform string) []string {
		return []string{platform, "DIST", "akash-network/akt", "v1.2.3"}
	}

	for _, tc := range []struct {
		name      string
		args      []string
		env       []string
		want      []string
		wantError string
	}{
		{
			name: "serves the release download URL from the dist directory",
			args: local("linux/amd64"),
			env:  []string{"FAKE_URL=" + release + "akt_1.2.3_linux_amd64.zip"},
			want: []string{"platform linux/amd64", "fetched linux-amd64-archive", "Using embedded checksum"},
		},
		{
			name:      "refuses another tag",
			args:      local("linux/amd64"),
			env:       []string{"FAKE_URL=https://github.com/akash-network/akt/releases/download/v1.2.4/akt_1.2.3_linux_amd64.zip"},
			wantError: "curl stand-in: refusing",
		},
		{
			name:      "refuses another repository",
			args:      local("linux/amd64"),
			env:       []string{"FAKE_URL=https://github.com/other/akt/releases/download/v1.2.3/akt_1.2.3_linux_amd64.zip"},
			wantError: "curl stand-in: refusing",
		},
		{
			name:      "refuses a path below the release",
			args:      local("linux/amd64"),
			env:       []string{"FAKE_URL=" + release + "nested/akt_1.2.3_linux_amd64.zip"},
			wantError: "curl stand-in: refusing",
		},
		{
			name:      "fails for an asset the release does not have",
			args:      local("linux/arm64"),
			env:       []string{"FAKE_URL=" + release + "akt_1.2.3_linux_arm64.zip"},
			wantError: "akt_1.2.3_linux_arm64.zip is not in",
		},
		{
			name:      "requires embedded checksum verification",
			args:      local("darwin/arm64"),
			env:       []string{"FAKE_NO_EMBEDDED=1"},
			wantError: "darwin/arm64: archive was not verified with an embedded checksum",
		},
		{
			name:      "fails when checksum verification fails after the marker",
			args:      local("linux/amd64"),
			env:       []string{"FAKE_FAIL_AFTER_EMBEDDED=1"},
			wantError: "linux/amd64: release installer dry run failed",
		},
		{
			name: "runs against the public release with the system curl",
			args: []string{"darwin/amd64"},
			env:  []string{"FAKE_URL=" + release + "akt_1.2.3_darwin_all.zip"},
			want: []string{"platform darwin/amd64", "fetched public-download", "Using embedded checksum"},
		},
		{name: "rejects a platform without an architecture", args: []string{"linux"}, wantError: "platform must be OS/ARCH"},
		{name: "rejects a partial local source", args: []string{"linux/amd64", "DIST"}, wantError: "usage:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			installer := commandTestWriteFile(t, root, "install.sh", fakeReleaseInstaller)
			dist := filepath.Join(root, "dist")
			commandTestWriteFile(t, dist, "akt_1.2.3_linux_amd64.zip", "linux-amd64-archive")
			// The "system" curl: public dry runs must use it, and the local
			// stand-in must shadow it.
			systemBin := filepath.Join(root, "system-bin")
			commandTestWriteFile(t, systemBin, "curl", fakePublicCurl)
			if err := os.Chmod(filepath.Join(systemBin, "curl"), 0o755); err != nil {
				t.Fatal(err)
			}
			args := []string{installer}
			for _, arg := range tc.args {
				if arg == "DIST" {
					arg = dist
				}
				args = append(args, arg)
			}

			// Executed directly, as release.yml does, so a script committed
			// without its executable bit fails here. GITHUB_TOKEN is set to
			// prove the installer never sees it.
			command := exec.Command(script, args...)
			command.Env = append(append(os.Environ(),
				"GITHUB_TOKEN=must-not-leak", "PATH="+systemBin+string(os.PathListSeparator)+os.Getenv("PATH")), tc.env...)
			output, err := command.CombinedOutput()

			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("dry run succeeded, want error containing %q:\n%s", tc.wantError, output)
				}
				if !strings.Contains(string(output), tc.wantError) {
					t.Fatalf("dry run error output does not contain %q:\n%s", tc.wantError, output)
				}
				return
			}
			if err != nil {
				t.Fatalf("dry run: %v\n%s", err, output)
			}
			for _, want := range tc.want {
				if !strings.Contains(string(output), want) {
					t.Errorf("dry run output does not contain %q:\n%s", want, output)
				}
			}
		})
	}
}

type workflowContract struct {
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Permissions map[string]string `yaml:"permissions"`
		Steps       []workflowStep    `yaml:"steps"`
	} `yaml:"jobs"`
}

type workflowStep struct {
	If              string            `yaml:"if"`
	ContinueOnError string            `yaml:"continue-on-error"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	Env             map[string]string `yaml:"env"`
	With            map[string]string `yaml:"with"`
}

// isInstallerDryRun reports whether step dry-runs the release installer; a
// dry run against prepared files names GoReleaser's dist directory.
func isInstallerDryRun(step workflowStep) bool {
	return strings.Contains(step.Run, "script/release-installer-dry-run.sh")
}

func readWorkflowContract(t *testing.T, name string) workflowContract {
	t.Helper()

	filename := filepath.Join("..", "..", ".github", "workflows", name)
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	var workflow workflowContract
	if err := yaml.Unmarshal(contents, &workflow); err != nil {
		t.Fatalf("decode %s: %v", filename, err)
	}
	return workflow
}

func workflowStepIndex(steps []workflowStep, match func(workflowStep) bool) int {
	for index, step := range steps {
		if match(step) {
			return index
		}
	}
	return -1
}

func runsCommand(step workflowStep, command string) bool {
	for _, line := range strings.Split(step.Run, "\n") {
		if strings.TrimSpace(line) == command {
			return true
		}
	}
	return false
}

func TestReleaseWorkflowAttestsInstallerBeforePublishingIt(t *testing.T) {
	t.Parallel()

	workflow := readWorkflowContract(t, "release.yml")
	for _, scope := range []string{"id-token", "attestations"} {
		if workflow.Permissions[scope] != "" {
			t.Errorf("workflow-level permissions grant %s", scope)
		}
		for _, job := range []string{"quality", "manual"} {
			if workflow.Jobs[job].Permissions[scope] != "" {
				t.Errorf("%s job is granted %s", job, scope)
			}
		}
	}

	quality := workflow.Jobs["quality"].Steps
	if workflowStepIndex(quality, func(step workflowStep) bool { return runsCommand(step, "make installer-check") }) < 0 {
		t.Error("release quality gate does not reject a stale install.sh")
	}

	publish := workflow.Jobs["publish"]
	for scope, want := range map[string]string{"contents": "write", "id-token": "write", "attestations": "write"} {
		if got := publish.Permissions[scope]; got != want {
			t.Errorf("publish permission %s = %q, want %q", scope, got, want)
		}
	}
	steps := publish.Steps
	// Releases are immutable once published, so the installer must be
	// generated, dry-run and attested between GoReleaser's prepare and
	// publish steps, and nothing may be uploaded afterwards.
	prepare := workflowStepIndex(steps, func(step workflowStep) bool { return runsCommand(step, "make release-prepare") })
	generate := workflowStepIndex(steps, func(step workflowStep) bool {
		return runsCommand(step, `make release-installer RELEASE_TAG="$EVENT_TAG"`)
	})
	localDryRun := workflowStepIndex(steps, func(step workflowStep) bool {
		return isInstallerDryRun(step) && strings.Contains(step.Run, `.cache/goreleaser "$REPOSITORY" "$EVENT_TAG"`)
	})
	attest := workflowStepIndex(steps, func(step workflowStep) bool { return strings.HasPrefix(step.Uses, "actions/attest@") })
	release := workflowStepIndex(steps, func(step workflowStep) bool { return runsCommand(step, "make release-publish") })
	publicDryRun := workflowStepIndex(steps, func(step workflowStep) bool {
		return isInstallerDryRun(step) && !strings.Contains(step.Run, ".cache/goreleaser")
	})
	verify := workflowStepIndex(steps, func(step workflowStep) bool { return strings.Contains(step.Run, "gh attestation verify") })
	order := []struct {
		name  string
		index int
	}{
		{"GoReleaser prepare", prepare},
		{"installer generation for the event tag", generate},
		{"installer dry run against the prepared assets", localDryRun},
		{"attestation", attest},
		{"GoReleaser publication", release},
		{"installer dry run against the published release", publicDryRun},
		{"published installer verification", verify},
	}
	for position, step := range order {
		if step.index < 0 {
			t.Errorf("publish job is missing %s", step.name)
			continue
		}
		if position > 0 && order[position-1].index >= 0 && order[position-1].index >= step.index {
			t.Errorf("%s must run after %s", step.name, order[position-1].name)
		}
		// Ordering only protects publication if every earlier step runs and
		// stops the job when it fails.
		if s := steps[step.index]; s.If != "" || s.ContinueOnError != "" {
			t.Errorf("%s must always run and fail the job (if %q, continue-on-error %q)", step.name, s.If, s.ContinueOnError)
		}
	}
	if attest >= 0 {
		if got := steps[attest].With["subject-checksums"]; got != ".cache/installer/attestation-subjects.txt" {
			t.Errorf("attestation subjects = %q, want the generated subject list", got)
		}
	}
	for _, step := range steps {
		if strings.Contains(step.Run, "gh release upload") || runsCommand(step, "make release") {
			t.Errorf("publish job adds to the release outside GoReleaser's publication:\n%s", step.Run)
		}
	}
	const licenseKey = "${{ secrets.GORELEASER_KEY }}"
	if prepare >= 0 {
		if got := steps[prepare].Env["GORELEASER_KEY"]; got != licenseKey {
			t.Errorf("prepare step GORELEASER_KEY = %q, want %q", got, licenseKey)
		}
	}
	if release >= 0 {
		for name, want := range map[string]string{
			"GORELEASER_KEY":          licenseKey,
			"GITHUB_TOKEN":            "${{ secrets.GITHUB_TOKEN }}",
			"GORELEASER_ACCESS_TOKEN": "${{ secrets.GORELEASER_ACCESS_TOKEN }}",
		} {
			if got := steps[release].Env[name]; got != want {
				t.Errorf("publication step %s = %q, want %q", name, got, want)
			}
		}
	}
	if verify >= 0 {
		for _, want := range []string{"cmp ", `--signer-workflow="$REPOSITORY/.github/workflows/release.yml"`} {
			if !strings.Contains(steps[verify].Run, want) {
				t.Errorf("published installer verification is missing %q", want)
			}
		}
	}

	manual := workflow.Jobs["manual"].Steps
	if index := workflowStepIndex(manual, func(step workflowStep) bool { return runsCommand(step, "make release-installer") }); index < 0 {
		t.Error("manual snapshot and dry-run modes do not generate the installer")
	} else if manual[index].If != "inputs.mode != 'check'" {
		t.Errorf("manual installer generation condition = %q", manual[index].If)
	}
	if index := workflowStepIndex(manual, func(step workflowStep) bool {
		return isInstallerDryRun(step) && strings.Contains(step.Run, ".cache/goreleaser")
	}); index < 0 {
		t.Error("manual snapshot and dry-run modes do not dry-run the installer against their own build")
	} else if manual[index].If != "inputs.mode != 'check'" {
		t.Errorf("manual installer dry-run condition = %q", manual[index].If)
	}
	// Manual modes get the GoReleaser Pro license, which the dry-run's
	// prepare step needs, and never a publishing token.
	run := workflowStepIndex(manual, func(step workflowStep) bool { return strings.Contains(step.Run, `make "release-${MODE}"`) })
	if run < 0 {
		t.Error("manual job does not run the selected release mode")
	} else {
		if got := manual[run].Env["GORELEASER_KEY"]; got != licenseKey {
			t.Errorf("manual run GORELEASER_KEY = %q, want %q", got, licenseKey)
		}
		for _, name := range []string{"GITHUB_TOKEN", "GORELEASER_ACCESS_TOKEN"} {
			if _, ok := manual[run].Env[name]; ok {
				t.Errorf("manual run receives publishing token %s", name)
			}
		}
	}
	artifacts := workflowStepIndex(manual, func(step workflowStep) bool { return strings.HasPrefix(step.Uses, "actions/upload-artifact@") })
	if artifacts < 0 || !strings.Contains(manual[artifacts].With["path"], ".cache/installer/install.sh") {
		t.Error("manual artifacts do not include the generated installer")
	}
}

func TestInstallerDriftCheckRunsInCI(t *testing.T) {
	t.Parallel()

	workflow := readWorkflowContract(t, "ci.yml")
	job, ok := workflow.Jobs["installer"]
	if !ok {
		t.Fatal("ci workflow has no installer job")
	}
	if workflowStepIndex(job.Steps, func(step workflowStep) bool { return runsCommand(step, "make installer-check") }) < 0 {
		t.Error("installer job does not run make installer-check")
	}
}

func TestReadmeInstallerVerifiesReleaseWorkflow(t *testing.T) {
	t.Parallel()

	repository := strings.TrimPrefix(strings.TrimSpace(commandTestReadFile(t, filepath.Join("..", "..", ".github", "repo"))), "github.com/")
	readme := commandTestReadFile(t, filepath.Join("..", "..", "README.md"))
	for _, want := range []string{
		"https://github.com/" + repository + "/releases/latest/download/install.sh",
		"--repo=" + repository,
		"--signer-workflow=" + repository + "/.github/workflows/release.yml",
		"https://raw.githubusercontent.com/" + repository + "/main/install.sh",
		"--signer-workflow=" + repository + "/.github/workflows/installer-attest.yml",
		"--source-ref=refs/heads/main",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README installer instructions are missing %q", want)
		}
	}
	if strings.Contains(readme, "trusted-go-releaser") {
		t.Error("README verifies installers against a workflow that does not sign akt releases")
	}
}

func TestReleaseInstallerTargetUsesWorkflowLayout(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// release.yml reads the installer from .cache/installer relative to the
	// checkout, so the target must not follow a relocated developer cache.
	command := exec.Command("make", "-n", "-s", "-C", root, "release-installer",
		"AKT_ROOT="+root, "AKT_DEVCACHE="+filepath.Join(t.TempDir(), "cache"),
		"BINST=/fake/binst", "RELEASE_TAG=v9.9.9")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n release-installer: %v\n%s", err, output)
	}
	normalized := strings.Join(strings.Fields(strings.ReplaceAll(string(output), "\\\n", " ")), " ")
	want := `"` + root + `/script/release-installer.sh" "/fake/binst" "` + root + `/.config/binstaller.yml" "` +
		root + `/.cache/goreleaser" "` + root + `/.cache/installer" "v9.9.9"`
	if !strings.Contains(normalized, want) {
		t.Errorf("release-installer recipe does not run\n%s\nmake printed:\n%s", want, output)
	}
}

// makeDryRunRelease prints the commands `make target` would run, with the
// GoReleaser knobs pinned so the developer's environment cannot change them.
func makeDryRunRelease(t *testing.T, target string) (string, error) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("make", "-n", "-s", "-C", root, target,
		"AKT_ROOT="+root, "AKT_DEVCACHE="+filepath.Join(t.TempDir(), "cache"),
		"GORELEASER_VERBOSE=false", "GORELEASER_SKIP=", "GORELEASER_MOUNT_CONFIG=false")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	return string(output), err
}

// dockerRunLine returns the single `docker run` command in make's output.
func dockerRunLine(t *testing.T, target, output string) string {
	t.Helper()

	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "docker run ") {
			lines = append(lines, strings.Join(strings.Fields(line), " "))
		}
	}
	if len(lines) != 1 {
		t.Fatalf("make %s runs %d docker commands, want 1:\n%s", target, len(lines), output)
	}
	return lines[0]
}

func TestReleaseTargetsSplitPublication(t *testing.T) {
	t.Parallel()

	var config struct {
		Dist string `yaml:"dist"`
	}
	if err := yaml.Unmarshal([]byte(commandTestReadFile(t, filepath.Join("..", "..", ".goreleaser.yaml"))), &config); err != nil {
		t.Fatal(err)
	}
	const image = "ghcr.io/goreleaser/goreleaser-cross-pro:"
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		target    string
		suffix    string
		preflight bool
		tokens    bool
	}{
		{target: "release-dry-run", suffix: " release --clean --prepare --skip=validate"},
		{target: "release-prepare", suffix: " release --clean --prepare", preflight: true},
		{target: "release-publish", suffix: " publish --dist " + config.Dist, preflight: true, tokens: true},
	} {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()

			output, err := makeDryRunRelease(t, tc.target)
			if err != nil {
				t.Fatalf("make -n %s: %v\n%s", tc.target, err, output)
			}
			docker := dockerRunLine(t, tc.target, output)
			if !strings.HasSuffix(docker, tc.suffix) {
				t.Errorf("make %s runs\n%s\nwant it to end with %q", tc.target, docker, tc.suffix)
			}
			if !strings.Contains(docker, " "+image) || !strings.Contains(docker, "@sha256:") {
				t.Errorf("make %s does not run the digest-pinned GoReleaser Pro image:\n%s", tc.target, docker)
			}
			if !strings.Contains(docker, " -e GORELEASER_KEY ") {
				t.Errorf("make %s does not pass the GoReleaser Pro license:\n%s", tc.target, docker)
			}
			for _, token := range []string{"GITHUB_TOKEN", "GORELEASER_ACCESS_TOKEN"} {
				if got := strings.Contains(docker, " -e "+token+" "); got != tc.tokens {
					t.Errorf("make %s passes %s to GoReleaser = %t, want %t:\n%s", tc.target, token, got, tc.tokens, docker)
				}
			}
			for _, want := range []string{"GITHUB_TOKEN is required", "GORELEASER_KEY (the GoReleaser Pro license) is required"} {
				if got := strings.Contains(output, want); got != tc.preflight {
					t.Errorf("make %s runs the publication preflight check %q = %t, want %t", tc.target, want, got, tc.preflight)
				}
			}
		})
	}

	// publish uploads release.extra_files, so the installer generated for
	// this prepared release must be checked before GoReleaser creates it.
	output, err := makeDryRunRelease(t, "release-publish")
	if err != nil {
		t.Fatalf("make -n release-publish: %v\n%s", err, output)
	}
	guard := strings.Index(output, `installer="`+root+`/.cache/installer/install.sh"`)
	if guard < 0 || guard > strings.Index(output, "docker run ") {
		t.Errorf("release-publish does not verify the installer before publishing:\n%s", output)
	}

	// No one-shot target may publish a release without its installer.
	if output, err := makeDryRunRelease(t, "release"); err == nil || !strings.Contains(output, "No rule to make target") {
		t.Errorf("make release still exists (err %v):\n%s", err, output)
	}
}

func TestReleaseTargetsUseHostImageVariant(t *testing.T) {
	t.Parallel()

	// The GoReleaser image is a multi-arch index; Docker selects the host's
	// variant, so no target may pin one.
	for _, target := range []string{"release-check", "release-snapshot", "release-dry-run", "release-prepare", "release-publish"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			output, err := makeDryRunRelease(t, target)
			if err != nil {
				t.Fatalf("make -n %s: %v\n%s", target, err, output)
			}
			if docker := dockerRunLine(t, target, output); strings.Contains(docker, "--platform") {
				t.Errorf("make %s pins the image platform:\n%s", target, docker)
			}
		})
	}
}

func TestReleaseInstallerVerifyBindsInstallerToPreparedRelease(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sum := func(contents string) string {
		digest := sha256.Sum256([]byte(contents))
		return hex.EncodeToString(digest[:])
	}
	const manifest = "1111111111111111111111111111111111111111111111111111111111111111  akt_1.2.3_linux_amd64.zip\n"
	const installer = "#!/bin/sh\n# pinned v1.2.3\n"
	// What release-installer.sh writes for this manifest and installer.
	subjects := manifest + sum(manifest) + "  akt_1.2.3_checksums.txt\n" + sum(installer) + "  install.sh\n"

	for _, tc := range []struct {
		name      string
		manifests map[string]string
		installer string
		subjects  string
		wantError string
	}{
		{name: "accepts the installer generated for the prepared release", installer: installer, subjects: subjects},
		{name: "rejects an edited installer", installer: installer + "echo extra\n", subjects: subjects, wantError: "does not belong to the prepared release"},
		{
			name:      "rejects an installer generated for another release",
			manifests: map[string]string{"akt_1.2.3_checksums.txt": strings.Replace(manifest, "1111", "2222", 1)},
			installer: installer, subjects: subjects, wantError: "does not belong to the prepared release",
		},
		{name: "rejects a missing installer", subjects: subjects, wantError: "run make release-installer"},
		{name: "rejects a missing subject list", installer: installer, wantError: "run make release-installer"},
		{
			name:      "rejects an ambiguous prepared release",
			manifests: map[string]string{"akt_1.2.3_checksums.txt": manifest, "akt_1.2.4_checksums.txt": manifest},
			installer: installer, subjects: subjects, wantError: "exactly one akt_<version>_checksums.txt",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			dist := filepath.Join(dir, "dist")
			out := filepath.Join(dir, "installer")
			manifests := tc.manifests
			if manifests == nil {
				manifests = map[string]string{"akt_1.2.3_checksums.txt": manifest}
			}
			for name, contents := range manifests {
				commandTestWriteFile(t, dist, name, contents)
			}
			if err := os.MkdirAll(out, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.installer != "" {
				commandTestWriteFile(t, out, "install.sh", tc.installer)
			}
			if tc.subjects != "" {
				commandTestWriteFile(t, out, "attestation-subjects.txt", tc.subjects)
			}

			command := exec.Command("make", "-s", "-C", root, "release-installer-verify",
				"AKT_ROOT="+root, "AKT_DEVCACHE="+filepath.Join(dir, "cache"),
				"GORELEASER_DIST="+dist, "RELEASE_INSTALLER_DIR="+out)
			command.Env = append(os.Environ(), "GOWORK=off")
			output, err := command.CombinedOutput()
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("release-installer-verify: %v\n%s", err, output)
				}
				return
			}
			if err == nil || !strings.Contains(string(output), tc.wantError) {
				t.Fatalf("release-installer-verify error = %v, want %q:\n%s", err, tc.wantError, output)
			}
		})
	}
}

func TestGoreleaserPublishesReleaseInstaller(t *testing.T) {
	t.Parallel()

	var config struct {
		Release struct {
			Draft      bool `yaml:"draft"`
			ExtraFiles []struct {
				Glob string `yaml:"glob"`
			} `yaml:"extra_files"`
		} `yaml:"release"`
	}
	if err := yaml.Unmarshal([]byte(commandTestReadFile(t, filepath.Join("..", "..", ".goreleaser.yaml"))), &config); err != nil {
		t.Fatal(err)
	}
	if config.Release.Draft {
		t.Error("release.draft leaves publication to a later step that an immutable release would reject")
	}
	if len(config.Release.ExtraFiles) != 1 || config.Release.ExtraFiles[0].Glob != "./.cache/installer/install.sh" {
		t.Errorf("release.extra_files = %+v, want only ./.cache/installer/install.sh", config.Release.ExtraFiles)
	}
}

func TestReleaseInstallerDryRunCoversEverySupportedPlatform(t *testing.T) {
	t.Parallel()

	var config struct {
		SupportedPlatforms []struct {
			OS   string `yaml:"os"`
			Arch string `yaml:"arch"`
		} `yaml:"supported_platforms"`
	}
	if err := yaml.Unmarshal([]byte(commandTestReadFile(t, filepath.Join("..", "..", ".config", "binstaller.yml"))), &config); err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, platform := range config.SupportedPlatforms {
		want = append(want, platform.OS+"/"+platform.Arch)
	}
	if len(want) == 0 {
		t.Fatal(".config/binstaller.yml lists no supported platforms")
	}

	slices.Sort(want)

	// Prepared assets and published release in the publish job, plus the
	// manual job's own build.
	workflow := readWorkflowContract(t, "release.yml")
	runs := 0
	for _, job := range []string{"publish", "manual"} {
		for _, step := range workflow.Jobs[job].Steps {
			if !isInstallerDryRun(step) {
				continue
			}
			runs++
			match := regexp.MustCompile(`for platform in ([^;\n]+);`).FindStringSubmatch(step.Run)
			if match == nil {
				t.Errorf("%s installer dry run does not iterate a platform list:\n%s", job, step.Run)
				continue
			}
			got := strings.Fields(match[1])
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("%s installer dry-run platforms = %v, want supported_platforms %v", job, got, want)
			}
		}
	}
	if runs != 3 {
		t.Errorf("release workflow has %d installer dry runs, want 3", runs)
	}
}

func TestRootInstallerIsAttestedFromMain(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("..", "..", ".github", "workflows", "installer-attest.yml")
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	var triggers struct {
		On map[string]struct {
			Branches []string `yaml:"branches"`
			Paths    []string `yaml:"paths"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal(contents, &triggers); err != nil {
		t.Fatal(err)
	}
	// Only main may sign: a branch or pull request run would let unmerged
	// script content carry this workflow's identity.
	if len(triggers.On) != 1 {
		t.Errorf("installer attestation triggers = %v, want push to main only", triggers.On)
	}
	push, ok := triggers.On["push"]
	if !ok || !slices.Equal(push.Branches, []string{"main"}) || !slices.Contains(push.Paths, "install.sh") {
		t.Errorf("installer attestation push trigger = %+v, want main for install.sh changes", push)
	}

	workflow := readWorkflowContract(t, "installer-attest.yml")
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Errorf("workflow-level permissions = %v, want contents: read only", workflow.Permissions)
	}
	if len(workflow.Jobs) != 1 {
		t.Fatalf("installer attestation jobs = %d, want 1", len(workflow.Jobs))
	}
	for _, job := range workflow.Jobs {
		for scope, want := range map[string]string{"contents": "read", "id-token": "write", "attestations": "write"} {
			if got := job.Permissions[scope]; got != want {
				t.Errorf("attest permission %s = %q, want %q", scope, got, want)
			}
		}
		steps := job.Steps
		checkout := workflowStepIndex(steps, func(step workflowStep) bool { return strings.HasPrefix(step.Uses, "actions/checkout@") })
		if checkout < 0 || steps[checkout].With["persist-credentials"] != "false" {
			t.Error("installer attestation checkout retains its credential")
		}
		check := workflowStepIndex(steps, func(step workflowStep) bool { return runsCommand(step, "make installer-check") })
		attest := workflowStepIndex(steps, func(step workflowStep) bool { return strings.HasPrefix(step.Uses, "actions/attest@") })
		verify := workflowStepIndex(steps, func(step workflowStep) bool { return strings.Contains(step.Run, "gh attestation verify") })
		if check < 0 || attest < 0 || verify < 0 || check >= attest || attest >= verify {
			t.Errorf("installer attestation steps (drift check %d, attest %d, verify %d) are missing or out of order", check, attest, verify)
		}
		if attest >= 0 && steps[attest].With["subject-path"] != "install.sh" {
			t.Errorf("attested subject = %q, want install.sh", steps[attest].With["subject-path"])
		}
		if verify >= 0 {
			for _, want := range []string{`--signer-workflow="$REPOSITORY/.github/workflows/installer-attest.yml"`, "--source-ref=refs/heads/main"} {
				if !strings.Contains(steps[verify].Run, want) {
					t.Errorf("root installer verification is missing %q", want)
				}
			}
		}
	}
}

func TestOnlyInstallerWorkflowsWriteAttestations(t *testing.T) {
	t.Parallel()

	// gh attestation verify trusts a signer workflow path; any other
	// workflow able to write attestations widens what users must audit.
	allowed := map[string]bool{"release.yml": true, "installer-attest.yml": true}
	filenames, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, filename := range filenames {
		name := filepath.Base(filename)
		workflow := readWorkflowContract(t, name)
		writes := workflow.Permissions["attestations"] == "write"
		for _, job := range workflow.Jobs {
			writes = writes || job.Permissions["attestations"] == "write"
		}
		if writes && !allowed[name] {
			t.Errorf("%s may write attestations", name)
		}
	}
}
