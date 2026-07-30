package output

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// formatFlag is the pflag.Value behind --output. It rejects an unrecognised
// format at parse time.
//
// The flag used to be a plain string, and FormatFromCmd maps anything it does
// not recognise to the table renderer -- so `-o josn | jq` printed an
// ANSI-coloured table into jq and exited 0, with the typo reported as a
// downstream parse error, if at all. Validating here rather than in a
// PersistentPreRunE hook covers every command: the query subtree installs its
// own hook, and cobra runs only the closest one.
type formatFlag string

// NewFormatFlag returns the value to register --output with, seeded with the
// default format.
func NewFormatFlag(def string) pflag.Value {
	v := formatFlag(def)

	return &v
}

func (v *formatFlag) String() string { return string(*v) }

// Type is "string" so pflag's GetString keeps working on this flag.
func (v *formatFlag) Type() string { return "string" }

func (v *formatFlag) Set(s string) error {
	switch s {
	// "pretty" and "table" both select the table renderer; both spellings are
	// already in use across the help text.
	case "pretty", "table", string(FormatJSON), string(FormatYAML):
		*v = formatFlag(s)

		return nil
	default:
		return fmt.Errorf("must be one of %s", strings.Join([]string{"pretty", "json", "yaml"}, ", "))
	}
}
