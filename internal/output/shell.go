package output

import (
	"bytes"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// ShellRunner executes a remote shell with the selected output writers and
// TTY mode.
type ShellRunner func(stdout, stderr io.Writer, tty bool) error

type shellResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// ValidateShellOutput rejects the interactive mode that structured output
// cannot represent. Callers use this before resolving a provider connection;
// RunShellOutput repeats it to keep the shared boundary safe in isolation.
func ValidateShellOutput(cmd *cobra.Command, interactive bool) error {
	format := FormatFromCmd(cmd)
	if format != FormatTable && interactive {
		return fmt.Errorf("--output %s requires an explicit remote command after --; interactive shells use pretty output", format)
	}

	return nil
}

// RunShellOutput preserves the interactive byte stream in pretty mode and
// turns an explicit remote command into one structured result in JSON/YAML
// mode. Structured output cannot represent an interactive terminal session,
// so it is rejected before the runner opens a provider connection.
func RunShellOutput(cmd *cobra.Command, interactive, tty bool, run ShellRunner) error {
	format := FormatFromCmd(cmd)
	if err := ValidateShellOutput(cmd, interactive); err != nil {
		return err
	}
	if format == FormatTable {
		return run(cmd.OutOrStdout(), cmd.ErrOrStderr(), tty)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runErr := run(&stdout, &stderr, false)
	result := shellResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err := FprintJSONSemantics(cmd.OutOrStdout(), format, result); err != nil {
		if runErr != nil {
			return fmt.Errorf("%w; render shell output: %v", runErr, err)
		}
		return fmt.Errorf("render shell output: %w", err)
	}

	return runErr
}
