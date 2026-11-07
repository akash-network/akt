package flags

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	flagdefs "pkg.akt.dev/akt/internal/flags"
)

// RejectDuplicateArgs pairs each positional slot with its optional flag.
func RejectDuplicateArgs(flags *pflag.FlagSet, args []string, names ...string) error {
	for index, name := range names {
		if index < len(args) && flags.Changed(name) {
			return fmt.Errorf("%s cannot be supplied both positionally and with --%s", name, name)
		}
	}
	return nil
}

func DSeqFromFlagsAndArgs(flags *pflag.FlagSet, args []string) (uint64, error) {
	if err := RejectDuplicateArgs(flags, args, flagdefs.FlagDSeq); err != nil {
		return 0, err
	}
	fallback, err := flags.GetUint64(flagdefs.FlagDSeq)
	if err != nil {
		return 0, err
	}
	return DSeqFromArgs(args, fallback)
}

func ExpressionFromFlagsAndArgs(flags *pflag.FlagSet, args []string, name string) (string, error) {
	if err := RejectDuplicateArgs(flags, args, name); err != nil {
		return "", err
	}
	fallback, err := flags.GetString(name)
	if err != nil {
		return "", err
	}
	return ExprFromArgs(args, fallback), nil
}

// ResourceFilterArgs accepts one identity source and one state source.
func ResourceFilterArgs(state func(string) (string, error), identityFlags ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		maxArgs := 1
		if state != nil {
			maxArgs = 2
		}
		if err := cobra.MaximumNArgs(maxArgs)(cmd, args); err != nil {
			return err
		}
		flags := cmd.Flags()
		positionalState := len(args) > 1
		positionalIdentity := len(args) > 0
		if len(args) > 0 && state != nil {
			if _, err := state(args[0]); err == nil {
				positionalState = true
				positionalIdentity = false
			}
		}
		if state != nil && flags.Changed(flagdefs.FlagState) {
			if positionalState {
				return fmt.Errorf("state cannot be supplied both positionally and with --state")
			}
			value, _ := flags.GetString(flagdefs.FlagState)
			if value != "" {
				if _, err := state(value); err != nil {
					return err
				}
			}
		}
		for _, name := range identityFlags {
			if !flags.Changed(name) {
				continue
			}
			if positionalIdentity {
				return fmt.Errorf("use a positional identity or identity flags; --%s cannot accompany the identity argument", name)
			}
			value := flags.Lookup(name).Value.String()
			switch name {
			case flagdefs.FlagOwner, flagdefs.FlagProvider:
				if value != "" {
					if _, err := sdk.AccAddressFromBech32(value); err != nil {
						return fmt.Errorf("--%s: %w", name, err)
					}
				}
			}
		}
		return nil
	}
}

// PositionalOrFlagArgs rejects conflicting sources before command preflight.
func PositionalOrFlagArgs(names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.MaximumNArgs(len(names))(cmd, args); err != nil {
			return err
		}
		return RejectDuplicateArgs(cmd.Flags(), args, names...)
	}
}
