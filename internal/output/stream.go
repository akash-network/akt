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
	checked := NewCheckedWriter(cmd.OutOrStdout())

	switch FormatFromCmd(cmd) {
	case FormatJSON:
		err := json.NewEncoder(checked).Encode(structured)
		if err != nil {
			err = fmt.Errorf("marshal stream output: %w", err)
		}
		return checked.Complete(err)
	case FormatYAML:
		if err := FprintJSONSemantics(checked, FormatYAML, structured); err != nil {
			return fmt.Errorf("marshal stream output: %w", err)
		}
		return checked.Complete(nil)
	default:
		_, err := fmt.Fprintln(checked, pretty)
		return checked.Complete(err)
	}
}
