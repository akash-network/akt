package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/cli"
)

type skillExample struct {
	file    string
	section string
	line    int
	args    []string
	output  string
}

// Read the shipped examples rather than maintaining a second command list.
// Recipes use one unquoted argv per line, with optional stdout redirection.
// Reject additional shell syntax instead of evaluating it in the test runner.
func readSkillExamples(t *testing.T) []skillExample {
	t.Helper()
	root := filepath.Dir(filepath.Dir(filepath.Dir(aktBinary(t))))
	skill := filepath.Join(root, ".agents", "skills", "akt-cli")
	files, err := filepath.Glob(filepath.Join(skill, "references", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	files = append([]string{filepath.Join(skill, "SKILL.md")}, files...)
	var examples []skillExample
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		section, bash := "", false
		for i, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "## ") {
				section = strings.TrimPrefix(line, "## ")
			}
			if strings.HasPrefix(line, "```") {
				bash = line == "```bash"
				continue
			}
			line = strings.TrimSpace(line)
			if !bash || line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.HasPrefix(line, "akt ") || strings.ContainsAny(line, "\"'`$\\;|&<") {
				t.Fatalf("%s:%d: expected a single literal akt command, got %q", file, i+1, line)
			}
			example := skillExample{file: filepath.Base(file), section: section, line: i + 1}
			command, output, redirected := strings.Cut(line, " > ")
			if redirected {
				if output == "" || strings.ContainsAny(output, " >\t") || filepath.Base(output) != output {
					t.Fatalf("%s:%d: expected redirection to a local filename", file, i+1)
				}
				example.output = output
			}
			example.args = strings.Fields(command)[1:]
			examples = append(examples, example)
		}
	}
	return examples
}

func isolateSkillExamples(t *testing.T) string {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "AKT_") {
			t.Setenv(key, "")
		}
	}
	home := t.TempDir()
	t.Setenv("AKT_HOME", home)
	t.Chdir(t.TempDir())
	initHome(t, home)
	return home
}

func TestAgentSkillExampleSyntax(t *testing.T) {
	examples := readSkillExamples(t)
	home := isolateSkillExamples(t)
	if len(examples) == 0 {
		t.Fatal("skill has no command examples")
	}
	for _, example := range examples {
		t.Run(fmt.Sprintf("%s:%d", example.file, example.line), func(t *testing.T) {
			root := cli.NewRootCmd(cli.BuildInfo{Version: "skill-test"})
			cmd, args, err := root.Find(example.args)
			if err != nil {
				t.Fatal(err)
			}
			cmd.InitDefaultHelpFlag()
			if err := cmd.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			helpRequested, _ := cmd.Flags().GetBool("help")
			if !helpRequested {
				if err := cmd.ValidateArgs(cmd.Flags().Args()); err != nil {
					t.Fatal(err)
				}
				if err := cmd.ValidateFlagGroups(); err != nil {
					t.Fatal(err)
				}
			}
			// Put help before any remote-shell "--" separator. No startup
			// hook or action runs, including for mutating examples.
			separator := slices.Index(example.args, "--")
			if separator < 0 {
				separator = len(example.args)
			}
			help := example
			help.output = ""
			help.args = append(append(slices.Clone(example.args[:separator]), "--help"), example.args[separator:]...)
			stdout := runSkillExample(t, home, help)
			if !strings.Contains(stdout, "Usage:") {
				t.Fatalf("example did not resolve to command help:\n%s", stdout)
			}
		})
	}
}

func TestAgentSkillOfflineRecipes(t *testing.T) {
	examples := readSkillExamples(t)
	home := isolateSkillExamples(t)
	runSection := func(file, section string) []string {
		t.Helper()
		var results []string
		for _, example := range examples {
			if example.file == file && example.section == section {
				results = append(results, runSkillExample(t, home, example))
			}
		}
		if len(results) == 0 {
			t.Fatalf("missing recipe %s: %s", file, section)
		}
		return results
	}

	var version struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	discovery := runSection("SKILL.md", "Discover the installed CLI")
	if err := json.Unmarshal([]byte(discovery[0]), &version); err != nil || version.Version == "" || version.Commit == "" {
		t.Fatalf("discovery did not report build identity: %s (%v)", discovery[0], err)
	}

	var validation struct {
		Valid    bool `json:"valid"`
		Services int  `json:"services"`
		Groups   int  `json:"groups"`
	}
	sdl := runSection("deployments.md", "Author and validate locally")
	if err := json.Unmarshal([]byte(sdl[1]), &validation); err != nil || !validation.Valid || validation.Services != 1 || validation.Groups != 1 {
		t.Fatalf("generated SDL was not a valid single-service deployment: %s (%v)", sdl[1], err)
	}

	runSection("setup.md", "Create a Console context")
	var selected struct {
		Name         string `json:"name"`
		AuthMethod   string `json:"auth_method"`
		Capabilities struct {
			ChainQuery bool `json:"chain_query"`
			Console    bool `json:"console"`
		} `json:"capabilities"`
	}
	inspection := runSection("setup.md", "Inspect a context")
	if err := json.Unmarshal([]byte(inspection[0]), &selected); err != nil || selected.Name != "example" || selected.AuthMethod != "console-api" || selected.Capabilities.ChainQuery || selected.Capabilities.Console {
		t.Fatalf("context inspection did not report the unauthenticated Console context: %s (%v)", inspection[0], err)
	}

	// If a preview ever attempts a mutation, it can reach only this local
	// rejecting server. Never give documentation tests a real credential.
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	mustRunAkt(t, home, "context", "edit", "example", "--console-api-url", server.URL)
	t.Setenv("AKT_CONSOLE_API_KEY", "skill-test-key")
	logBefore := mustRunAkt(t, home, "context", "log", "--context", "example", "-o", "json")
	preview := runSection("deployments.md", "Preview a deployment")
	steps := strings.Split(strings.TrimSpace(preview[0]), "\n")
	if len(steps) < 2 {
		t.Fatalf("expected a multi-step JSONL deployment plan: %s", preview[0])
	}
	for _, line := range steps {
		var step struct {
			Workflow string            `json:"workflow"`
			ID       string            `json:"id"`
			Result   string            `json:"result"`
			Txs      []json.RawMessage `json:"txs"`
		}
		if err := json.Unmarshal([]byte(line), &step); err != nil || step.Workflow != "deploy" || step.ID == "" || step.Result != "planned" || len(step.Txs) != 0 {
			t.Fatalf("preview emitted an invalid planned step: %s (%v)", line, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("preview made %d API requests", requests.Load())
	}
	logAfter := mustRunAkt(t, home, "context", "log", "--context", "example", "-o", "json")
	if logBefore != logAfter {
		t.Fatal("preview appended a mutation to the action log")
	}
}

func runSkillExample(t *testing.T, home string, example skillExample) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := append([]string{"--home", home}, example.args...)
	cmd := exec.CommandContext(ctx, aktBinary(t), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s:%d: akt %s: %v\nstdout: %s\nstderr: %s", example.file, example.line, strings.Join(example.args, " "), err, &stdout, &stderr)
	}
	if example.output != "" {
		if err := os.WriteFile(example.output, stdout.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return stdout.String()
}
