package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssemblyPreservesHistoryAndRetriesCleanup(t *testing.T) {
	root := t.TempDir()
	archive := "# AI Changelog\n\n## Unreleased\n\n### Fixed\n\n- Existing note.\n\n## v0.1.0\n\n- Historical note.\n"
	writeFile(t, root, "AICHANGELOG.md", archive)
	writeFile(t, root, ".changelog/README.md", "Instructions stay here.\n")
	writeFile(t, root, ".changelog/z.fixed.md", "- Last fix.\n")
	writeFile(t, root, ".changelog/a.fixed.md", "- First fix.\n  With details.\n")
	writeFile(t, root, ".changelog/feature.added.md", "- New feature.\n")
	if err := run(root, []string{"check"}); err != nil {
		t.Fatal(err)
	}
	if err := run(root, []string{"release-check"}); err == nil {
		t.Fatal("release accepted pending fragments")
	}
	if err := run(root, []string{"assemble"}); err != nil {
		t.Fatal(err)
	}
	want := "# AI Changelog\n\n## Unreleased\n\n" +
		"### Added\n\n<!-- changelog: feature.added.md -->\n- New feature.\n<!-- /changelog -->\n\n" +
		"### Fixed\n\n<!-- changelog: a.fixed.md -->\n- First fix.\n  With details.\n<!-- /changelog -->\n\n" +
		"<!-- changelog: z.fixed.md -->\n- Last fix.\n<!-- /changelog -->\n\n" +
		"### Fixed\n\n- Existing note.\n\n## v0.1.0\n\n- Historical note.\n"
	if got := readFile(t, root, "AICHANGELOG.md"); got != want {
		t.Fatalf("unexpected archive:\n%s", got)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".changelog"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "README.md" {
		t.Fatalf("consumed fragments were not removed: %v, %v", entries, err)
	}
	// Simulate interruption after the archive was written but before all
	// fragments were removed. The next invocation must only finish cleanup.
	writeFile(t, root, ".changelog/a.fixed.md", "- First fix.\n  With details.\n")
	for range 2 {
		if err := run(root, []string{"assemble"}); err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, root, "AICHANGELOG.md"); got != want {
			t.Fatal("retry changed the assembled archive")
		}
	}
	if err := run(root, []string{"release-check"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".changelog/a.fixed.md", "- Different fix.\n")
	if err := run(root, []string{"assemble"}); err == nil {
		t.Fatal("reused fragment name accepted different contents")
	}
	if got := readFile(t, root, "AICHANGELOG.md"); got != want {
		t.Fatal("failed assembly modified history")
	}
	if _, err := os.Stat(filepath.Join(root, ".changelog/a.fixed.md")); err != nil {
		t.Fatal("failed assembly removed an unconsumed fragment")
	}
}

func TestInvalidFragments(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"empty.fixed.md", "\n"},
		{"missing-category.md", "- Note.\n"},
		{"typo.fix.md", "- Note.\n"},
		{"note.fixed.md", "Plain paragraph.\n"},
		{"heading.fixed.md", "- Note.\n\n## Unreleased\n"},
		{"marker.fixed.md", "- Note.\n<!-- changelog: forged.fixed.md -->\n"},
		{"conflict.fixed.md", "- Note.\n<<<<<<< HEAD\n"},
		{"subdir/note.fixed.md", "- Note.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, ".changelog/"+tc.name, tc.content)
			if err := run(root, []string{"check"}); err == nil {
				t.Fatal("invalid fragment accepted")
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "outside.md", "- External contents.\n")
		if err := os.Mkdir(filepath.Join(root, ".changelog"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../outside.md", filepath.Join(root, ".changelog/link.fixed.md")); err != nil {
			t.Fatal(err)
		}
		if err := run(root, []string{"check"}); err == nil {
			t.Fatal("symlink fragment accepted")
		}
	})
}

