// Package workflow generates CLI commands dynamically from workflow definitions.
// Commands only appear when a corresponding workflow YAML file exists — either
// as a built-in embedded definition or as a user-defined file in the config.
package workflow

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"pkg.akt.dev/akt/internal/capability"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/cliutil"
	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
	"pkg.akt.dev/akt/internal/transport"
	wf "pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/akt/internal/workflow/builtin"
	"pkg.akt.dev/akt/internal/workflow/steps"
)

// outputJSONL is the --output value selecting JSONL step output (SPEC §2.3.8).
const outputJSONL = "jsonl"

// Commands discovers available workflow definitions and returns a cobra command
// for each one. Only workflows that exist (built-in or user-defined) produce
// commands. Returns nil if no workflows are found.
//
// Deprecated: commands built without a context manager cannot execute
// workflows (only --dry-run works). Use CommandsWithManager.
func Commands(homeFn func() string, ctxNameFn func() string) []*cobra.Command {
	return CommandsWithManager(homeFn, ctxNameFn, nil)
}

// CommandsWithManager is Commands plus the context manager the generated
// commands need to resolve credentials (keyring wallet vs Console API key)
// when executing workflows.
func CommandsWithManager(homeFn func() string, ctxNameFn func() string, mgrFn func() *aktctx.Manager) []*cobra.Command {
	loader := wf.NewLoader(homeFn(), ctxNameFn(), builtin.Workflows())
	names := loader.List()

	var cmds []*cobra.Command
	for _, name := range names {
		def, err := loader.Load(name)
		if err != nil {
			continue // skip workflows that fail to parse
		}
		cmds = append(cmds, commandFromDef(def, homeFn, ctxNameFn, mgrFn))
	}

	return cmds
}

