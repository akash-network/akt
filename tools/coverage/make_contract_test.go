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

type workflowContract struct {
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Permissions map[string]string `yaml:"permissions"`
		Steps       []workflowStep    `yaml:"steps"`
	} `yaml:"jobs"`
}

type workflowStep struct {
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
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
	release := workflowStepIndex(steps, func(step workflowStep) bool { return runsCommand(step, "make release") })
	generate := workflowStepIndex(steps, func(step workflowStep) bool {
		return runsCommand(step, `make release-installer RELEASE_TAG="$EVENT_TAG"`)
	})
	dryRun := workflowStepIndex(steps, func(step workflowStep) bool {
		return strings.Contains(step.Run, "BINSTALLER_OS=") && strings.Contains(step.Run, "Using embedded checksum")
	})
	attest := workflowStepIndex(steps, func(step workflowStep) bool { return strings.HasPrefix(step.Uses, "actions/attest@") })
	upload := workflowStepIndex(steps, func(step workflowStep) bool { return strings.Contains(step.Run, "gh release upload") })
	verify := workflowStepIndex(steps, func(step workflowStep) bool { return strings.Contains(step.Run, "gh attestation verify") })
	order := []struct {
		name  string
		index int
	}{
		{"GoReleaser publication", release},
		{"installer generation for the event tag", generate},
		{"installer dry run against published assets", dryRun},
		{"attestation", attest},
		{"installer upload", upload},
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
	}
	if attest >= 0 {
		if got := steps[attest].With["subject-checksums"]; got != ".cache/installer/attestation-subjects.txt" {
			t.Errorf("attestation subjects = %q, want the generated subject list", got)
		}
	}
	if upload >= 0 && !strings.Contains(steps[upload].Run, ".cache/installer/install.sh") {
		t.Error("publish job does not upload the attested installer")
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

	steps := readWorkflowContract(t, "release.yml").Jobs["publish"].Steps
	index := workflowStepIndex(steps, func(step workflowStep) bool { return strings.Contains(step.Run, "BINSTALLER_OS=") })
	if index < 0 {
		t.Fatal("publish job has no installer dry run")
	}
	match := regexp.MustCompile(`for platform in ([^;\n]+);`).FindStringSubmatch(steps[index].Run)
	if match == nil {
		t.Fatalf("installer dry run does not iterate a platform list:\n%s", steps[index].Run)
	}
	got := strings.Fields(match[1])
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("installer dry-run platforms = %v, want supported_platforms %v", got, want)
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
