package steps

import (
	"io"
	"os"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/workflow"
)

func TestPromptExecutorTemplateAndSelectionFailures(t *testing.T) {
	state := workflow.NewRunState("run-1", "deploy", "akash1owner", nil)

	t.Run("mode template", func(t *testing.T) {
		result, err := (&PromptExecutor{}).Execute(t.Context(), workflow.StepDef{
			Name: "prompt", Type: workflow.StepPrompt, Mode: "{{", Data: "[]",
		}, state)
		assertFailedStep(t, result, err, "resolve bid selection")
	})

	t.Run("data template", func(t *testing.T) {
		result, err := (&PromptExecutor{}).Execute(t.Context(), workflow.StepDef{
			Name: "prompt", Type: workflow.StepPrompt, Mode: "cheapest", Data: "{{",
		}, state)
		assertFailedStep(t, result, err, "resolve prompt data")
	})

	t.Run("invalid list", func(t *testing.T) {
		result, err := (&PromptExecutor{}).Execute(t.Context(), workflow.StepDef{
			Name: "prompt", Type: workflow.StepPrompt, Mode: "cheapest", Data: "not-json",
		}, state)
		assertFailedStep(t, result, err, "no items to select from")
	})

	t.Run("valid provider absent", func(t *testing.T) {
		result, err := (&PromptExecutor{}).Execute(t.Context(), workflow.StepDef{
			Name: "prompt",
			Type: workflow.StepPrompt,
			Mode: "provider=akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
			Data: `[{"id":{"provider":"akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl"}}]`,
		}, state)
		assertFailedStep(t, result, err, "no item selected")
	})
}

func TestInteractiveSelectReadsOneBoundedChoice(t *testing.T) {
	items := []map[string]any{
		{"provider": "akash1first", "price": "20uakt"},
		{"provider": "akash1second", "price": "10uakt"},
	}

	t.Run("empty list", func(t *testing.T) {
		selected, err := interactiveSelect(nil, []string{"provider"})
		if err == nil || selected != nil || !strings.Contains(err.Error(), "no items") {
			t.Fatalf("selected = %#v, error = %v", selected, err)
		}
	})

	t.Run("valid selection", func(t *testing.T) {
		selected, output, err := runInteractiveSelection(t, "2\n", items, []string{"provider", "price"})
		if err != nil {
			t.Fatalf("interactiveSelect: %v", err)
		}
		if selected["provider"] != "akash1second" {
			t.Fatalf("selected = %#v", selected)
		}
		for _, marker := range []string{"akash1first", "akash1second", "Select [1-2]"} {
			if !strings.Contains(output, marker) {
				t.Errorf("prompt output missing %q: %q", marker, output)
			}
		}
	})

	for _, input := range []string{"0\n", "3\n", "word\n", "\n"} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			selected, _, err := runInteractiveSelection(t, input, items, []string{"provider"})
			if err == nil || selected != nil || !strings.Contains(err.Error(), "invalid selection") {
				t.Fatalf("input %q selected = %#v, error = %v", input, selected, err)
			}
		})
	}
}

func TestPromptExecutorInteractiveModeExposesIdentity(t *testing.T) {
	state := workflow.NewRunState("run-1", "deploy", "akash1owner", nil)
	step := workflow.StepDef{
		Name: "prompt",
		Type: workflow.StepPrompt,
		Mode: "interactive",
		Data: `[
			{"bid":{"id":{"dseq":"42","gseq":1,"oseq":1,"provider":"akash1first"},"price":{"amount":"20","denom":"uakt"}}},
			{"bid":{"id":{"dseq":"42","gseq":1,"oseq":2,"provider":"akash1second"},"price":{"amount":"10","denom":"uakt"}}}
		]`,
		Display: workflow.DisplayDef{Columns: []string{"provider", "price"}},
	}

	var result *workflow.StepResult
	var executeErr error
	_, ioErr := runInteractiveSelectionFunc(t, "2\n", func() {
		result, executeErr = (&PromptExecutor{}).Execute(t.Context(), step, state)
	})
	if ioErr != nil {
		t.Fatalf("prompt I/O: %v", ioErr)
	}
	if executeErr != nil {
		t.Fatalf("Execute: %v", executeErr)
	}
	if result.Status != "success" || result.Output["provider"] != "akash1second" || result.Output["oseq"] != "2" || result.Output["price"] != "10uakt" {
		t.Fatalf("result = %+v", result)
	}
}

func TestPromptExecutorInteractiveRejectionIsFailedStep(t *testing.T) {
	state := workflow.NewRunState("run-1", "deploy", "akash1owner", nil)
	step := workflow.StepDef{
		Name:    "prompt",
		Type:    workflow.StepPrompt,
		Mode:    "interactive",
		Data:    `[{"provider":"akash1first"}]`,
		Display: workflow.DisplayDef{Columns: []string{"provider"}},
	}

	var result *workflow.StepResult
	var executeErr error
	_, ioErr := runInteractiveSelectionFunc(t, "0\n", func() {
		result, executeErr = (&PromptExecutor{}).Execute(t.Context(), step, state)
	})
	if ioErr != nil {
		t.Fatalf("prompt I/O: %v", ioErr)
	}
	if executeErr == nil || !strings.Contains(executeErr.Error(), "invalid selection") || result.Status != "failed" {
		t.Fatalf("result = %+v, error = %v", result, executeErr)
	}
}

func TestBidPriceRejectsMalformedAmountsAndCheapestRejectsEmptyInput(t *testing.T) {
	for name, item := range map[string]map[string]any{
		"malformed string": {"price": map[string]any{"amount": "not-a-number"}},
		"unexpected type":  {"price": map[string]any{"amount": true}},
	} {
		t.Run(name, func(t *testing.T) {
			if price, ok := bidPrice(item); ok {
				t.Fatalf("bidPrice(%#v) = %v, true", item, price)
			}
		})
	}

	if selected := selectCheapest(nil); selected != nil {
		t.Fatalf("selectCheapest(nil) = %#v", selected)
	}
}

func runInteractiveSelection(
	t *testing.T,
	input string,
	items []map[string]any,
	columns []string,
) (map[string]any, string, error) {
	t.Helper()
	var selected map[string]any
	var selectionErr error
	output, ioErr := runInteractiveSelectionFunc(t, input, func() {
		selected, selectionErr = interactiveSelect(items, columns)
	})
	if ioErr != nil {
		t.Fatalf("capture interactive selection: %v", ioErr)
	}
	return selected, output, selectionErr
}

func runInteractiveSelectionFunc(t *testing.T, input string, fn func()) (string, error) {
	t.Helper()

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return "", err
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		stdinReader.Close()
		stdinWriter.Close()
		return "", err
	}

	oldStdin, oldStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = stdinReader, stdoutWriter
	defer func() {
		os.Stdin, os.Stdout = oldStdin, oldStdout
	}()

	if _, err := io.WriteString(stdinWriter, input); err != nil {
		return "", err
	}
	if err := stdinWriter.Close(); err != nil {
		return "", err
	}

	fn()
	if err := stdoutWriter.Close(); err != nil {
		return "", err
	}
	output, err := io.ReadAll(stdoutReader)
	stdinReader.Close()
	stdoutReader.Close()
	return string(output), err
}
