package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConsoleGateChangedFiles(t *testing.T) {
	script, err := filepath.Abs("../../script/ci-console-required.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, want  string
		paths       []string
		rename      bool
		delete      bool
		advanceBase bool
	}{
		{name: "documentation", paths: []string{"README.md", "SPEC.md", "docs/guide with spaces.md"}, want: "false"},
		{name: "changelog documentation", paths: []string{".changelog/101.fixed.md", ".changelog/README.md"}, want: "false"},
		{name: "vault PR", paths: []string{"AICHANGELOG.md", "DESIGN.md", "SPEC.md", "internal/output/pretty/bme.go", "internal/output/pretty/bme_layout_test.go", "internal/output/pretty/testdata/TestRenderBMEVaultState/WithBalances.golden"}, want: "false"},
		{name: "vault PR with fragment", paths: []string{".changelog/101.fixed.md", "internal/output/pretty/bme.go"}, want: "false"},
		{name: "runtime change with fragment", paths: []string{".changelog/104.fixed.md", "internal/console/client.go"}, want: "true"},
		{name: "non-Markdown changelog file", paths: []string{".changelog/helper.go"}, want: "true"},
		{name: "shared startup", paths: []string{"internal/cli/root.go"}, want: "true"},
		{name: "shared formatting", paths: []string{"internal/output/pretty/helpers.go"}, want: "true"},
		{name: "console", paths: []string{"internal/console/client.go"}, want: "true"},
		{name: "workflow", paths: []string{"internal/workflow/builtin/deploy.yaml"}, want: "true"},
		{name: "transport", paths: []string{"internal/transport/console.go"}, want: "true"},
		{name: "provider", paths: []string{"internal/provider/client.go"}, want: "true"},
		{name: "config", paths: []string{"internal/context/config.go"}, want: "true"},
		{name: "SDL", paths: []string{"internal/cli/sdl/templates.go"}, want: "true"},
		{name: "dependency", paths: []string{"go.mod"}, want: "true"},
		{name: "build", paths: []string{"Makefile"}, want: "true"},
		{name: "workflow definition", paths: []string{".github/workflows/ci.yml"}, want: "true"},
		{name: "test harness", paths: []string{"e2e/console_live_test.go"}, want: "true"},
		{name: "unknown", paths: []string{"new-package/file.go"}, want: "true"},
		{name: "runtime markdown", paths: []string{"internal/workflow/template.md"}, want: "true"},
		{name: "mixed", paths: []string{"README.md", "internal/console/client.go"}, want: "true"},
		{name: "newline filename", paths: []string{"README.md\ninternal/console/client.go"}, want: "true"},
		{name: "rename runtime into docs", rename: true, want: "true"},
		{name: "delete runtime file", delete: true, want: "true"},
		{name: "base advanced independently", paths: []string{"README.md"}, advanceBase: true, want: "false"},
		{name: "empty diff", want: "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			runTestGit(t, repo, "init", "--quiet")
			runTestGit(t, repo, "config", "user.name", "CI fixture")
			runTestGit(t, repo, "config", "user.email", "ci-fixture@example.invalid")
			runTestGit(t, repo, "config", "commit.gpgsign", "false")
			commandTestWriteFile(t, repo, "internal/console/client.go", "original\n")
			runTestGit(t, repo, "add", ".")
			runTestGit(t, repo, "commit", "--quiet", "-m", "base")
			base := strings.TrimSpace(runTestGit(t, repo, "rev-parse", "HEAD"))
			for _, path := range tc.paths {
				commandTestWriteFile(t, repo, path, "changed\n")
			}
			if tc.rename {
				commandTestWriteFile(t, repo, "docs/renamed.md", "original\n")
			}
			if tc.rename || tc.delete {
				if err := os.Remove(filepath.Join(repo, "internal/console/client.go")); err != nil {
					t.Fatal(err)
				}
			}
			runTestGit(t, repo, "add", ".")
			runTestGit(t, repo, "commit", "--quiet", "--allow-empty", "-m", "head")
			head := strings.TrimSpace(runTestGit(t, repo, "rev-parse", "HEAD"))
			if tc.advanceBase {
				runTestGit(t, repo, "checkout", "--quiet", "--detach", base)
				commandTestWriteFile(t, repo, "internal/cli/root.go", "independent base change\n")
				runTestGit(t, repo, "add", ".")
				runTestGit(t, repo, "commit", "--quiet", "-m", "advance base")
				base = strings.TrimSpace(runTestGit(t, repo, "rev-parse", "HEAD"))
			}
			cmd := exec.Command("bash", script, base, head)
			cmd.Dir = repo
			out, err := cmd.CombinedOutput()
			if err != nil || strings.TrimSpace(string(out)) != tc.want {
				t.Fatalf("selection = %q, %v; want %s", out, err, tc.want)
			}
		})
	}
	for _, args := range [][]string{nil, {"main", "HEAD"}, {strings.Repeat("0", 40), strings.Repeat("1", 40)}} {
		cmd := exec.Command("bash", append([]string{script}, args...)...)
		if out, err := cmd.Output(); err == nil || strings.TrimSpace(string(out)) == "false" {
			t.Fatalf("invalid revisions authorized a skip: %q, %v", out, err)
		}
	}
}