// commandFromDef generates a cobra command from a workflow definition.
// Flags are auto-generated from the workflow's declared params.
func commandFromDef(def *wf.WorkflowDef, homeFn func() string, ctxNameFn func() string, mgrFn func() *aktctx.Manager) *cobra.Command {
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

	// discoverErr records a chain-client discovery failure from PreRunE so
	// RunE can surface it inside a clearer, workflow-specific error.
	var discoverErr error

	cmd := &cobra.Command{
		Use:     use,
		Short:   def.Description,
		Example: fmt.Sprintf("  akt %s --help", def.Name),
		// Workflow commands run on either rail (internal/transport): chain
		// tx broadcasting on keyring contexts or Console API calls on
		// console-api contexts. Either capability satisfies the gate.
		Annotations: map[string]string{
			capability.AnnotationKey: string(capability.ChainTx) + "|" + string(capability.Console),
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Dry runs never need clients.
			if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
				return nil
			}

			// Chain client discovery only applies when running under the
			// real root command (which seeds the SDK client context) with a
			// keyring-auth context. console-api contexts do not need a
			// wallet or an RPC connection to execute.
			if cmd.Context() == nil || cmd.Context().Value(chaincli.ClientContextKey) == nil {
				return nil
			}

			rc := resolveContext(mgrFn, ctxNameFn)
			if rc == nil || rc.AuthMethod == aktctx.AuthMethodConsoleAPI {
				return nil // missing context/credentials surfaced in RunE
			}

			if err := chaincli.TxPersistentPreRunE(cmd, args); err != nil {
				// Keep the cause; RunE reports it with workflow context.
				discoverErr = err
			}

			return nil
		},
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
			jsonl := outputFormat(cmd) == outputJSONL

			// Print the execution plan, except in JSONL mode where stdout
			// must carry only JSONL step lines.
			if dryRun || !jsonl {
				printPlan(out, rtDef, params)
			}

			if dryRun {
				fmt.Fprintln(out, "\nDry run — no transactions broadcast.")
				return nil
			}

			return executeWorkflow(cmd, rtDef, params, mgrFn, ctxNameFn, jsonl, discoverErr)
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
	cmd.Flags().VarP(output.NewFormatFlag(cflags.OutputPretty, outputJSONL), cflags.FlagOutput, "o", "Output format (pretty|json|yaml|jsonl)")

	// Standard chain tx flags (--from, --gas, --node, ...) so keyring-auth
	// execution can discover a chain client; flags already defined above
	// (e.g. --yes, --dry-run, workflow params) keep their workflow meaning.
	addMissingTxFlags(cmd)

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

// executeWorkflow resolves credentials for the active context, picks the
// transport for its rail (internal/transport: chain for keyring auth,
// console for console-api auth — abstracted away from the user; the command
// arguments are identical on both), runs the engine, and renders per-step
// results.
func executeWorkflow(
	cmd *cobra.Command,
	rtDef *wf.WorkflowDef,
	params map[string]any,
	mgrFn func() *aktctx.Manager,
	ctxNameFn func() string,
	jsonl bool,
	discoverErr error,
) error {
	out := cmd.OutOrStdout()

	if mgrFn == nil || mgrFn() == nil {
		return fmt.Errorf("no configuration loaded; workflow execution requires a configured context (create one with `akt context create`)")
	}

	rc, err := mgrFn().Resolve(ctxNameFn())
	if err != nil {
		return fmt.Errorf("no usable context for workflow execution: %w", err)
	}

	var (
		chainCl    steps.ChainClient
		providerCl steps.ProviderClient
	)

	account := rc.DefaultAccount

	if rc.AuthMethod == aktctx.AuthMethodConsoleAPI {
		if rc.ConsoleAPIKey == "" {
			return fmt.Errorf(
				"context %q uses console-api auth but no Console credential is available.\n"+
					"Set %s, store a key with `akt context edit %s --console-api-key <key>`, or switch to a keyring context",
				rc.Name, aktctx.EnvConsoleAPIKey, rc.Name,
			)
		}

		cc := console.New(rc.ConsoleAPIURL, rc.ConsoleAPIKey).
			WithActionLog(cliutil.ActionLogFromContext(cmd.Context()))

		// Chain queries still go directly to the chain when a client is
		// available (SPEC §7.4); otherwise the console transport falls back
		// to the Console bids endpoint.
		var chainQueries steps.ChainClient
		if cl, cerr := chaincli.ClientFromContext(cmd.Context()); cerr == nil {
			chainQueries = transport.NewChain(cl)
		}

		chainCl = transport.NewConsole(cc, chainQueries, rc.Root, rc.Name)

		// Provider gateway steps are not supported with console-api auth:
		// the Console API submits the manifest internally during lease
		// creation (SPEC §7.4), so drop them instead of failing the run.
		rtDef = filterProviderSteps(rtDef, cmd.ErrOrStderr())
	} else {
		cl, cerr := chaincli.ClientFromContext(cmd.Context())
		if cerr != nil {
			detail := cerr
			if discoverErr != nil {
				detail = discoverErr
			}

			return fmt.Errorf(
				"no wallet/chain client available for context %q (keyring auth): %v.\n"+
					"Keyring execution needs a reachable RPC node and a configured key; check the context's network endpoints and keyring, or switch to a console-api context with an API key",
				rc.Name, detail,
			)
		}

		chainCl = transport.NewChain(cl)
		providerCl = transport.NewProvider(cl.ClientContext(), rc.AuthType)

		if addr := cl.ClientContext().GetFromAddress(); !addr.Empty() {
			account = addr.String()
		} else if name := cl.ClientContext().FromName; name != "" {
			account = name
		}
	}

	registry := steps.NewRegistry(chainCl, providerCl)
	if jsonl {
		// Keep stdout pure JSONL: output-step text is recorded in the step
		// result ("text" output) instead of printed.
		registry.Register(&steps.OutputExecutor{Out: io.Discard})
	}

	logger := wf.NewActionLogAdapter(cliutil.ActionLogFromContext(cmd.Context()), rtDef.Name)
	engine := wf.NewEngine(registry, logger)

	state, runErr := engine.Run(cmd.Context(), rtDef, account, params)

	if jsonl {
		emitJSONL(out, state)
	} else {
		printResults(out, state, runErr)
	}

	if runErr != nil {
		return fmt.Errorf("workflow %q failed: %w", rtDef.Name, runErr)
	}

	return nil
}

// resolveContext resolves the active context, returning nil when no manager
// or context is available.
func resolveContext(mgrFn func() *aktctx.Manager, ctxNameFn func() string) *aktctx.Context {
	if mgrFn == nil {
		return nil
	}

	mgr := mgrFn()
	if mgr == nil {
		return nil
	}

	rc, err := mgr.Resolve(ctxNameFn())
	if err != nil {
		return nil
	}

	return rc
}

// filterProviderSteps returns a copy of def without provider-type steps,
// noting each skipped step on w. Used for console-api auth, where manifest
// submission is handled by the Console API (SPEC §7.4).
func filterProviderSteps(def *wf.WorkflowDef, w io.Writer) *wf.WorkflowDef {
	kept := make([]wf.StepDef, 0, len(def.Steps))
	removed := false

	for _, s := range def.Steps {
		if s.Type == wf.StepProvider {
			fmt.Fprintf(w, "note: skipping step %q (manifest submission handled by Console)\n", s.Name)
			removed = true
			continue
		}
		kept = append(kept, s)
	}

	if !removed {
		return def
	}

	filtered := *def
	filtered.Steps = kept

	return &filtered
}

// printPlan renders the workflow execution plan (name, params, steps).
func printPlan(out io.Writer, rtDef *wf.WorkflowDef, params map[string]any) {
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
}

// printResults renders per-step outcomes and the overall workflow status in
// simple aligned text.
func printResults(out io.Writer, state *wf.RunState, runErr error) {
	fmt.Fprintln(out, "\nResults:")

	for _, name := range state.StepOrder {
		sr := state.Steps[name]
		if sr == nil {
			continue
		}

		fmt.Fprintf(out, "  %-20s %-9s", sr.Name, sr.Status)
		if sr.TxHash != "" {
			fmt.Fprintf(out, "  tx: %s", sr.TxHash)
		}
		if sr.Error != "" {
			fmt.Fprintf(out, "  error: %s", sr.Error)
		}
		fmt.Fprintln(out)
	}

	if runErr == nil {
		fmt.Fprintf(out, "\nWorkflow %q completed successfully.\n", state.Workflow)
	}
}

// jsonlTx is the tx object of a JSONL step line (SPEC §2.3.8).
type jsonlTx struct {
	Hash    string `json:"hash"`
	Height  int64  `json:"height"`
	GasUsed int64  `json:"gas_used,omitempty"`
	Code    uint32 `json:"code"`
}

// jsonlLine is one JSONL step line (SPEC §2.3.8).
type jsonlLine struct {
	Workflow string    `json:"workflow"`
	ID       string    `json:"id"`
	Step     string    `json:"step"`
	Result   string    `json:"result"`
	Errors   []string  `json:"errors"`
	Txs      []jsonlTx `json:"txs"`
}

// emitJSONL writes one JSONL line per completed step (SPEC §2.3.8).
func emitJSONL(out io.Writer, state *wf.RunState) {
	enc := json.NewEncoder(out)

	for _, name := range state.StepOrder {
		sr := state.Steps[name]
		if sr == nil {
			continue
		}

		line := jsonlLine{
			Workflow: state.Workflow,
			ID:       state.WorkflowID,
			Step:     sr.Name,
			Errors:   []string{},
			Txs:      []jsonlTx{},
		}

		switch sr.Status {
		case "success":
			line.Result = "completed"
		case "skipped":
			line.Result = "skipped"
		default:
			line.Result = "error"
		}

		if sr.Error != "" {
			line.Errors = append(line.Errors, sr.Error)
		}

		if sr.TxHash != "" {
			line.Txs = append(line.Txs, jsonlTx{
				Hash:   sr.TxHash,
				Height: sr.Height,
			})
		}

		_ = enc.Encode(line)
	}
}

// outputFormat returns the effective --output value for the command,
// defaulting to "pretty" when the flag is not registered (e.g. bare command
// execution in tests).
func outputFormat(cmd *cobra.Command) string {
	if f := cmd.Flags().Lookup("output"); f != nil && f.Value.String() != "" {
		return f.Value.String()
	}

	return "pretty"
}

// addMissingTxFlags registers the standard chain tx flags (defined by
// cflags.AddTxFlagsToCmd) on cmd, skipping any flag whose name or shorthand
// the command already uses (e.g. --yes, --dry-run, or a workflow param).
func addMissingTxFlags(cmd *cobra.Command) {
	tmp := &cobra.Command{}
	cflags.AddTxFlagsToCmd(tmp)

	tmp.Flags().VisitAll(func(f *pflag.Flag) {
		if cmd.Flags().Lookup(f.Name) != nil {
			return
		}
		if f.Shorthand != "" && cmd.Flags().ShorthandLookup(f.Shorthand) != nil {
			return
		}
		cmd.Flags().AddFlag(f)
	})
}
