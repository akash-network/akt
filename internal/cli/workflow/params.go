package workflow

import (
	"fmt"
	"os"
	"strings"
	"time"

	gosdl "pkg.akt.dev/go/sdl"

	"pkg.akt.dev/akt/internal/transport"
	wf "pkg.akt.dev/akt/internal/workflow"
)

func validateWorkflowParams(def *wf.WorkflowDef, params map[string]any) error {
	for _, name := range sortedParamNames(def.Params) {
		param := def.Params[name]
		value, present := params[name]
		if param.Required && (!present || workflowParamEmpty(value)) {
			return fmt.Errorf("workflow parameter %q is required", name)
		}
		if !present || workflowParamEmpty(value) {
			continue
		}

		var err error
		switch param.Type {
		case wf.ParamString:
			if _, ok := value.(string); !ok {
				err = fmt.Errorf("must be a string")
			}
		case wf.ParamInt:
			v, ok := value.(int)
			if !ok {
				err = fmt.Errorf("must be a base-10 integer")
			} else if isSequenceParam(name) && v <= 0 {
				err = fmt.Errorf("must be greater than zero")
			}
		case wf.ParamBool:
			if _, ok := value.(bool); !ok {
				err = fmt.Errorf("must be a boolean")
			}
		case wf.ParamDuration:
			text, ok := value.(string)
			if !ok {
				err = fmt.Errorf("must be a duration")
				break
			}
			var duration time.Duration
			duration, err = time.ParseDuration(text)
			if err != nil {
				err = fmt.Errorf("invalid duration %q: %w", text, err)
			} else if duration <= 0 {
				err = fmt.Errorf("duration must be greater than zero")
			}
		case wf.ParamFile:
			err = validateReadableFile(value)
		case wf.ParamSDL:
			err = validateSDLFile(value)
		case wf.ParamDeposit:
			text, ok := value.(string)
			if !ok {
				err = fmt.Errorf("must be a deposit string")
				break
			}
			_, err = transport.ParseDeposit(text)
		case wf.ParamBidSelection:
			text, ok := value.(string)
			if !ok {
				err = fmt.Errorf("must be a bid selection string")
				break
			}
			err = wf.ValidateBidSelection(text)
		default:
			err = fmt.Errorf("uses unsupported type %q", param.Type)
		}

		if err != nil {
			return fmt.Errorf("invalid workflow parameter %q: %w", name, err)
		}
	}

	return nil
}

func workflowParamEmpty(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}

func isSequenceParam(name string) bool {
	switch name {
	case "dseq", "gseq", "oseq":
		return true
	default:
		return false
	}
}

func validateReadableFile(value any) error {
	path, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a file path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("file %q is not a regular file", path)
	}
	file, err := os.Open(path) //nolint:gosec -- user-selected workflow input
	if err != nil {
		return fmt.Errorf("read file %q: %w", path, err)
	}

	return file.Close()
}

func validateSDLFile(value any) error {
	if err := validateReadableFile(value); err != nil {
		return err
	}
	path := value.(string)
	doc, err := gosdl.ReadFile(path)
	if err != nil {
		return fmt.Errorf("invalid SDL %q: %w", path, err)
	}
	if _, err := doc.Manifest(); err != nil {
		return fmt.Errorf("invalid SDL %q manifest: %w", path, err)
	}
	if _, err := doc.DeploymentGroups(); err != nil {
		return fmt.Errorf("invalid SDL %q deployment groups: %w", path, err)
	}
	if _, err := doc.Version(); err != nil {
		return fmt.Errorf("invalid SDL %q version: %w", path, err)
	}

	return nil
}
