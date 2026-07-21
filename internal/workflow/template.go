package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// templateFuncs returns the functions available to workflow templates in
// addition to the text/template builtins.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// toJson marshals a value to compact JSON. Steps that pass
		// structured data to other steps (e.g. the bid list handed to a
		// prompt step) must serialize it with toJson so the receiving step
		// can parse it back instead of getting Go's fmt representation.
		"toJson": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
	}
}

// ResolveTemplate evaluates a Go template string against the workflow run state.
// Templates use {{ .Params.key }}, {{ .Steps.name.key }}, {{ .Account }}, etc.
func ResolveTemplate(tmpl string, state *RunState) (string, error) {
	if tmpl == "" {
		return "", nil
	}

	// Fast path: no template markers.
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	t, err := template.New("").Funcs(templateFuncs()).Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", tmpl, err)
	}

	// Build the template data from run state.
	data := buildTemplateData(state)

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", tmpl, err)
	}

	return strings.TrimSpace(buf.String()), nil
}

// ResolveParams resolves all template values in a params map.
func ResolveParams(params map[string]string, state *RunState) (map[string]string, error) {
	if len(params) == 0 {
		return params, nil
	}

	resolved := make(map[string]string, len(params))
	for k, v := range params {
		r, err := ResolveTemplate(v, state)
		if err != nil {
			return nil, fmt.Errorf("resolve param %q: %w", k, err)
		}

		resolved[k] = r
	}

	return resolved, nil
}

// ExtractOutputs extracts named outputs from a raw JSON result using template expressions.
// Each output entry maps a name to a template that extracts a value from the result.
func ExtractOutputs(outputDefs map[string]string, raw json.RawMessage, state *RunState) (map[string]any, error) {
	if len(outputDefs) == 0 {
		// If no output definitions, return the raw result as a generic map.
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, nil
		}

		return m, nil
	}

	// Parse the raw result into a generic structure for template access.
	var resultData any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &resultData)
	}

	// Temporarily add .Result to the state for template resolution.
	augmented := &RunState{
		WorkflowID: state.WorkflowID,
		Workflow:   state.Workflow,
		Account:    state.Account,
		Params:     state.Params,
		Steps:      state.Steps,
		StepOrder:  state.StepOrder,
	}

	outputs := make(map[string]any, len(outputDefs))
	for name, tmpl := range outputDefs {
		resolved, err := resolveWithResult(tmpl, augmented, resultData)
		if err != nil {
			outputs[name] = nil
			continue
		}

		outputs[name] = resolved
	}

	return outputs, nil
}

// EvalCondition evaluates a condition expression against the run state.
// Returns true if the condition is met.
// The condition is a Go template that should render to "true" or a non-empty, non-"false" string.
func EvalCondition(condition string, data json.RawMessage, state *RunState) (bool, error) {
	if condition == "" {
		return true, nil
	}

	var resultData any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &resultData)
	}

	rendered, err := resolveWithResult(condition, state, resultData)
	if err != nil {
		return false, err
	}

	s := fmt.Sprintf("%v", rendered)
	s = strings.TrimSpace(s)

	// Empty, "false", "0", or "<no value>" means condition not met.
	return s != "" && s != "false" && s != "0" && s != "<no value>", nil
}

// ParseList attempts to parse a string as a JSON array of objects.
// If the input is not JSON, returns nil.
func ParseList(data string) ([]map[string]any, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, fmt.Errorf("empty data")
	}

	var list []map[string]any
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		return nil, fmt.Errorf("parse list: %w", err)
	}

	return list, nil
}

// buildTemplateData constructs the data map available to templates.
func buildTemplateData(state *RunState) map[string]any {
	data := map[string]any{
		"Account":    state.Account,
		"WorkflowID": state.WorkflowID,
		"Params":     state.Params,
	}

	// Build Steps map with output values accessible as .Steps.<name>.<key>
	steps := make(map[string]any, len(state.Steps))
	for name, sr := range state.Steps {
		stepData := make(map[string]any)
		if sr.Output != nil {
			for k, v := range sr.Output {
				stepData[k] = v
			}
		}

		stepData["_status"] = sr.Status
		stepData["_tx_hash"] = sr.TxHash
		stepData["_height"] = sr.Height
		stepData["_error"] = sr.Error

		steps[name] = stepData
	}

	data["Steps"] = steps

	return data
}

// resolveWithResult evaluates a template with both the run state and a result value.
func resolveWithResult(tmpl string, state *RunState, result any) (any, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	data := buildTemplateData(state)
	data["Result"] = result

	t, err := template.New("").Funcs(templateFuncs()).Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", tmpl, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template %q: %w", tmpl, err)
	}

	return strings.TrimSpace(buf.String()), nil
}
