// Package workflow provides a declarative workflow engine for orchestrating
// multi-step chain operations. Workflows are defined as YAML files with
// typed steps (tx, query, wait, prompt, provider, output, shell, check).
//
// Built-in workflows (deploy, update, close) ship as embedded defaults.
// Users can override them globally or per-context.
package workflow

import (
	"encoding/json"
	"time"
)

// StepType classifies what a workflow step does.
type StepType string

const (
	StepTx       StepType = "tx"       // Broadcast a transaction
	StepQuery    StepType = "query"    // Execute a chain query
	StepWait     StepType = "wait"     // Poll a query until a condition is met
	StepPrompt   StepType = "prompt"   // Interactive user input
	StepProvider StepType = "provider" // Provider gateway call
	StepOutput   StepType = "output"   // Display formatted output
	StepShell    StepType = "shell"    // Run a shell command
	StepCheck    StepType = "check"    // Assert a condition
)

// ErrorAction defines what happens when a step fails.
type ErrorAction string

const (
	OnErrorAbort    ErrorAction = "abort"    // Stop the workflow
	OnErrorContinue ErrorAction = "continue" // Log the error and proceed
	OnErrorSkip     ErrorAction = "skip"     // Skip this step silently
)

// ParamType defines the type of a workflow parameter.
type ParamType string

const (
	ParamString   ParamType = "string"
	ParamInt      ParamType = "int"
	ParamBool     ParamType = "bool"
	ParamDuration ParamType = "duration"
	ParamFile     ParamType = "file"
)

// ParamDef defines a workflow input parameter.
// CLI flags are auto-generated from these definitions.
type ParamDef struct {
	Type        ParamType `yaml:"type"                  json:"type"`
	Required    bool      `yaml:"required,omitempty"    json:"required,omitempty"`
	Default     string    `yaml:"default,omitempty"     json:"default,omitempty"`
	Description string    `yaml:"description,omitempty" json:"description,omitempty"`
}

// RetryDef defines retry behavior for a step.
type RetryDef struct {
	Max   int    `yaml:"max,omitempty"   json:"max,omitempty"`
	Delay string `yaml:"delay,omitempty" json:"delay,omitempty"` // duration string
}

// DisplayDef defines how data is displayed in a prompt step.
type DisplayDef struct {
	Columns []string `yaml:"columns,omitempty" json:"columns,omitempty"`
}

// StepDef defines a single step in a workflow.
type StepDef struct {
	Name string   `yaml:"name"              json:"name"`
	Type StepType `yaml:"type"              json:"type"`

	// tx step fields
	Msg string `yaml:"msg,omitempty" json:"msg,omitempty"` // message type, e.g. "deployment.MsgCreateDeployment"

	// query / wait step fields
	Query string `yaml:"query,omitempty" json:"query,omitempty"` // query path, e.g. "market.bids"

	// wait step fields
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"` // duration template
	Until   string `yaml:"until,omitempty"   json:"until,omitempty"`   // condition expression template

	// prompt step fields
	Mode    string     `yaml:"mode,omitempty"    json:"mode,omitempty"`    // "interactive", "cheapest", "provider=<addr>"
	Data    string     `yaml:"data,omitempty"    json:"data,omitempty"`    // template resolving to the data to prompt over
	Display DisplayDef `yaml:"display,omitempty" json:"display,omitempty"` // display configuration

	// provider step fields
	Action string `yaml:"action,omitempty" json:"action,omitempty"` // e.g. "send-manifest", "lease-status"

	// shell step fields
	Command string `yaml:"command,omitempty" json:"command,omitempty"` // shell command template

	// check step fields
	Condition string `yaml:"condition,omitempty" json:"condition,omitempty"` // condition expression template

	// output step fields
	Template string `yaml:"template,omitempty" json:"template,omitempty"` // Go template for output

	// Common fields
	Params  map[string]string `yaml:"params,omitempty"   json:"params,omitempty"`   // template-valued parameters
	Output  map[string]string `yaml:"output,omitempty"   json:"output,omitempty"`   // named outputs extracted from result
	OnError ErrorAction       `yaml:"on-error,omitempty" json:"on_error,omitempty"` // abort, continue, skip
	Retry   *RetryDef         `yaml:"retry,omitempty"    json:"retry,omitempty"`    // retry configuration
	OnFail  ErrorAction       `yaml:"on-fail,omitempty"  json:"on_fail,omitempty"`  // for check steps: skip or abort
}

// WorkflowDef is the top-level workflow definition loaded from YAML.
type WorkflowDef struct {
	Name        string              `yaml:"name"                  json:"name"`
	Description string              `yaml:"description,omitempty" json:"description,omitempty"`
	Version     int                 `yaml:"version,omitempty"     json:"version,omitempty"`
	Params      map[string]ParamDef `yaml:"params,omitempty"      json:"params,omitempty"`
	Steps       []StepDef           `yaml:"steps"                 json:"steps"`
}

// StepResult holds the outcome of executing a single step.
type StepResult struct {
	Name      string          `json:"name"`
	Type      StepType        `json:"type"`
	Status    string          `json:"status"` // "success", "failed", "skipped"
	Output    map[string]any  `json:"output,omitempty"`
	RawResult json.RawMessage `json:"raw_result,omitempty"`
	Error     string          `json:"error,omitempty"`
	Duration  time.Duration   `json:"duration"`
	TxHash    string          `json:"tx_hash,omitempty"`
	Height    int64           `json:"height,omitempty"`
}

// RunState holds the accumulated state of a workflow execution.
// It is passed through steps and provides variable resolution context.
type RunState struct {
	WorkflowID string                 `json:"workflow_id"`
	Workflow   string                 `json:"workflow"`
	Account    string                 `json:"account"`
	Params     map[string]any         `json:"params"`
	Steps      map[string]*StepResult `json:"steps"`
	StepOrder  []string               `json:"-"` // ordered step names for iteration
}

// NewRunState creates a fresh run state for a workflow execution.
func NewRunState(workflowID, workflowName, account string, params map[string]any) *RunState {
	return &RunState{
		WorkflowID: workflowID,
		Workflow:   workflowName,
		Account:    account,
		Params:     params,
		Steps:      make(map[string]*StepResult),
	}
}

// SetStepResult records a step result in the run state.
func (rs *RunState) SetStepResult(name string, result *StepResult) {
	rs.Steps[name] = result
	rs.StepOrder = append(rs.StepOrder, name)
}

// StepOutput returns a named output value from a completed step.
// Returns nil if the step or output key does not exist.
func (rs *RunState) StepOutput(stepName, key string) any {
	sr, ok := rs.Steps[stepName]
	if !ok || sr.Output == nil {
		return nil
	}

	return sr.Output[key]
}
