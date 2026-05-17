// Package workflow generates CLI commands dynamically from workflow definitions.
// Commands only appear when a corresponding workflow YAML file exists — either
// as a built-in embedded definition or as a user-defined file in the config.
package workflow

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	wf "pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/akt/internal/workflow/builtin"
)

// Commands discovers available workflow definitions and returns a cobra command
// for each one. Only workflows that exist (built-in or user-defined) produce
// commands. Returns nil if no workflows are found.
func Commands(homeFn func() string, ctxNameFn func() string) []*cobra.Command {
	loader := wf.NewLoader(homeFn(), ctxNameFn(), builtin.Workflows())
	names := loader.List()

	var cmds []*cobra.Command
	for _, name := range names {
		def, err := loader.Load(name)
		if err != nil {
			continue // skip workflows that fail to parse
		}
		cmds = append(cmds, commandFromDef(def, homeFn, ctxNameFn))
	}

	return cmds
}

// commandFromDef generates a cobra command from a workflow definition.
// Flags are auto-generated from the workflow's declared params.
func commandFromDef(def *wf.WorkflowDef, homeFn func() string, ctxNameFn func() string) *cobra.Command {
	// Determine positional arg usage from params.
	use := def.Name
	var fileParam string
	for pname, p := range def.Params {
		if p.Type == wf.ParamFile && p.Required {
			use += " <" + pname + ">"
			fileParam = pname
		}
	}

	// If the workflow has an optional int param (like dseq), allow it as positional.
	var dseqParam string
	for pname, p := range def.Params {
		if p.Type == wf.ParamInt && pname == "dseq" {
			use += " [dseq]"
			dseqParam = pname
		}
	}

	cmd := &cobra.Command{
		Use:   use,
		Short: def.Description,
		Example: fmt.Sprintf("  akt %s --help", def.Name),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Reload the workflow at runtime (config may have changed).
			rtLoader := wf.NewLoader(homeFn(), ctxNameFn(), builtin.Workflows())
			rtDef, err := rtLoader.Load(def.Name)
			if err != nil {
				return fmt.Errorf("load workflow %q: %w", def.Name, err)
			}

			params := make(map[string]any)

			// Resolve file param from positional arg.
			argIdx := 0
			if fileParam != "" && argIdx < len(args) {
				params[fileParam] = args[argIdx]
				argIdx++
			}

			// Resolve dseq from positional or flag.
			if dseqParam != "" {
				dseq, _ := cmd.Flags().GetInt(dseqParam)
				if argIdx < len(args) {
					parsed, parseErr := strconv.Atoi(args[argIdx])
					if parseErr != nil {
						return fmt.Errorf("invalid dseq %q: %w", args[argIdx], parseErr)
					}
					dseq = parsed
				}
				if dseq != 0 {
					params[dseqParam] = dseq
				}
			}

			// Resolve remaining flag-based params.
			for pname, pdef := range rtDef.Params {
				if pname == fileParam || pname == dseqParam {
					continue // already handled
				}
				switch pdef.Type {
				case wf.ParamString:
					v, _ := cmd.Flags().GetString(pname)
					params[pname] = v
				case wf.ParamInt:
					v, _ := cmd.Flags().GetInt(pname)
					params[pname] = v
				case wf.ParamBool:
					v, _ := cmd.Flags().GetBool(pname)
					params[pname] = v
				case wf.ParamDuration:
					v, _ := cmd.Flags().GetString(pname)
					params[pname] = v
				}
			}

			dryRun, _ := cmd.Flags().GetBool("dry-run")
			out := cmd.OutOrStdout()

			fmt.Fprintf(out, "Workflow: %s (v%d)\n", rtDef.Name, rtDef.Version)
			fmt.Fprintf(out, "  %s\n\n", rtDef.Description)

			fmt.Fprintln(out, "Parameters:")
			for k, v := range params {
				fmt.Fprintf(out, "  %-16s %v\n", k+":", v)
			}
			fmt.Fprintln(out)

			fmt.Fprintf(out, "Steps (%d):\n", len(rtDef.Steps))
			for i, step := range rtDef.Steps {
				fmt.Fprintf(out, "  %d. [%s] %s", i+1, step.Type, step.Name)
				if step.Msg != "" {
					fmt.Fprintf(out, " -> %s", step.Msg)
				}
				if step.Action != "" {
					fmt.Fprintf(out, " -> %s", step.Action)
				}
				if step.OnError != "" {
					fmt.Fprintf(out, " (on-error: %s)", step.OnError)
				}
				if step.Retry != nil {
					fmt.Fprintf(out, " (retry: %dx, %s delay)", step.Retry.Max, step.Retry.Delay)
				}
				fmt.Fprintln(out)
			}

			if dryRun {
				fmt.Fprintln(out, "\nDry run — no transactions broadcast.")
				return nil
			}

			// TODO: Execute the workflow via Engine.Run() once
			// ChainClient and ProviderClient are wired.
			fmt.Fprintln(out, "\nExecution requires chain client (not yet wired). Use --dry-run to preview.")
			return nil
		},
	}

	// Auto-generate flags from workflow params.
	for pname, pdef := range def.Params {
		if pdef.Type == wf.ParamFile {
			continue // positional, not a flag
		}
		switch pdef.Type {
		case wf.ParamString:
			cmd.Flags().String(pname, pdef.Default, pdef.Description)
		case wf.ParamInt:
			def := 0
			if pdef.Default != "" {
				def, _ = strconv.Atoi(pdef.Default)
			}
			cmd.Flags().Int(pname, def, pdef.Description)
		case wf.ParamBool:
			cmd.Flags().Bool(pname, pdef.Default == "true", pdef.Description)
		case wf.ParamDuration:
			cmd.Flags().String(pname, pdef.Default, pdef.Description)
		}
	}

	// Common workflow flags.
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().Bool("dry-run", false, "Show execution plan without broadcasting transactions")

	// Set args validation based on what we expect.
	minArgs := 0
	maxArgs := 0
	if fileParam != "" {
		minArgs++
		maxArgs++
	}
	if dseqParam != "" {
		maxArgs++ // optional positional
	}
	cmd.Args = cobra.RangeArgs(minArgs, maxArgs)

	return cmd
}
