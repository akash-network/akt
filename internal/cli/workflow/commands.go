// Package workflow generates CLI commands dynamically from workflow definitions.
// Commands only appear when a corresponding workflow YAML file exists — either
// as a built-in embedded definition or as a user-defined file in the config.
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	flagdefs "pkg.akt.dev/akt/internal/flags"

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
	"pkg.akt.dev/akt/internal/workflow/adapters"
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
	paramNames := sortedParamNames(def.Params)
	var positionalParams []string
	for _, pname := range paramNames {
		p := def.Params[pname]
		if (p.Type == wf.ParamFile || p.Type == wf.ParamSDL) && p.Required {
			use += " <" + pname + ">"
			positionalParams = append(positionalParams, pname)
		}
	}
	positional := make(map[string]struct{}, len(positionalParams))
	for _, pname := range positionalParams {
		positional[pname] = struct{}{}
	}

	// If the workflow has an optional int param (like dseq), allow it as positional.
	var dseqParam string
	for _, pname := range paramNames {
		p := def.Params[pname]
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
		Long:    def.Long,
		Example: def.Example,
		// Workflow commands run on either rail (internal/transport): chain
		// tx broadcasting on keyring contexts or Console API calls on
		// console-api contexts. Either capability satisfies the gate.
		Annotations: map[string]string{
			capability.AnnotationKey: string(capability.ChainTx) + "|" + string(capability.Console),
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Dry runs only discover a read client when the chain rail must
			// resolve an auto deposit from live deployment parameters.
			if dryRun, _ := cmd.Flags().GetBool(flagdefs.FlagDryRun); dryRun {
				rc := resolveContext(mgrFn, ctxNameFn)
				depositFlag := cmd.Flags().Lookup(flagdefs.FlagDeposit)
				if rc == nil || rc.AuthMethod == aktctx.AuthMethodConsoleAPI || depositFlag == nil {
					return nil
				}
				parsed, parseErr := transport.ParseDeposit(depositFlag.Value.String())
				if parseErr != nil {
					// RunE owns the user-facing deposit validation. PreRunE only
					// decides whether an otherwise-valid auto value needs a query
					// client before planning.
					return nil //nolint:nilerr // the parsing error is returned by RunE
				}
				if !parsed.Auto {
					return nil
				}

				return chaincli.QueryPersistentPreRunE(cmd, args)
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

			// Resolve required file and SDL params from positional args.
			argIdx := 0
			for _, pname := range positionalParams {
				if argIdx < len(args) {
					params[pname] = args[argIdx]
					argIdx++
				}
			}

			// Resolve dseq from positional or flag.
			if dseqParam != "" {
				dseq, _ := cmd.Flags().GetInt(dseqParam)
				dseqSet := cmd.Flags().Changed(dseqParam)
				if argIdx < len(args) {
					if dseqSet {
						return fmt.Errorf(
							"%s supplied both positionally and with --%s; use one form",
							dseqParam,
							dseqParam,
						)
					}
					parsed, parseErr := strconv.Atoi(args[argIdx])
					if parseErr != nil {
						return fmt.Errorf("invalid dseq %q: %w", args[argIdx], parseErr)
					}
					dseq = parsed
					dseqSet = true
				}
				if dseqSet {
					params[dseqParam] = dseq
				}
			}

			// Resolve remaining flag-based params.
			for _, pname := range sortedParamNames(rtDef.Params) {
				pdef := rtDef.Params[pname]
				if _, ok := positional[pname]; ok || pname == dseqParam {
					continue // already handled
				}
				switch pdef.Type {
				case wf.ParamString, wf.ParamFile, wf.ParamSDL, wf.ParamDeposit, wf.ParamBidSelection:
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

			if err := validateWorkflowParams(rtDef, params); err != nil {
				return err
			}

			dryRun, _ := cmd.Flags().GetBool(flagdefs.FlagDryRun)
			out := cmd.OutOrStdout()
			jsonl := outputFormat(cmd) == outputJSONL

			if dryRun {
				params, err = resolveDryRunParams(cmd, params, mgrFn, ctxNameFn)
				if err != nil {
					return err
				}
				if jsonl {
					return emitDryRunJSONL(out, rtDef, params)
				}

				if err := printPlan(out, rtDef, params, workflowRail(mgrFn, ctxNameFn)); err != nil {
					return err
				}
				return writeWorkflowReport(out, "\nDry run — no transactions broadcast.\n", "dry-run")
			}

			// During execution, keep stdout pure in JSONL mode; other modes
			// retain the human plan before step results.
			if !jsonl {
				if err := printPlan(out, rtDef, params, workflowRail(mgrFn, ctxNameFn)); err != nil {
					return err
				}
			}

			return executeWorkflow(cmd, rtDef, params, mgrFn, ctxNameFn, jsonl, discoverErr)
		},
	}

	// Auto-generate flags from workflow params.
	for _, pname := range paramNames {
		pdef := def.Params[pname]
		if _, ok := positional[pname]; ok {
			continue // positional, not a flag
		}
		switch pdef.Type {
		case wf.ParamString, wf.ParamFile, wf.ParamSDL, wf.ParamDeposit, wf.ParamBidSelection:
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
	cmd.Flags().BoolP(flagdefs.FlagSkipConfirmation, "y", false, "Skip confirmation prompts")
	cmd.Flags().Bool(flagdefs.FlagDryRun, false, "Show execution plan without broadcasting transactions")
	cmd.Flags().VarP(output.NewFormatFlag(cflags.OutputPretty, outputJSONL), flagdefs.FlagOutput, "o", "Output format (pretty|json|yaml|jsonl)")

	// Standard chain tx flags (--from, --gas, --node, ...) so keyring-auth
	// execution can discover a chain client; flags already defined above
	// (e.g. --yes, --dry-run, workflow params) keep their workflow meaning.
	addMissingTxFlags(cmd)

	// Set args validation based on what we expect.
	minArgs := len(positionalParams)
	maxArgs := len(positionalParams)
	if dseqParam != "" {
		maxArgs++ // optional positional
	}
	cmd.Args = cobra.RangeArgs(minArgs, maxArgs)

	return cmd
}

func resolveDryRunParams(
	cmd *cobra.Command,
	params map[string]any,
	mgrFn func() *aktctx.Manager,
	ctxNameFn func() string,
) (map[string]any, error) {
	raw, ok := params[flagdefs.FlagDeposit].(string)
	if !ok {
		return params, nil
	}
	rc := resolveContext(mgrFn, ctxNameFn)
	if rc == nil {
		// Preserve the deprecated standalone Commands API, which intentionally
		// has no rail. Production commands always have a resolved manager.
		return params, nil
	}

	parsed, err := transport.ParseDeposit(raw)
	if err != nil {
		return nil, err
	}
	resolved := ""
	if rc.AuthMethod == aktctx.AuthMethodConsoleAPI {
		resolved, err = parsed.RailValue(transport.KindConsole)
		if err == nil && parsed.Auto {
			err = fmt.Errorf("deposit %q: console-api deployments require an explicit USD amount of at least $%.2f", raw, transport.MinConsoleDepositUSD)
		}
		if err == nil && parsed.USD < transport.MinConsoleDepositUSD {
			err = fmt.Errorf("deposit must be at least $%.2f on the Console rail (got $%.2f)", transport.MinConsoleDepositUSD, parsed.USD)
		}
	} else {
		resolved, err = parsed.RailValue(transport.KindChain)
		if err == nil && parsed.Auto {
			cl, clientErr := chaincli.LightClientFromContext(cmd.Context())
			if clientErr != nil {
				return nil, fmt.Errorf("resolve chain auto deposit: %w", clientErr)
			}
			resolved, err = adapters.ResolveChainDepositValue(cmd.Context(), cl, resolved)
		}
	}
	if err != nil {
		return nil, err
	}

	copyParams := make(map[string]any, len(params))
	for key, value := range params {
		copyParams[key] = value
	}
	copyParams[flagdefs.FlagDeposit] = resolved

	return copyParams, nil
}

func sortedParamNames(params map[string]wf.ParamDef) []string {
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
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
		chainCl       steps.ChainClient
		providerCl    steps.ProviderClient
		consoleClient *console.Client
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
		consoleClient = cc

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
		rtDef, err = filterProviderSteps(rtDef, cmd.ErrOrStderr())
		if err != nil {
			return fmt.Errorf("report skipped Console workflow step: %w", err)
		}
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
	if outputFormat(cmd) == "pretty" {
		registry.Register(steps.NewWaitExecutor(chainCl, newBidWaitProgressReporter(func(message string) {
			cliutil.Status(cmd, message)
		})))
	}
	if jsonl {
		// Keep stdout pure JSONL: output-step text is recorded in the step
		// result ("text" output) instead of printed.
		registry.Register(&steps.OutputExecutor{Out: io.Discard})
	}

	logger := wf.NewActionLogAdapter(cliutil.ActionLogFromContext(cmd.Context()), rtDef.Name)
	engine := wf.NewEngine(registry, logger)

	state, runErr := engine.Run(cmd.Context(), rtDef, account, params)
	if runErr == nil {
		if readinessErr := enrichDeployCompletion(cmd.Context(), state, rc, consoleClient, providerCl); readinessErr != nil {
			runErr = readinessErr
		}
	}
	recovery := deployRecoveryAdvice(state, runErr)

	// Record the outcome locally before reporting it (SPEC §6.6). A one-shot
	// CLI run exits before any chain event it produced could be observed, so
	// the run itself is the only thing that can populate the store.
	recordWorkflowOutcome(cmd, rc, state)

	var renderErr error
	if jsonl {
		renderErr = emitJSONL(out, state, recovery)
	} else {
		renderErr = printResults(out, state, runErr, recovery)
	}

	if runErr != nil {
		runErr = workflowFailureError(rtDef.Name, runErr, recovery)
		if renderErr != nil {
			return errors.Join(runErr, renderErr)
		}

		return runErr
	}

	return renderErr
}

type deploymentServiceStatus struct {
	Available int      `json:"available"`
	Total     int      `json:"total"`
	URIs      []string `json:"uris"`
}

type deploymentRuntimeStatus struct {
	Services map[string]deploymentServiceStatus `json:"services"`
}

type readinessFetcher func(context.Context) (json.RawMessage, error)

func enrichDeployCompletion(
	ctx context.Context,
	state *wf.RunState,
	rc *aktctx.Context,
	cc *console.Client,
	provider steps.ProviderClient,
) error {
	if state == nil || state.Workflow != "deploy" || rc == nil {
		return nil
	}
	readyTimeoutValue, hasReadiness := state.Params["ready-timeout"]
	if !hasReadiness {
		// A user workflow named deploy is allowed to define another lifecycle.
		return nil
	}
	created := state.Steps["create-deployment"]
	if created == nil {
		return errors.New("prepare deployment readiness: create-deployment result is missing")
	}
	dseq, err := workflowOutputUint64(created.Output, "dseq")
	if err != nil {
		return fmt.Errorf("prepare deployment readiness: %w", err)
	}
	providerAddress := workflowOutputString(state.Steps["create-lease"], "provider")
	if providerAddress == "" {
		providerAddress = workflowOutputString(state.Steps["select-bid"], "provider")
	}

	completion := map[string]any{
		"dseq":     strconv.FormatUint(dseq, 10),
		"provider": providerAddress,
		"ready":    false,
	}
	if price := state.StepOutput("select-bid", "price"); price != nil {
		completion["price"] = price
	}
	if rc.AuthMethod == aktctx.AuthMethodConsoleAPI {
		completion["auto_top_up"] = "daily"
		if cc != nil {
			if link := cc.DeploymentURL(strconv.FormatUint(dseq, 10)); link != "" {
				completion["console_url"] = link
			}
		}
	}

	noWait, _ := state.Params["no-wait-active"].(bool)
	if noWait {
		completion["readiness"] = "not-waited"
		mergeCompletionOutput(state, completion)
		state.SetStepResult("wait-for-ready", &wf.StepResult{
			Name:   "wait-for-ready",
			Type:   wf.StepWait,
			Status: "skipped",
			Output: completion,
		})
		return nil
	}

	timeoutText, ok := readyTimeoutValue.(string)
	if !ok {
		return fmt.Errorf("invalid ready-timeout %v", readyTimeoutValue)
	}
	timeout, err := time.ParseDuration(timeoutText)
	if err != nil || timeout <= 0 {
		return fmt.Errorf("invalid ready-timeout %q", timeoutText)
	}

	var fetch readinessFetcher
	if rc.AuthMethod == aktctx.AuthMethodConsoleAPI {
		if cc == nil {
			return errors.New("console readiness client is unavailable")
		}
		fetch = func(callCtx context.Context) (json.RawMessage, error) {
			detail, getErr := cc.GetDeployment(callCtx, strconv.FormatUint(dseq, 10))
			if getErr != nil {
				return nil, getErr
			}

			return consoleRuntimeStatus(detail)
		}
	} else {
		if provider == nil || providerAddress == "" {
			return errors.New("provider readiness client or selected provider is unavailable")
		}
		fetch = func(callCtx context.Context) (json.RawMessage, error) {
			return provider.LeaseStatus(callCtx, providerAddress, dseq)
		}
	}

	uris, raw, readinessErr := waitForDeploymentReadiness(ctx, timeout, fetch)
	completion["uris"] = uris
	completion["ready"] = readinessErr == nil
	mergeCompletionOutput(state, completion)
	result := &wf.StepResult{
		Name:      "wait-for-ready",
		Type:      wf.StepWait,
		Output:    completion,
		RawResult: raw,
		Status:    "success",
	}
	if readinessErr != nil {
		result.Status = "failed"
		result.Error = readinessErr.Error()
	}
	state.SetStepResult(result.Name, result)

	return readinessErr
}

func mergeCompletionOutput(state *wf.RunState, completion map[string]any) {
	result := state.Steps["display-result"]
	if result == nil {
		return
	}
	if result.Output == nil {
		result.Output = make(map[string]any)
	}
	for key, value := range completion {
		result.Output[key] = value
	}
}

func consoleRuntimeStatus(detail *console.DeploymentDetail) (json.RawMessage, error) {
	if detail == nil {
		return nil, errors.New("console deployment status is empty")
	}
	services := make(map[string]json.RawMessage)
	for _, lease := range detail.Leases {
		if lease.Status == nil || len(lease.Status.Services) == 0 {
			continue
		}
		var current map[string]json.RawMessage
		if err := json.Unmarshal(lease.Status.Services, &current); err != nil {
			return nil, fmt.Errorf("decode Console lease services: %w", err)
		}
		for name, status := range current {
			services[name] = status
		}
	}

	return json.Marshal(map[string]any{"services": services})
}

func waitForDeploymentReadiness(
	ctx context.Context,
	timeout time.Duration,
	fetch readinessFetcher,
) (map[string][]string, json.RawMessage, error) {
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		lastRaw json.RawMessage
		lastErr error
	)
	for {
		raw, err := fetch(deadlineCtx)
		if err == nil {
			lastRaw = raw
			ready, uris, decodeErr := deploymentReady(raw)
			if decodeErr == nil && ready {
				return uris, raw, nil
			}
			lastErr = decodeErr
		} else {
			lastErr = err
		}

		timer := time.NewTimer(2 * time.Second)
		select {
		case <-deadlineCtx.Done():
			timer.Stop()
			if lastErr != nil {
				return nil, lastRaw, fmt.Errorf("deployment did not become ready within %s: %w", timeout, lastErr)
			}
			return nil, lastRaw, fmt.Errorf("deployment did not become ready within %s", timeout)
		case <-timer.C:
		}
	}
}

func deploymentReady(raw json.RawMessage) (bool, map[string][]string, error) {
	var status deploymentRuntimeStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return false, nil, fmt.Errorf("decode deployment service status: %w", err)
	}
	if len(status.Services) == 0 {
		return false, nil, nil
	}

	uris := make(map[string][]string)
	for name, service := range status.Services {
		if service.Total <= 0 || service.Available < service.Total {
			return false, nil, nil
		}
		if len(service.URIs) > 0 {
			uris[name] = append([]string(nil), service.URIs...)
		}

	}

	return true, uris, nil
}

func newBidWaitProgressReporter(report func(string)) steps.WaitProgressReporter {
	lastCount := -1
	nextPeriodicReport := time.Duration(0)

	return func(progress steps.WaitProgress) {
		if progress.Query != "market.bids" || report == nil {
			return
		}

		var result struct {
			Bids []json.RawMessage `json:"bids"`
		}
		if err := json.Unmarshal(progress.Result, &result); err != nil {
			return
		}

		elapsed := progress.Elapsed.Round(time.Second)
		remaining := progress.Remaining.Round(time.Second)
		count := len(result.Bids)
		if count == lastCount && elapsed < nextPeriodicReport {
			return
		}

		lastCount = count
		nextPeriodicReport = (elapsed/(30*time.Second) + 1) * 30 * time.Second
		report(fmt.Sprintf(
			"Waiting for bids: %d received, %s elapsed, %s remaining",
			count,
			elapsed,
			remaining,
		))
	}
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

func workflowRail(mgrFn func() *aktctx.Manager, ctxNameFn func() string) transport.Kind {
	rc := resolveContext(mgrFn, ctxNameFn)
	if rc == nil {
		return ""
	}
	if rc.AuthMethod == aktctx.AuthMethodConsoleAPI {
		return transport.KindConsole
	}

	return transport.KindChain
}

// filterProviderSteps returns a copy of def without provider-type steps,
// noting each skipped step on w. Used for console-api auth, where manifest
// submission is handled by the Console API (SPEC §7.4).
func filterProviderSteps(def *wf.WorkflowDef, w io.Writer) (*wf.WorkflowDef, error) {
	kept := make([]wf.StepDef, 0, len(def.Steps))
	removed := false
	diagnostics := output.NewCheckedWriter(w)

	for _, s := range def.Steps {
		if s.Type == wf.StepProvider {
			if _, err := fmt.Fprintf(diagnostics, "note: skipping step %q (manifest submission handled by Console)\n", s.Name); err != nil {
				return nil, diagnostics.Complete(err)
			}
			removed = true
			continue
		}
		kept = append(kept, s)
	}

	if !removed {
		return def, nil
	}

	filtered := *def
	filtered.Steps = kept

	return &filtered, nil
}

// printPlan renders the workflow execution plan (name, params, steps).
func printPlan(out io.Writer, rtDef *wf.WorkflowDef, params map[string]any, rail transport.Kind) error {
	var rendered strings.Builder

	fmt.Fprintf(&rendered, "Workflow: %s (v%d)\n", rtDef.Name, rtDef.Version)
	fmt.Fprintf(&rendered, "  %s\n\n", rtDef.Description)
	if rail != "" {
		fmt.Fprintf(&rendered, "Rail: %s\n\n", rail)
	}

	fmt.Fprintln(&rendered, "Parameters:")
	for k, v := range params {
		fmt.Fprintf(&rendered, "  %-16s %v\n", k+":", v)
	}
	fmt.Fprintln(&rendered)

	fmt.Fprintf(&rendered, "Steps (%d):\n", len(rtDef.Steps))
	for i, step := range rtDef.Steps {
		fmt.Fprintf(&rendered, "  %d. [%s] %s", i+1, step.Type, step.Name)
		if rail == transport.KindConsole && step.Msg != "" {
			fmt.Fprintf(&rendered, " -> %s", consolePlanAction(step.Msg))
		} else if rail == transport.KindConsole && step.Type == wf.StepProvider {
			fmt.Fprint(&rendered, " -> handled by Console during lease creation")
		} else if step.Msg != "" {
			fmt.Fprintf(&rendered, " -> %s", step.Msg)
		}
		if step.Action != "" && (rail != transport.KindConsole || step.Type != wf.StepProvider) {
			fmt.Fprintf(&rendered, " -> %s", step.Action)
		}
		if step.OnError != "" {
			fmt.Fprintf(&rendered, " (on-error: %s)", step.OnError)
		}
		if step.Retry != nil {
			fmt.Fprintf(&rendered, " (retry: %dx, %s delay)", step.Retry.Max, step.Retry.Delay)
		}
		fmt.Fprintln(&rendered)
	}

	return writeWorkflowReport(out, rendered.String(), "plan")
}

func consolePlanAction(msg string) string {
	switch msg {
	case "deployment.MsgCreateDeployment":
		return "Console API create deployment"
	case "deployment.MsgUpdateDeployment":
		return "Console API update deployment"
	case "deployment.MsgCloseDeployment":
		return "Console API close deployment"
	case "market.MsgCreateLease":
		return "Console API create lease"
	default:
		return "unsupported Console action " + msg
	}
}

// printResults renders per-step outcomes and the overall workflow status in
// simple aligned text.
func printResults(out io.Writer, state *wf.RunState, runErr error, recovery *workflowRecovery) error {
	var rendered strings.Builder

	fmt.Fprintln(&rendered, "\nResults:")

	for _, name := range state.StepOrder {
		sr := state.Steps[name]
		if sr == nil {
			continue
		}

		fmt.Fprintf(&rendered, "  %-20s %-9s", sr.Name, sr.Status)
		if sr.TxHash != "" {
			fmt.Fprintf(&rendered, "  tx: %s", sr.TxHash)
		}
		if sr.Error != "" {
			fmt.Fprintf(&rendered, "  error: %s", sr.Error)
		}
		fmt.Fprintln(&rendered)
	}

	if runErr == nil {
		fmt.Fprintf(&rendered, "\nWorkflow %q completed successfully.\n", state.Workflow)
		renderDeployNext(&rendered, state)
	} else if recovery != nil {
		fmt.Fprintln(&rendered, "\nPartial deployment state:")
		fmt.Fprintf(&rendered, "  DSEQ: %d\n", recovery.DSeq)
		if recovery.Provider != "" {
			fmt.Fprintf(&rendered, "  Provider: %s\n", recovery.Provider)
		}
		fmt.Fprintln(&rendered, "  WARNING: This deployment remains open; escrow may continue to be consumed.")
		if recovery.Recovery != "" {
			fmt.Fprintf(&rendered, "  Recovery: %s\n", recovery.Recovery)
		}
		fmt.Fprintf(&rendered, "  Explicit cleanup: %s\n", recovery.Cleanup)
	}

	return writeWorkflowReport(out, rendered.String(), "pretty")
}

func renderDeployNext(rendered *strings.Builder, state *wf.RunState) {
	if state == nil || state.Workflow != "deploy" {
		return
	}
	result := state.Steps["display-result"]
	if result == nil || result.Output == nil {
		return
	}
	dseq := strings.TrimSpace(fmt.Sprint(result.Output["dseq"]))
	if dseq == "" || dseq == "<nil>" {
		return
	}

	fmt.Fprintln(rendered, "\nDeployment:")
	fmt.Fprintf(rendered, "  DSEQ: %s\n", dseq)
	if provider := strings.TrimSpace(fmt.Sprint(result.Output["provider"])); provider != "" && provider != "<nil>" {
		fmt.Fprintf(rendered, "  Provider: %s\n", provider)
	}
	if ready, ok := result.Output["ready"].(bool); ok {
		if ready {
			fmt.Fprintln(rendered, "  Ready: yes")
		} else if result.Output["readiness"] == "not-waited" {
			fmt.Fprintln(rendered, "  Ready: not checked")
		}
	}
	if uris, ok := result.Output["uris"].(map[string][]string); ok {
		services := make([]string, 0, len(uris))
		for service := range uris {
			services = append(services, service)
		}
		sort.Strings(services)
		for _, service := range services {
			for _, uri := range uris[service] {
				fmt.Fprintf(rendered, "  URI (%s): %s\n", service, uri)
			}
		}
	}
	if link := strings.TrimSpace(fmt.Sprint(result.Output["console_url"])); link != "" && link != "<nil>" {
		fmt.Fprintf(rendered, "  Console: %s\n", link)
	}

	fmt.Fprintln(rendered, "\nNext:")
	if _, consoleRail := result.Output["auto_top_up"]; consoleRail {
		fmt.Fprintf(rendered, "  Status:  akt console status %s\n", dseq)
		fmt.Fprintf(rendered, "  Logs:    akt console logs %s\n", dseq)
		fmt.Fprintln(rendered, "  Auto top-up: enabled daily")
		fmt.Fprintf(rendered, "  Disable: akt console deployment settings %s false\n", dseq)
	} else {
		provider := strings.TrimSpace(fmt.Sprint(result.Output["provider"]))
		if provider != "" && provider != "<nil>" {
			fmt.Fprintf(rendered, "  Status:  akt provider lease-status %s --provider %s\n", dseq, provider)
			fmt.Fprintf(rendered, "  Logs:    akt provider lease-logs %s --provider %s\n", dseq, provider)
		}
	}
	fmt.Fprintf(rendered, "  Close:   akt close %s\n", dseq)
}

type workflowRecovery struct {
	DSeq     uint64
	Provider string
	Recovery string
	Cleanup  string
}

func deployRecoveryAdvice(state *wf.RunState, runErr error) *workflowRecovery {
	if state == nil || runErr == nil || state.Workflow != "deploy" {
		return nil
	}

	created := state.Steps["create-deployment"]
	if created == nil || created.Status != "success" {
		return nil
	}

	dseq, err := workflowOutputUint64(created.Output, "dseq")
	if err != nil || dseq == 0 {
		return nil
	}

	provider := workflowOutputString(state.Steps["create-lease"], "provider")
	if provider == "" {
		provider = workflowOutputString(state.Steps["select-bid"], "provider")
	}

	recovery := &workflowRecovery{
		DSeq:     dseq,
		Provider: provider,
		Cleanup:  fmt.Sprintf("akt close %d", dseq),
	}
	manifest := state.Steps["send-manifest"]
	if provider != "" && manifest != nil && manifest.Status == "failed" {
		if sdlPath, ok := state.Params["sdl-file"].(string); ok && strings.TrimSpace(sdlPath) != "" {
			recovery.Recovery = fmt.Sprintf(
				"akt provider send-manifest %s --dseq %d --provider %s",
				shellQuote(sdlPath), dseq, provider,
			)
		}
	}

	return recovery
}

func workflowOutputUint64(output map[string]any, key string) (uint64, error) {
	if output == nil || output[key] == nil {
		return 0, fmt.Errorf("workflow output %q is missing", key)
	}

	value := strings.TrimSpace(fmt.Sprint(output[key]))
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("workflow output %q is not an unsigned integer: %w", key, err)
	}

	return parsed, nil
}

func workflowOutputString(result *wf.StepResult, key string) string {
	if result == nil || result.Output == nil || result.Output[key] == nil {
		return ""
	}

	return strings.TrimSpace(fmt.Sprint(result.Output[key]))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func workflowFailureError(workflow string, runErr error, recovery *workflowRecovery) error {
	if recovery == nil {
		return fmt.Errorf("workflow %q failed: %w", workflow, runErr)
	}

	provider := ""
	if recovery.Provider != "" {
		provider = fmt.Sprintf(" with provider %s", recovery.Provider)
	}
	retry := ""
	if recovery.Recovery != "" {
		retry = fmt.Sprintf("\nretry manifest delivery: %s", recovery.Recovery)
	}

	return fmt.Errorf(
		"workflow %q failed: %w\n"+
			"deployment DSEQ %d%s remains open; escrow may continue to be consumed.%s\n"+
			"explicit cleanup: %s",
		workflow, runErr, recovery.DSeq, provider, retry, recovery.Cleanup,
	)
}

// jsonlTx is the tx object of a JSONL step line (SPEC §2.3.8).
//
// Height is a pointer so it is omitted, not reported as 0, when the step's
// transaction has not been confirmed — which is every transaction under the
// default sync broadcast mode (SPEC §10.11.1).
type jsonlTx struct {
	Hash    string `json:"hash"`
	Height  *int64 `json:"height,omitempty"`
	GasUsed int64  `json:"gas_used,omitempty"`
	Code    uint32 `json:"code"`
}

// jsonlLine is one JSONL step line (SPEC §2.3.8).
type jsonlLine struct {
	Workflow string         `json:"workflow"`
	ID       string         `json:"id"`
	Step     string         `json:"step"`
	Result   string         `json:"result"`
	Errors   []string       `json:"errors"`
	Txs      []jsonlTx      `json:"txs"`
	Outputs  map[string]any `json:"outputs,omitempty"`
	DSeq     uint64         `json:"dseq,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Recovery string         `json:"recovery,omitempty"`
	Cleanup  string         `json:"cleanup,omitempty"`
}

// emitJSONL writes one JSONL line per completed step (SPEC §2.3.8).
func emitJSONL(out io.Writer, state *wf.RunState, recovery *workflowRecovery) error {
	enc := json.NewEncoder(completeWriter{writer: out})

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
			Outputs:  sr.Output,
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
		if line.Result == "error" && recovery != nil {
			line.DSeq = recovery.DSeq
			line.Provider = recovery.Provider
			line.Recovery = recovery.Recovery
			line.Cleanup = recovery.Cleanup
		}

		if sr.TxHash != "" {
			tx := jsonlTx{Hash: sr.TxHash}
			if sr.Height > 0 {
				height := sr.Height
				tx.Height = &height
			}
			line.Txs = append(line.Txs, tx)
		}

		if err := enc.Encode(line); err != nil {
			return fmt.Errorf("write workflow JSONL report: %w", err)
		}
	}

	return nil
}

func writeWorkflowReport(out io.Writer, report, format string) error {
	_, err := io.WriteString(completeWriter{writer: out}, report)
	if err != nil {
		return fmt.Errorf("write workflow %s report: %w", format, err)
	}

	return nil
}

type completeWriter struct {
	writer io.Writer
}

func (w completeWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}

	return written, err
}

// emitDryRunJSONL renders the validated plan without executing or discovering
// clients. Every line shares one run ID so consumers can treat it like an
// execution stream, while "planned" distinguishes it from completed work.
func emitDryRunJSONL(out io.Writer, def *wf.WorkflowDef, params map[string]any) error {
	enc := json.NewEncoder(completeWriter{writer: out})
	runID := wf.GenerateWorkflowID()

	for _, step := range def.Steps {
		line := jsonlLine{
			Workflow: def.Name,
			ID:       runID,
			Step:     step.Name,
			Result:   "planned",
			Errors:   []string{},
			Txs:      []jsonlTx{},
		}
		if step.Name == "create-deployment" {
			if deposit, ok := params[flagdefs.FlagDeposit]; ok {
				line.Outputs = map[string]any{flagdefs.FlagDeposit: deposit}
			}
		}
		if err := enc.Encode(line); err != nil {
			return fmt.Errorf("render workflow dry-run JSONL: %w", err)
		}
	}

	return nil
}

// outputFormat returns the effective --output value for the command,
// defaulting to "pretty" when the flag is not registered (e.g. bare command
// execution in tests).
func outputFormat(cmd *cobra.Command) string {
	if f := cmd.Flags().Lookup(flagdefs.FlagOutput); f != nil && f.Value.String() != "" {
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
