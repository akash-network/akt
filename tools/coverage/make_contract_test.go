package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
