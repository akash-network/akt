package sdl

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"pkg.akt.dev/akt/internal/cliutil"
)

// Commands returns the `akt sdl` command group. The group is transport-
// independent: scaffolding and validation run entirely locally, so no
// context, keyring, or RPC endpoint is required and no capability
// annotations are declared.
func Commands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sdl",
		Short: "Author and validate deployment SDLs",
		Long: `Generate SDL manifests from built-in scaffolds and validate them offline.

All sdl subcommands run locally and work without a configured context,
key, or network connection.`,
	}

	cmd.AddCommand(
		scaffoldsCmd(),
		initCmd(),
		validateCmd(),
	)

	return cmd
}

func scaffoldsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "scaffolds",
		// Back-compat with the reference CLI, where this command was named
		// `sdl templates` before "template" was taken by the Console catalog.
		Aliases: []string{"templates"},
		Short:   "List the built-in SDL scaffolds",
		Long:    "List the local SDL scaffolds that `akt sdl init` can generate, with the flags each one honors.",
		Args:    cobra.NoArgs,
		Example: `  akt sdl scaffolds`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)

			fmt.Fprintln(w, "NAME\tDESCRIPTION\tFLAGS")
			for _, s := range Scaffolds() {
				fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.Description, strings.Join(s.Params, " "))
			}

			if err := w.Flush(); err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "\nGenerate one with: akt sdl init <scaffold> [flags] > deploy.yaml")

			return nil
		},
	}
}

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init <scaffold>",
		Short: "Generate an SDL from a scaffold (YAML on stdout)",
		Long: `Generate a deployable SDL from a built-in scaffold and print it to stdout,
ready to redirect to a file or pipe into another command. The output is
self-checked against "akt sdl validate" before it is printed.`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			return ScaffoldNames(), cobra.ShellCompDirectiveNoFileComp
		},
		Example: `  # Generate a web service SDL
  akt sdl init web > deploy.yaml

  # GPU workload with a specific model
  akt sdl init gpu --gpu-model h100 --image myorg/model:1.0

  # Validate the generated SDL from stdin
  akt sdl init web --image nginx:1.27 | akt sdl validate -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sc := Lookup(args[0])
			if sc == nil {
				return &cliutil.CLIError{
					Code:       cliutil.ExitUsage,
					Message:    fmt.Sprintf("unknown scaffold %q", args[0]),
					Suggestion: fmt.Sprintf("Available scaffolds: %s (see \"akt sdl scaffolds\").", strings.Join(ScaffoldNames(), ", ")),
				}
			}

			opts, err := optionsFromFlags(cmd)
			if err != nil {
				return err
			}

			out, err := Marshal(sc.Build(opts))
			if err != nil {
				return fmt.Errorf("marshal SDL: %w", err)
			}

			// Safety net: a scaffold must never emit an SDL that its own
			// validator rejects.
			if res := Validate(out); !res.Valid {
				msgs := make([]string, len(res.Errors))
				for i, e := range res.Errors {
					msgs[i] = e.Message
				}

				return fmt.Errorf("internal error: generated SDL failed validation: %s", strings.Join(msgs, "; "))
			}

			_, err = cmd.OutOrStdout().Write(out)

			return err
		},
	}

	// Generation parameters, not positional-argument twins: each flag is an
	// optional knob with a per-scaffold default (see Options), so the
	// zero-flag invocation always produces a deployable SDL.
	fl := cmd.Flags()
	fl.String("name", "", "Service name (default per scaffold)")
	fl.String("image", "", "Container image; must be tagged, e.g. nginx:1.27")
	fl.Int("port", 0, "Container port to expose (default per scaffold)")
	fl.Int("as", 0, "External port (default per scaffold)")
	fl.String("cpu", "", "CPU units, e.g. 0.5 or 500m")
	fl.String("memory", "", "Memory size, e.g. 512Mi, 2Gi")
	fl.String("storage", "", "Storage size, e.g. 1Gi")
	fl.Int("count", 0, "Replica count")
	fl.Int("price", 0, "Max price per block in uact")
	fl.StringArray("env", nil, "Environment variable KEY=value (repeatable)")
	fl.Int("gpu", 0, "GPU units (gpu scaffold)")
	fl.String("gpu-model", "", "NVIDIA GPU model, e.g. a100 (gpu scaffold)")

	return cmd
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate an SDL offline (use - for stdin)",
		Long: `Parse and validate an SDL document without touching the network, using