func TestConsoleGateWorkflow(t *testing.T) {
	contents, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			If    string
			Needs yaml.Node
			Steps []struct {
				Run string
				Env map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(contents, &workflow); err != nil {
		t.Fatal(err)
	}
	if _, ok := workflow.Jobs["console-changes"]; !ok {
		t.Fatal("missing secretless change-selection job")
	}
	if _, ok := workflow.Jobs["changelog"]; !ok {
		t.Fatal("missing changelog validation job")
	}
	for _, name := range []string{"e2e-console-sandbox", "coverage-live-report", "required-ci"} {
		job := workflow.Jobs[name]
		var needs []string
		if err := job.Needs.Decode(&needs); err != nil {
			t.Fatalf("decode %s dependencies: %v", name, err)
		}
		if !slices.Contains(needs, "console-changes") {
			t.Errorf("%s does not depend on the selection job", name)
		}
		if name == "required-ci" && !slices.Contains(needs, "changelog") {
			t.Error("required-ci does not depend on changelog validation")
		}
		if name != "required-ci" && !strings.Contains(job.If, "needs.console-changes.outputs.required == 'true'") {
			t.Errorf("%s ignores changed-file selection", name)
		}
		if name != "required-ci" {
			for _, boundary := range []string{"github.event_name == 'pull_request'", "github.event.pull_request.base.ref == 'main'", "github.event.pull_request.head.repo.full_name == github.repository", "github.event.pull_request.user.login != 'dependabot[bot]'"} {
				if !strings.Contains(job.If, boundary) {
					t.Errorf("%s lost trust boundary %q", name, boundary)
				}
			}
		}
	}
	gate := workflow.Jobs["required-ci"]
	if !strings.Contains(gate.If, "always()") {
		t.Fatal("required-ci must report failures even when a dependency fails")
	}
	if len(gate.Steps) != 1 {
		t.Fatalf("expected one final gate step, got %d", len(gate.Steps))
	}
	if !strings.Contains(gate.Steps[0].Env["CONSOLE_REQUIRED"], "needs.console-changes.outputs.required == 'true'") {
		t.Error("final gate does not include changed-file selection in Console eligibility")
	}
	for _, tc := range []struct {
		name string
		env  []string
		pass bool
	}{
		{name: "all required jobs pass", pass: true},
		{name: "intentional skip", env: []string{"CONSOLE_CHANGED=false", "CONSOLE_REQUIRED=false", "CONSOLE_RESULT=skipped", "LIVE_REPORT_RESULT=skipped"}, pass: true},
		{name: "failed changelog with sandbox skipped", env: []string{"CHANGELOG_RESULT=failure", "CONSOLE_CHANGED=false", "CONSOLE_REQUIRED=false", "CONSOLE_RESULT=skipped", "LIVE_REPORT_RESULT=skipped"}},
		{name: "skipped changelog", env: []string{"CHANGELOG_RESULT=skipped"}},
		{name: "failed selection", env: []string{"CHANGES_RESULT=failure"}},
		{name: "skipped selection", env: []string{"CHANGES_RESULT=skipped"}},
		{name: "missing decision", env: []string{"CONSOLE_CHANGED="}},
		{name: "invalid decision", env: []string{"CONSOLE_CHANGED=maybe"}},
		{name: "unexpected Console skip", env: []string{"CONSOLE_RESULT=skipped"}},
		{name: "Console failure", env: []string{"CONSOLE_RESULT=failure"}},
		{name: "missing live report", env: []string{"LIVE_REPORT_RESULT=skipped"}},
		{name: "failed live report", env: []string{"LIVE_REPORT_RESULT=failure"}},
		{name: "coverage still required", env: []string{"COVERAGE_RESULT=failure"}},
		{name: "main remains secretless", env: []string{"EVENT_NAME=push", "CODECOV_MAIN_RESULT=success", "CONSOLE_CHANGED=false", "CONSOLE_REQUIRED=false", "CONSOLE_RESULT=skipped", "LIVE_REPORT_RESULT=skipped"}, pass: true},
		{name: "ineligible PR remains secretless", env: []string{"CONSOLE_REQUIRED=false", "CONSOLE_RESULT=skipped", "LIVE_REPORT_RESULT=skipped"}, pass: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", gate.Steps[0].Run)
			cmd.Env = append(os.Environ(), "CHANGELOG_RESULT=success", "LINT_RESULT=success", "BUILD_RESULT=success", "RACE_RESULT=success", "COVERAGE_RESULT=success", "CHANGES_RESULT=success", "CONSOLE_CHANGED=true", "CONSOLE_REQUIRED=true", "CONSOLE_RESULT=success", "LIVE_REPORT_RESULT=success", "CODECOV_MAIN_RESULT=skipped", "EVENT_NAME=pull_request")
			cmd.Env = append(cmd.Env, tc.env...)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.pass {
				t.Fatalf("final gate error = %v, want success %t: %s", err, tc.pass, out)
			}
		})
	}
}
