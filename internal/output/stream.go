package output

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// PrintStreamRecord writes one record in the selected stream format. JSON is
// one compact object per line, YAML is one document per record, and the
// caller supplies the human-readable pretty line.
func PrintStreamRecord(cmd *cobra.Command, structured any, pretty string) error {
	switch FormatFromCmd(cmd) {
	case FormatJSON:
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(structured); err != nil {
			return fmt.Errorf("marshal stream output: %w", err)
		}
		return nil
	case FormatYAML:
		if err := FprintJSONSemantics(cmd.OutOrStdout(), FormatYAML, structured); err != nil {
			return fmt.Errorf("marshal stream output: %w", err)
		}
		return nil
	default:
		_, err := fmt.Fprintln(cmd.OutOrStdout(), pretty)
		return err
	}
}