func TestInvalidInvocationAndArchive(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"assemble", "unexpected"}, {"check", "base", "extra"}} {
		if err := run(t.TempDir(), args); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("args %v: expected usage error, got %v", args, err)
		}
	}
	for _, archive := range []string{"# Missing Unreleased\n", "## Unreleased\n\n## Unreleased\n"} {
		root := t.TempDir()
		writeFile(t, root, "AICHANGELOG.md", archive)
		writeFile(t, root, ".changelog/note.fixed.md", "- Keep this note.\n")
		if err := run(root, []string{"assemble"}); err == nil {
			t.Fatal("malformed archive accepted")
		}
		if got := readFile(t, root, "AICHANGELOG.md"); got != archive {
			t.Fatal("malformed archive was modified")
		}
		if _, err := os.Stat(filepath.Join(root, ".changelog/note.fixed.md")); err != nil {
			t.Fatal("fragment was removed without being archived")
		}
	}
}

func TestPRPolicy(t *testing.T) {
	for _, scenario := range []string{"ordinary", "missing", "edit-archive", "edit-fragment", "delete-fragment", "release", "release-with-code", "incomplete-release", "invalid-base"} {
		t.Run(scenario, func(t *testing.T) {
			root, base := newRepository(t)
			switch scenario {
			case "ordinary", "edit-archive", "edit-fragment", "delete-fragment":
				writeFile(t, root, ".changelog/pr-two.added.md", "- Another feature.\n")
			}
			switch scenario {
			case "edit-archive":
				writeFile(t, root, "AICHANGELOG.md", "# AI Changelog\n\n## Unreleased\n\n- Manual edit.\n")
			case "edit-fragment":
				writeFile(t, root, ".changelog/pr-one.fixed.md", "- Changed someone else's note.\n")
			case "delete-fragment":
				if err := os.Remove(filepath.Join(root, ".changelog/pr-one.fixed.md")); err != nil {
					t.Fatal(err)
				}
			case "release", "release-with-code", "incomplete-release":
				if err := run(root, []string{"assemble"}); err != nil {
					t.Fatal(err)
				}
				if scenario == "release-with-code" {
					writeFile(t, root, "example.txt", "Unrelated change.\n")
				}
				if scenario == "incomplete-release" {
					writeFile(t, root, ".changelog/pr-one.fixed.md", "- First fix.\n")
				}
			case "invalid-base":
				base = "nonexistent-base"
			}
			git(t, root, "add", ".")
			err := run(root, []string{"check", base})
			wantOK := scenario == "ordinary" || scenario == "release"
			if (err == nil) != wantOK {
				t.Fatalf("check returned %v, want success=%v", err, wantOK)
			}
		})
	}
}

func TestSeparatePRsMergeAndAssemble(t *testing.T) {
	root, base := newRepository(t)
	git(t, root, "checkout", "-b", "first", base)
	writeFile(t, root, ".changelog/first.added.md", "- First parallel feature.\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "first")
	git(t, root, "checkout", "-b", "second", base)
	writeFile(t, root, ".changelog/second.added.md", "- Second parallel feature.\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "second")
	git(t, root, "merge", "--no-edit", "first")
	if err := run(root, []string{"check", base}); err != nil {
		t.Fatal(err)
	}
	if err := run(root, []string{"assemble"}); err != nil {
		t.Fatal(err)
	}
	archive := readFile(t, root, "AICHANGELOG.md")
	for _, note := range []string{"First parallel feature.", "Second parallel feature.", "First fix.", "Historical note."} {
		if strings.Count(archive, note) != 1 {
			t.Fatalf("expected exactly one copy of %q in archive", note)
		}
	}
}

func TestPRCannotReuseAssembledName(t *testing.T) {
	root, _ := newRepository(t)
	if err := run(root, []string{"assemble"}); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "assembled")
	base := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	writeFile(t, root, ".changelog/pr-one.fixed.md", "- First fix.\n")
	if err := run(root, []string{"check", base}); err == nil {
		t.Fatal("old fragment accepted as a new change record")
	}
}

func newRepository(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init")
	writeFile(t, root, "AICHANGELOG.md", "# AI Changelog\n\n## Unreleased\n\n- Historical note.\n")
	writeFile(t, root, ".changelog/README.md", "Instructions.\n")
	writeFile(t, root, ".changelog/pr-one.fixed.md", "- First fix.\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "base")
	return root, strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Changelog Test", "-c", "user.email=changelog@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