the same parser as "akt deploy" and the chain tx commands, then apply
best-practice lint rules (pinned image tags, pricing denoms).

Exits 0 when the SDL is valid (warnings allowed) and 1 when it is not.`,
		Args: cobra.ExactArgs(1),
		Example: `  # Validate a file
  akt sdl validate deploy.yaml

  # Validate from stdin
  akt sdl init web | akt sdl validate -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, name, err := readFileOrStdin(cmd, args[0])
			if err != nil {
				return err
			}

			res := Validate(data)

			if res.Valid {
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "valid: %d service(s), %d group(s), %d warning(s)\n",
					res.Services, res.Groups, len(res.Warnings))
				printIssues(out, "warning", res.Warnings)

				return nil
			}

			errOut := cmd.ErrOrStderr()
			fmt.Fprintf(errOut, "invalid: %d error(s), %d warning(s)\n", len(res.Errors), len(res.Warnings))
			printIssues(errOut, "error", res.Errors)
			printIssues(errOut, "warning", res.Warnings)

			return &cliutil.CLIError{
				Code:       cliutil.ExitGeneral,
				Message:    fmt.Sprintf("SDL validation failed with %d error(s)", len(res.Errors)),
				Context:    fmt.Sprintf("validating %s", name),
				Suggestion: "Fix the errors above and re-run \"akt sdl validate\".",
			}
		},
	}
}

func printIssues(w io.Writer, kind string, issues []Issue) {
	for _, i := range issues {
		fmt.Fprintf(w, "  %s: %s: %s\n", kind, i.Path, i.Message)

		if i.Hint != "" {
			fmt.Fprintf(w, "    hint: %s\n", i.Hint)
		}
	}
}

// readFileOrStdin reads the SDL bytes from a path, or from stdin when the
// argument is "-". The returned name is used in error context.
func readFileOrStdin(cmd *cobra.Command, arg string) (data []byte, name string, err error) {
	if arg == "-" {
		data, err = io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, "", fmt.Errorf("read stdin: %w", err)
		}

		return data, "stdin", nil
	}

	data, err = os.ReadFile(arg)
	if err != nil {
		return nil, "", &cliutil.CLIError{
			Code:    cliutil.ExitUsage,
			Message: fmt.Sprintf("cannot read SDL file %q", arg),
			Cause:   err,
		}
	}

	return data, arg, nil
}

// optionsFromFlags collects the generation parameters. Int flags use
// Changed() so a scaffold can tell "not set" (use its default) apart from
// an explicit value.
func optionsFromFlags(cmd *cobra.Command) (Options, error) {
	fl := cmd.Flags()

	o := Options{}
	o.Name, _ = fl.GetString("name")
	o.Image, _ = fl.GetString("image")
	o.CPU, _ = fl.GetString("cpu")
	o.Memory, _ = fl.GetString("memory")
	o.Storage, _ = fl.GetString("storage")
	o.GPUModel, _ = fl.GetString("gpu-model")

	var err error
	if o.Port, err = intFlag(fl, "port"); err != nil {
		return o, err
	}
	if o.As, err = intFlag(fl, "as"); err != nil {
		return o, err
	}
	if o.Count, err = intFlag(fl, "count"); err != nil {
		return o, err
	}
	if o.Price, err = intFlag(fl, "price"); err != nil {
		return o, err
	}
	if o.GPU, err = intFlag(fl, "gpu"); err != nil {
		return o, err
	}

	env, _ := fl.GetStringArray("env")
	for _, e := range env {
		if !strings.Contains(e, "=") {
			return o, cliutil.ErrUsage(fmt.Sprintf("--env expects KEY=value, got %q", e), nil)
		}
	}
	o.Env = env

	return o, nil
}

// intFlag returns the flag value only when it was explicitly set, and
// rejects negative values.
func intFlag(fl *pflag.FlagSet, name string) (*int, error) {
	if !fl.Changed(name) {
		return nil, nil
	}

	v, err := fl.GetInt(name)
	if err != nil {
		return nil, err
	}

	if v < 0 {
		return nil, cliutil.ErrUsage(fmt.Sprintf("--%s must be a non-negative integer, got %d", name, v), nil)
	}

	return &v, nil
}
