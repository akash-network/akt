package sdl

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
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
		RunE:  sdkclient.ValidateCmd,
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

			if err := rejectInapplicableFlags(cmd, sc); err != nil {
				return err
			}

			opts, err := optionsFromFlags(cmd)
			if err != nil {
				return err
			}

			out, err := Marshal(sc.Build(opts))
			if err != nil {
				return fmt.Errorf("marshal SDL: %w", err)
			}

			if err := validateGeneratedSDL(cmd, sc, out); err != nil {
				return err
			}

			_, err = cmd.OutOrStdout().Write(out)

			return err
		},
	}

	// Generation parameters, not positional-argument twins: each flag is an
	// optional knob with a per-scaffold default (see Options), so the
	// zero-flag invocation always produces a deployable SDL. Int flags are
	// range-checked in optionsFromFlags (Changed() tells an explicit value
	// apart from the unset default), so out-of-range input — including an
	// explicit 0 — is a usage error, never an internal one.
	fl := cmd.Flags()
	fl.String("name", "", "Service name (default per scaffold)")
	fl.String("image", "", "Container image pinned by tag or sha256 digest, e.g. nginx:1.27")
	fl.Int("port", 0, "Container port to expose, 1-65535 (default per scaffold)")
	fl.Int("as", 0, "External port, 1-65535 (default per scaffold)")
	fl.String("cpu", "", "CPU units, e.g. 0.5 or 500m")
	fl.String("memory", "", "Memory size, e.g. 512Mi, 2Gi")
	fl.String("storage", "", "Storage size, e.g. 1Gi")
	fl.Int("count", 0, "Replica count, minimum 1 (default per scaffold)")
	fl.Int("price", 0, "Max price per block in uact, minimum 1 (default per scaffold)")
	fl.StringArray("env", nil, "Environment variable KEY=value (repeatable)")
	fl.Int("gpu", 0, "GPU units, minimum 1 (gpu scaffold)")
	fl.String("gpu-model", "", "NVIDIA GPU model, e.g. a100 (gpu scaffold)")

	return cmd
}

// validateGeneratedSDL is the final boundary between scaffold parameters and
// generated output. Validation failures caused by explicit parameters are
// usage errors; a scaffold that fails without overrides is a broken built-in
// invariant and remains an internal error.
func validateGeneratedSDL(cmd *cobra.Command, sc *Scaffold, out []byte) error {
	res := Validate(out)
	if res.Valid {
		return nil
	}

	changed := changedScaffoldFlags(cmd, sc)
	if len(changed) == 0 {
		return generatedSDLInvariantError(sc, res)
	}

	defaultOut, err := Marshal(sc.Build(Options{}))
	if err != nil {
		return fmt.Errorf("internal error: marshal default %q scaffold: %w", sc.Name, err)
	}

	if defaultRes := Validate(defaultOut); !defaultRes.Valid {
		return generatedSDLInvariantError(sc, defaultRes)
	}

	return &cliutil.CLIError{
		Code:       cliutil.ExitUsage,
		Message:    fmt.Sprintf("invalid scaffold input for %s", strings.Join(changed, ", ")),
		Cause:      fmt.Errorf("%s", formatValidationIssues(res.Errors)),
		Context:    fmt.Sprintf("generating scaffold %q", sc.Name),
		Suggestion: validationSuggestion(res.Errors),
	}
}

func generatedSDLInvariantError(sc *Scaffold, res Result) error {
	return fmt.Errorf("internal error: scaffold %q default output failed validation: %s",
		sc.Name, formatValidationIssues(res.Errors))
}

func formatValidationIssues(issues []Issue) string {
	msgs := make([]string, len(issues))
	for i, issue := range issues {
		msgs[i] = fmt.Sprintf("%s: %s", issue.Path, issue.Message)
	}

	return strings.Join(msgs, "; ")
}

func validationSuggestion(issues []Issue) string {
	for _, issue := range issues {
		if issue.Hint != "" {
			return issue.Hint
		}
	}

	return "Correct the flagged values and run the command again."
}

