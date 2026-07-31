package output

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// enumFlag is the pflag.Value behind a flag with a fixed set of string values.
// It rejects an unrecognised value at parse time.
//
// The flag used to be a plain string, and FormatFromCmd maps anything it does
// not recognise to the table renderer -- so `-o josn | jq` printed an
// ANSI-coloured table into jq and exited 0, with the typo reported as a
// downstream parse error, if at all. Validating here rather than in a
// PersistentPreRunE hook covers every command: the query subtree installs its
// own hook, and cobra runs only the closest one.
type enumFlag struct {
	value   string
	allowed []string
}

type constrainedValue struct {
	pflag.Value
	allowed []string
}

// NewFormatFlag returns the value to register --output with, seeded with the
// default format. Additional values are command-specific extensions, such as
// jsonl on workflow commands.
func NewFormatFlag(def string, additional ...string) pflag.Value {
	allowed := append([]string{"pretty", string(FormatJSON), string(FormatYAML)}, additional...)

	return NewEnumFlag(def, allowed...)
}

// NewEnumFlag returns a string flag value restricted to allowed.
func NewEnumFlag(def string, allowed ...string) pflag.Value {
	return &enumFlag{
		value:   def,
		allowed: append([]string(nil), allowed...),
	}
}

// ConstrainFlag applies a default and enum validation to an existing string
// flag. Flags already created by NewEnumFlag or NewFormatFlag retain their
// command-specific default and allowed values.
func ConstrainFlag(flag *pflag.Flag, def string, allowed ...string) {
	if flag == nil {
		return
	}
	if _, ok := flag.Value.(*enumFlag); ok {
		return
	}

	flag.Value = &constrainedValue{
		Value:   flag.Value,
		allowed: append([]string(nil), allowed...),
	}
	flag.DefValue = def
	_ = flag.Value.Set(def)
}

func (v *enumFlag) String() string { return v.value }

// Type is "string" so pflag's GetString keeps working on this flag.
func (v *enumFlag) Type() string { return "string" }

func (v *enumFlag) Set(s string) error {
	for _, allowed := range v.allowed {
		if s == allowed {
			v.value = s
			return nil
		}
	}

	return fmt.Errorf("must be one of %s", strings.Join(v.allowed, ", "))
}

func (v *constrainedValue) Set(s string) error {
	for _, allowed := range v.allowed {
		if s == allowed {
			return v.Value.Set(s)
		}
	}

	return fmt.Errorf("must be one of %s", strings.Join(v.allowed, ", "))
}