func changedScaffoldFlags(cmd *cobra.Command, sc *Scaffold) []string {
	changed := make([]string, 0, len(sc.Params))
	for _, param := range sc.Params {
		if cmd.Flags().Changed(strings.TrimPrefix(param, "--")) {
			changed = append(changed, param)
		}
	}

	sort.Strings(changed)

	return changed
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate an SDL offline (use - for stdin)",
		Long: `Parse and validate an SDL document without touching the network, using
the same parser as "akt deploy" and the chain tx commands, then apply
best-practice lint rules (pinned image references, pricing denoms).

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
	if o.Port, err = intFlag(fl, "port", 1, maxPort); err != nil {
		return o, err
	}
	if o.As, err = intFlag(fl, "as", 1, maxPort); err != nil {
		return o, err
	}
	if o.Count, err = intFlag(fl, "count", 1, 0); err != nil {
		return o, err
	}
	if o.Price, err = intFlag(fl, "price", 1, 0); err != nil {
		return o, err
	}
	if o.GPU, err = intFlag(fl, "gpu", 1, 0); err != nil {
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

// maxPort is the highest valid TCP/UDP port for --port/--as.
const maxPort = 65535

// intFlag returns the flag value only when it was explicitly set —
// pflag's Changed() distinguishes an explicit zero from the unset default,
// which falls back to the per-scaffold value. Explicitly set values are
// range-checked here so invalid input (an explicit 0, a negative number, a
// port above 65535) fails as a usage error (exit 2) before generation
// instead of surfacing later as an internal self-validation failure.
// maxVal <= 0 means "no upper bound".
func intFlag(fl *pflag.FlagSet, name string, minVal, maxVal int) (*int, error) {
	if !fl.Changed(name) {
		return nil, nil
	}

	v, err := fl.GetInt(name)
	if err != nil {
		return nil, err
	}

	if v < minVal || (maxVal > 0 && v > maxVal) {
		if maxVal > 0 {
			return nil, cliutil.ErrUsage(fmt.Sprintf("--%s must be between %d and %d, got %d", name, minVal, maxVal, v), nil)
		}

		return nil, cliutil.ErrUsage(fmt.Sprintf("--%s must be at least %d, got %d", name, minVal, v), nil)
	}

	return &v, nil
}

// rejectInapplicableFlags fails when the user set a flag the chosen scaffold
// does not implement.
//
// Every scaffold declares the flags it honours, and `akt sdl scaffolds` prints
// them, but nothing checked the two against each other -- so
// `akt sdl init web --gpu 4` silently produced a CPU-only SDL that deploys and
// bills perfectly well while the user believes they provisioned a GPU. The
// generated file contains the string "gpu" zero times.
func rejectInapplicableFlags(cmd *cobra.Command, sc *Scaffold) error {
	applicable := make(map[string]struct{}, len(sc.Params))
	for _, p := range sc.Params {
		applicable[strings.TrimPrefix(p, "--")] = struct{}{}
	}

	var offenders []string

	cmd.Flags().Visit(func(f *pflag.Flag) {
		// Visit reports only flags the user actually set. Globals such as
		// --output are not scaffold parameters and are never offenders.
		if _, ok := applicable[f.Name]; ok {
			return
		}

		if _, scaffoldParam := allScaffoldParams()[f.Name]; !scaffoldParam {
			return
		}

		offenders = append(offenders, "--"+f.Name)
	})

	if len(offenders) == 0 {
		return nil
	}

	sort.Strings(offenders)

	return &cliutil.CLIError{
		Code: cliutil.ExitUsage,
		Message: fmt.Sprintf("scaffold %q does not use %s",
			sc.Name, strings.Join(offenders, ", ")),
		Suggestion: fmt.Sprintf("%q accepts: %s (see \"akt sdl scaffolds\").",
			sc.Name, strings.Join(sc.Params, " ")),
	}
}

// allScaffoldParams is the union of every scaffold's parameters, used to tell a
// scaffold parameter apart from a global flag.
func allScaffoldParams() map[string]struct{} {
	all := map[string]struct{}{}

	for i := range scaffoldRegistry {
		for _, p := range scaffoldRegistry[i].Params {
			all[strings.TrimPrefix(p, "--")] = struct{}{}
		}
	}

	return all
}
