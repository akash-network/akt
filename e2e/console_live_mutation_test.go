package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestConsoleLiveManagedWalletLifecycle sends every mutation through the
// public akt subprocess, then verifies resulting state with a separate raw
// HTTP observer. It is separate from the read-only live suite because an API
// key alone must never authorize spending or resource creation.
//
// Required opt-in:
//
//	AKT_E2E_CONSOLE_API_KEY=<isolated managed-wallet sandbox key>
//	AKT_E2E_CONSOLE_MUTATION=I_UNDERSTAND_THIS_SPENDS_SANDBOX_FUNDS
//	AKT_E2E_CONSOLE_API_URL=<exact Akash sandbox API origin or loopback URL>
//
// Optional hard-bounded budgets:
//
//	AKT_E2E_CONSOLE_MAX_REQUEST_USD=1.00
//	AKT_E2E_CONSOLE_MAX_SPEND_USD=1.00
//	AKT_E2E_CONSOLE_MAX_DEPLOYMENTS=1
//	AKT_E2E_CONSOLE_MAX_RUNTIME=5m
func TestConsoleLiveManagedWalletLifecycle(t *testing.T) {
	optIn := os.Getenv(envConsoleMutationOptIn)
	if optIn == "" {
		t.Skipf("skipping managed-wallet mutation lifecycle: set %s=%s separately from the API key to authorize bounded sandbox writes", envConsoleMutationOptIn, consoleMutationOptInValue)
	}
	if optIn != consoleMutationOptInValue {
		t.Fatalf("%s must equal %q exactly", envConsoleMutationOptIn, consoleMutationOptInValue)
	}

	key := os.Getenv(envConsoleLiveKey)
	if key == "" {
		t.Fatalf("selected managed-wallet mutation lifecycle requires %s", envConsoleLiveKey)
	}

	config, err := loadConsoleMutationConfig(os.Getenv)
	if err != nil {
		t.Fatalf("unsafe Console mutation configuration: %v", err)
	}

	now := time.Now()
	overallDeadline := now.Add(config.MaxRuntime)
	if testDeadline, ok := t.Deadline(); ok {
		outerDeadline := testDeadline.Add(-10 * time.Second)
		if outerDeadline.Before(overallDeadline) {
			overallDeadline = outerDeadline
		}
	}
	if overallDeadline.Sub(now) < consoleCleanupReserve+30*time.Second {
		t.Fatalf("the Go test deadline leaves less than 30s of mutation time plus the %s cleanup reserve", consoleCleanupReserve)
	}
	lifecycleDeadline := overallDeadline.Add(-consoleCleanupReserve)
	lifecycleCtx, cancelLifecycle := context.WithDeadline(t.Context(), lifecycleDeadline)
	defer cancelLifecycle()

	// The secret reaches akt through its documented environment boundary. It
	// is never placed in argv, stdout, the context config, or the action log.
	t.Setenv(consoleRuntimeAPIKeyEnv, key)
	observer := newConsoleAPIObserver(config.APIURL, key)

	runID := newConsoleRunID(t)
	contextName := "console-" + runID[len(runID)-12:]
	home := t.TempDir()
	initHome(t, home)
	t.Cleanup(func() { assertConsoleSecretAbsent(t, home, contextName, key) })

	createContext := runConsoleAkt(lifecycleCtx, t, home,
		"context", "create", contextName,
		"--auth-method", "console-api",
		"--console-api-url", config.APIURL,
		"--set-current",
	)
	requireConsoleSuccess(t, createContext, "akt context create (isolated Console E2E context)")

	var identity struct {
		Username string `json:"username"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "whoami"),
		"akt console whoami",
		&identity,
	)
	if identity.Username == "" {
		t.Fatal("Console mutation prerequisite failed: authenticated tenant has no username")
	}

	var wallet struct {
		Available string `json:"available"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "wallet", "balance"),
		"akt console wallet balance",
		&wallet,
	)
	available, err := parseConsoleDollarAmount(wallet.Available)
	if err != nil {
		t.Fatalf("could not validate managed-wallet balance output: %v", err)
	}

	baselineBalances, err := observer.getBalances(lifecycleCtx)
	if err != nil {
		t.Fatalf("capture authoritative pre-mutation Console balance: %v", err)
	}
	if available+0.0051 < baselineBalances.AvailableUSD() || available-0.0051 > baselineBalances.AvailableUSD() {
		t.Fatalf("akt wallet balance disagrees with the independent Console observation")
	}
	if baselineBalances.AvailableUSD()+1e-9 < consoleLifecycleRequestUSD {
		t.Fatalf("managed-wallet sandbox prerequisite failed: available balance is below the bounded $%.2f lifecycle request total", consoleLifecycleRequestUSD)
	}

	baseline, err := observer.listAllDeployments(lifecycleCtx)
	if err != nil {
		t.Fatalf("capture pre-mutation Console state: %v", err)
	}
	for _, item := range baseline {
		if !strings.EqualFold(item.Deployment.State, "closed") {
			t.Fatalf("managed-wallet sandbox prerequisite failed: isolated tenant already has non-terminal deployment %s in state %q; sweep it before running mutations",
				item.Deployment.ID.DSeq, item.Deployment.State)
		}
	}

	// Exercise the public list command while keeping the raw HTTP observer as
	// the authority for the baseline.
	cliBaseline, err := listAllConsoleDeployments(lifecycleCtx, t, home)
	if err != nil {
		t.Fatalf("exercise akt console deployment list before mutation: %v", err)
	}
	if !sameConsoleDeploymentRecords(cliBaseline, baseline) {
		t.Fatalf("akt deployment list disagrees with the independent Console baseline")
	}
	assertConsoleActions(t, home, contextName, "")

	initialSDL := consoleLifecycleSDL(runID, "initial")
	updatedSDL := consoleLifecycleSDL(runID, "updated")
	initialHash, err := consoleSDLVersionHash(initialSDL)
	if err != nil {
		t.Fatalf("derive initial SDL version: %v", err)
	}
	updatedHash, err := consoleSDLVersionHash(updatedSDL)
	if err != nil {
		t.Fatalf("derive updated SDL version: %v", err)
	}
	sdlDir := t.TempDir()
	initialSDLPath := writeConsoleSDL(t, sdlDir, "initial.yaml", initialSDL)
	updatedSDLPath := writeConsoleSDL(t, sdlDir, "updated.yaml", updatedSDL)

	// Register cleanup before the first write. The tracker knows the unique
	// SDL hashes as well as the returned dseq, so it can recover an ambiguous
	// create without closing another runner's deployment.
	tracker := newConsoleResourceTracker(
		t,
		home,
		contextName,
		overallDeadline,
		observer,
		baseline,
		baselineBalances.Total,
		config,
		initialHash,
		updatedHash,
	)
	t.Cleanup(tracker.cleanup)
	budget := newConsoleMutationBudget(config)
	if err := budget.reserveDeployment(); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserveLease(); err != nil {
		t.Fatal(err)
	}
	// Reserve the complete immutable request plan before the first mutation.
	// Ambiguous subprocess outcomes never refund these reservations.
	if err := budget.reserveRequest(consoleCreateDepositUSD); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserveRequest(consoleAdditionalDepositUSD); err != nil {
		t.Fatal(err)
	}
	tracker.markCreateAttempted()

	var created struct {
		DSeq      consoleFlexibleID `json:"dseq"`
		TxHash    *string           `json:"txHash"`
		AutoTopUp struct {
			Enabled        *bool  `json:"enabled"`
			Frequency      string `json:"frequency"`
			DisableCommand string `json:"disableCommand"`
		} `json:"autoTopUp"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home,
			"console", "deployment", "create", initialSDLPath,
			strconv.FormatFloat(consoleCreateDepositUSD, 'f', 2, 64),
		),
		"akt console deployment create",
		&created,
	)
	dseq := created.DSeq.String()
	if parsed, err := strconv.ParseUint(dseq, 10, 64); err != nil || parsed == 0 {
		t.Fatalf("create returned invalid dseq %q", dseq)
	}
	for _, item := range baseline {
		if item.Deployment.ID.DSeq.String() == dseq {
			t.Fatalf("create returned pre-existing dseq %s", dseq)
		}
	}
	tracker.track(dseq)
	if created.AutoTopUp.Enabled == nil || !*created.AutoTopUp.Enabled {
		t.Fatal("create did not report managed-wallet auto-top-up enabled; the safety-disable path cannot be verified")
	}
	if created.TxHash != nil && strings.TrimSpace(*created.TxHash) == "" {
		t.Fatal("create acknowledgement included an empty managed-wallet transaction hash")
	}
	if created.AutoTopUp.Frequency != "daily" || created.AutoTopUp.DisableCommand != "akt console deployment settings "+dseq+" false" {
		t.Fatalf("create acknowledgement omitted the daily auto-top-up contract or exact disable command for dseq %s", dseq)
	}
	expectedActions := []string{"create-deployment"}
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	// Disable unattended spending as soon as the deployment identity exists.
	assertConsoleAutoTopUpDisabled(lifecycleCtx, t, home, dseq)
	expectedActions = append(expectedActions, "update-deployment-settings")
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)
	settingsObserveCtx, cancelSettingsObserve := context.WithTimeout(lifecycleCtx, 15*time.Second)
	if err := waitForConsoleAutoTopUpDisabled(settingsObserveCtx, observer, dseq); err != nil {
		cancelSettingsObserve()
		t.Fatalf("auto-top-up was not independently observed disabled before paid operations for dseq %s: %v", dseq, err)
	}
	cancelSettingsObserve()

	observeCtx, cancelObserve := context.WithTimeout(lifecycleCtx, 30*time.Second)
	var createdDetail consoleDeploymentObservation
	err = waitForConsoleCondition(observeCtx, 2*time.Second, func() (bool, string, error) {
		detail, err := observer.getDeployment(observeCtx, dseq)
		if err != nil {
			return false, "", err
		}
		if detail.Deployment.ID.DSeq.String() != dseq {
			return false, fmt.Sprintf("deployment get returned dseq %s", detail.Deployment.ID.DSeq), nil
		}
		if detail.Deployment.Hash != initialHash {
			return false, fmt.Sprintf("deployment hash is %q, waiting for %q", detail.Deployment.Hash, initialHash), nil
		}
		if !strings.EqualFold(detail.Deployment.State, "active") {
			return false, fmt.Sprintf("deployment state is %q", detail.Deployment.State), nil
		}

		listed, err := observer.listAllDeployments(observeCtx)
		if err != nil {
			return false, "", err
		}
		for _, item := range listed {
			if item.Deployment.ID.DSeq.String() == dseq && item.Deployment.Hash == initialHash && strings.EqualFold(item.Deployment.State, "active") {
				createdDetail = detail
				return true, "deployment independently observed by get and list", nil
			}
		}
		return false, "deployment is not yet visible in list", nil
	})
	cancelObserve()
	if err != nil {
		t.Fatalf("created deployment %s never became independently observable: %v", dseq, err)
	}
	tracker.confirm(dseq)
	if active := consoleActiveLeaseCount(createdDetail); active != 0 {
		t.Fatalf("new deployment %s had %d active leases before akt accepted a bid", dseq, active)
	}
	cliDetail, _, err := getConsoleDeployment(lifecycleCtx, t, home, dseq)
	if err != nil {
		t.Fatalf("exercise akt console deployment get for %s: %v", dseq, err)
	}
	if cliDetail.Deployment.ID.DSeq.String() != dseq || cliDetail.Deployment.Hash != initialHash || !strings.EqualFold(cliDetail.Deployment.State, "active") {
		t.Fatalf("akt deployment get disagrees with independent state for dseq %s", dseq)
	}
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	bid := waitForConsoleBid(
		lifecycleCtx,
		t,
		home,
		dseq,
		observer,
		config.MaxSpendUSD,
		time.Until(overallDeadline),
	)
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	// Deposit before accepting a lease. With no active provider consuming
	// escrow, the independent observer can prove the exact requested amount
	// rather than accepting any positive balance change.
	beforeDeposit, err := observer.getDeployment(lifecycleCtx, dseq)
	if err != nil {
		t.Fatalf("capture escrow before deposit: %v", err)
	}
	beforeFunds, err := consoleEscrowFunds(beforeDeposit)
	if err != nil {
		t.Fatal(err)
	}
	var deposited struct {
		DSeq      consoleFlexibleID `json:"dseq"`
		AmountUSD float64           `json:"amount_usd"`
		Status    string            `json:"status"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home,
			"console", "deployment", "deposit", dseq,
			strconv.FormatFloat(consoleAdditionalDepositUSD, 'f', 2, 64),
		),
		"akt console deployment deposit",
		&deposited,
	)
	if deposited.DSeq.String() != dseq || deposited.AmountUSD != consoleAdditionalDepositUSD || deposited.Status != "deposited" {
		t.Fatalf("deposit acknowledgement did not report dseq=%s amount=%.2f status=deposited", dseq, consoleAdditionalDepositUSD)
	}
	expectedActions = append(expectedActions, "deposit")
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	expectedDepositMicros := new(big.Rat).SetInt(big.NewInt(int64(math.Round(consoleAdditionalDepositUSD * 1e6))))
	depositObserveCtx, cancelDepositObserve := context.WithTimeout(lifecycleCtx, 30*time.Second)
	err = waitForConsoleCondition(depositObserveCtx, 2*time.Second, func() (bool, string, error) {
		detail, err := observer.getDeployment(depositObserveCtx, dseq)
		if err != nil {
			return false, "", err
		}
		afterFunds, err := consoleEscrowFunds(detail)
		if err != nil {
			return false, "", err
		}
		delta, ok := consoleFundsDeltaForDenom(beforeFunds, afterFunds, bid.Price.Denom)
		if !ok {
			return false, fmt.Sprintf("escrow omitted denomination %s", bid.Price.Denom), nil
		}
		if delta.Cmp(expectedDepositMicros) == 0 {
			return true, "escrow increased by the exact requested deposit", nil
		}
		return false, fmt.Sprintf("escrow delta in %s is %s, waiting for %s", bid.Price.Denom, delta, expectedDepositMicros), nil
	})
	cancelDepositObserve()
	if err != nil {
		t.Fatalf("deposit succeeded but independent escrow state did not increase by exactly %.2f USD for dseq %s: %v", consoleAdditionalDepositUSD, dseq, err)
	}

	var leased consoleDeploymentObservation
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home,
			"console", "lease", "create", dseq, bid.ID.Provider,
			"--gseq", strconv.FormatUint(uint64(bid.ID.GSeq), 10),
			"--oseq", strconv.FormatUint(uint64(bid.ID.OSeq), 10),
		),
		"akt console lease create",
		&leased,
	)
	if err := validateConsoleDeploymentObservation(leased, dseq, true, true); err != nil {
		t.Fatalf("lease-create response omitted required deployment state for dseq %s: %v", dseq, err)
	}
	if leased.Deployment.Hash != initialHash {
		t.Fatalf("lease-create response hash = %q, want %q", leased.Deployment.Hash, initialHash)
	}
	if !hasExactActiveLease(leased, bid.ID) {
		t.Fatalf("lease-create response did not contain the selected active lease dseq=%s gseq=%d oseq=%d provider=%s",
			bid.ID.DSeq, bid.ID.GSeq, bid.ID.OSeq, bid.ID.Provider)
	}
	expectedActions = append(expectedActions, "create-lease")
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	leaseObserveCtx, cancelLeaseObserve := context.WithTimeout(lifecycleCtx, 30*time.Second)
	err = waitForConsoleCondition(leaseObserveCtx, 2*time.Second, func() (bool, string, error) {
		detail, err := observer.getDeployment(leaseObserveCtx, dseq)
		if err != nil {
			return false, "", err
		}
		if hasExactActiveLease(detail, bid.ID) {
			leased = detail
			return true, "selected lease independently observed through deployment get", nil
		}
		return false, "selected lease is not active yet", nil
	})
	cancelLeaseObserve()
	if err != nil {
		t.Fatalf("lease phase blocked for dseq %s: the sandbox provider %s must expose active lease %s/%d/%d: %v",
			dseq, bid.ID.Provider, bid.ID.DSeq, bid.ID.GSeq, bid.ID.OSeq, err)
	}
	if active := consoleActiveLeaseCount(leased); active != consoleLifecycleMaxLeases {
		t.Fatalf("deployment %s has %d active leases after one authorized lease request, want %d", dseq, active, consoleLifecycleMaxLeases)
	}

	providerActions, err := readProviderActions(home, contextName)
	if err != nil {
		t.Fatalf("read provider action log before gateway queries: %v", err)
	}
	if len(providerActions) != 0 {
		t.Fatalf("read-only setup unexpectedly recorded %d provider actions", len(providerActions))
	}

	var workloadURIs []string
	statusPassed := t.Run("provider status", func(t *testing.T) {
		statusCtx, cancelStatus := context.WithTimeout(lifecycleCtx, 30*time.Second)
		defer cancelStatus()

		err := waitForConsoleCondition(statusCtx, 2*time.Second, func() (bool, string, error) {
			result := runConsoleAkt(statusCtx, t, home, "console", "status", dseq)
			if result.Exit != 0 || result.Err != nil || result.CredentialLeak {
				return false, "", errors.New(consoleCommandDiagnostic(result))
			}
			if result.StdoutTruncated || result.StderrTruncated {
				return false, "", fmt.Errorf("provider status exceeded bounded capture (%s)", consoleCommandDiagnostic(result))
			}
			trimmed := strings.TrimSpace(result.Stdout)
			var status struct {
				Services map[string]struct {
					Name      string   `json:"name"`
					Available int32    `json:"available"`
					Total     int32    `json:"total"`
					URIs      []string `json:"uris"`
				} `json:"services"`
			}
			if err := decodeConsoleJSONDocument([]byte(trimmed), &status); err != nil {
				return false, fmt.Sprintf("provider status returned invalid JSON (%d bytes): %v", result.StdoutBytes, err), nil
			}
			service, ok := status.Services["web"]
			if !ok || service.Name != "web" || service.Total < 1 || service.Available < 1 || len(service.URIs) == 0 {
				return false, "provider status has not reported one available web replica", nil
			}
			if err := validateConsoleLiveJSONContract(
				"provider status response",
				json.RawMessage(trimmed),
				consoleProviderStatusContract(),
			); err != nil {
				return false, "", fmt.Errorf("provider status contract is incomplete: %w", err)
			}
			workloadURIs = append(workloadURIs[:0], service.URIs...)
			return true, "provider status observed an available web replica", nil
		})
		if err != nil {
			t.Fatalf("provider-status phase failed for dseq %s: selected sandbox provider %s must publish a gateway hostUri, accept Console-minted JWT scope=status, and report the web workload ready: %v", dseq, bid.ID.Provider, err)
		}
	})
	if !statusPassed {
		t.FailNow()
	}
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	ingressPassed := t.Run("workload ingress", func(t *testing.T) {
		ingressCtx, cancelIngress := context.WithTimeout(lifecycleCtx, 30*time.Second)
		defer cancelIngress()

		var lastErr error
		for _, uri := range workloadURIs {
			err := probeConsoleWorkloadIngress(ingressCtx, uri)
			if err == nil {
				return
			}
			lastErr = err
		}
		t.Fatalf("none of the %d provider-reported workload URIs returned a bounded non-empty 2xx response: %v", len(workloadURIs), lastErr)
	})
	if !ingressPassed {
		t.FailNow()
	}

	logsPassed := t.Run("provider logs", func(t *testing.T) {
		logsCtx, cancelLogs := context.WithTimeout(lifecycleCtx, 30*time.Second)
		defer cancelLogs()

		result := runConsoleAkt(logsCtx, t, home, "console", "logs", dseq, "web", "--tail", "100")
		requireConsoleSuccess(t, result, "akt console logs")
		count, err := validateConsoleLogStream([]byte(result.Stdout), "web")
		if err != nil {
			t.Fatalf("akt console logs returned an invalid JSON stream: %v", err)
		}
		if count > 100 {
			t.Fatalf("akt console logs returned %d records, want at most 100", count)
		}
	})
	if !logsPassed {
		t.FailNow()
	}

	eventsPassed := t.Run("provider events", func(t *testing.T) {
		eventsCtx, cancelEvents := context.WithTimeout(lifecycleCtx, 30*time.Second)
		defer cancelEvents()

		result := runConsoleAkt(eventsCtx, t, home, "console", "events", dseq)
		requireConsoleSuccess(t, result, "akt console events")
		count, err := decodeConsoleJSONStream([]byte(result.Stdout), func(raw json.RawMessage) error {
			var record struct {
				Type   string `json:"type"`
				Reason string `json:"reason"`
				Object struct {
					Kind string `json:"kind"`
					Name string `json:"name"`
				} `json:"object"`
			}
			if err := json.Unmarshal(raw, &record); err != nil {
				return err
			}
			if strings.TrimSpace(record.Type) == "" || strings.TrimSpace(record.Reason) == "" ||
				strings.TrimSpace(record.Object.Kind) == "" || strings.TrimSpace(record.Object.Name) == "" {
				return errors.New("event identity is incomplete")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("akt console events returned an invalid JSON stream: %v", err)
		}
		if count == 0 {
			t.Fatal("akt console events returned no workload events")
		}
	})
	if !eventsPassed {
		t.FailNow()
	}
	readsPassed := t.Run("deployment read contracts", func(t *testing.T) {
		exerciseConsoleLiveDeploymentReads(lifecycleCtx, t, home, dseq)
	})
	if !readsPassed {
		t.FailNow()
	}
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	providerActions, err = readProviderActions(home, contextName)
	if err != nil {
		t.Fatalf("read provider action log after gateway queries: %v", err)
	}
	if len(providerActions) != 0 {
		t.Fatalf("read-only deployment/status/log/event calls recorded %d provider actions", len(providerActions))
	}

	shellPassed := t.Run("provider shell", func(t *testing.T) {
		shellCtx, cancelShell := context.WithTimeout(lifecycleCtx, 30*time.Second)
		defer cancelShell()

		result := runConsoleAkt(shellCtx, t, home,
			"console", "shell", dseq, "web", "--",
			"/bin/sh", "-c", `printf '%s\n' "$AKT_E2E_RUN_ID:$AKT_E2E_PHASE"`,
		)
		var shell struct {
			Stdout string `json:"stdout"`
			Stderr string `json:"stderr"`
		}
		requireConsoleJSON(t, result, "akt console shell", &shell)
		if shell.Stdout != runID+":initial\n" || shell.Stderr != "" {
			t.Fatalf("akt console shell returned stdout/stderr lengths %d/%d, want exact run identity and empty stderr", len(shell.Stdout), len(shell.Stderr))
		}
	})
	if !shellPassed {
		t.FailNow()
	}

	providerActions, err = readProviderActions(home, contextName)
	if err != nil {
		t.Fatalf("read provider action log after shell: %v", err)
	}
	dseqNumber, err := strconv.ParseUint(dseq, 10, 64)
	if err != nil {
		t.Fatalf("parse tracked dseq for provider action assertion: %v", err)
	}
	if len(providerActions) != 1 || providerActions[0].Action != "lease-shell" || providerActions[0].Status != "success" ||
		providerActions[0].Provider != bid.ID.Provider || providerActions[0].DSeq != dseqNumber || providerActions[0].Error != "" {
		t.Fatalf("provider action observation does not prove the exact successful shell operation: %+v", providerActions)
	}
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	// Exercise the existing-settings update path without ever re-enabling
	// unattended top-up.
	assertConsoleAutoTopUpDisabled(lifecycleCtx, t, home, dseq)
	expectedActions = append(expectedActions, "update-deployment-settings")
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)
	settingsObserveCtx, cancelSettingsObserve = context.WithTimeout(lifecycleCtx, 15*time.Second)
	if err := waitForConsoleAutoTopUpDisabled(settingsObserveCtx, observer, dseq); err != nil {
		cancelSettingsObserve()
		t.Fatalf("updated auto-top-up setting was not independently observable for dseq %s: %v", dseq, err)
	}
	cancelSettingsObserve()
	assertConsoleAutoTopUpRead(lifecycleCtx, t, home, dseq)
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	var updated consoleDeploymentObservation
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "deployment", "update", dseq, updatedSDLPath),
		"akt console deployment update",
		&updated,
	)
	if err := validateConsoleDeploymentObservation(updated, dseq, true, true); err != nil {
		t.Fatalf("update response omitted required deployment state for dseq %s: %v", dseq, err)
	}
	if updated.Deployment.Hash != updatedHash {
		t.Fatalf("update response hash = %q, want %q", updated.Deployment.Hash, updatedHash)
	}
	expectedActions = append(expectedActions, "update-deployment")
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	updateObserveCtx, cancelUpdateObserve := context.WithTimeout(lifecycleCtx, 30*time.Second)
	err = waitForConsoleCondition(updateObserveCtx, 2*time.Second, func() (bool, string, error) {
		detail, err := observer.getDeployment(updateObserveCtx, dseq)
		if err != nil {
			return false, "", err
		}
		if detail.Deployment.Hash == updatedHash {
			return true, "updated SDL hash independently observed", nil
		}
		return false, fmt.Sprintf("deployment hash is %q, waiting for %q", detail.Deployment.Hash, updatedHash), nil
	})
	cancelUpdateObserve()
	if err != nil {
		t.Fatalf("update succeeded but the Console API never exposed the updated SDL hash for dseq %s: %v", dseq, err)
	}
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	var closed struct {
		DSeq          consoleFlexibleID `json:"dseq"`
		State         string            `json:"state"`
		AlreadyClosed *bool             `json:"already_closed"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "deployment", "close", dseq),
		"akt console deployment close",
		&closed,
	)
	if closed.DSeq.String() != dseq || closed.State != "closed" || closed.AlreadyClosed == nil || *closed.AlreadyClosed {
		t.Fatalf("first close acknowledgement did not report dseq=%s state=closed already_closed=false", dseq)
	}
	expectedActions = append(expectedActions, "close-deployment")
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	terminalCtx, cancelTerminal := context.WithTimeout(lifecycleCtx, 30*time.Second)
	if err := waitForConsoleTerminalState(terminalCtx, observer, dseq); err != nil {
		cancelTerminal()
		t.Fatalf("close succeeded but terminal state was not independently observable for dseq %s: %v", dseq, err)
	}
	cancelTerminal()
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	// The public command promises idempotent close. Exercise the terminal
	// transition again and require another successful mutation audit record.
	var closedAgain struct {
		DSeq          consoleFlexibleID `json:"dseq"`
		State         string            `json:"state"`
		AlreadyClosed *bool             `json:"already_closed"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "deployment", "close", dseq),
		"akt console deployment close (idempotent repeat)",
		&closedAgain,
	)
	if closedAgain.DSeq.String() != dseq || closedAgain.State != "closed" || closedAgain.AlreadyClosed == nil {
		t.Fatalf("repeated close acknowledgement did not report dseq=%s state=closed with an already_closed boolean", dseq)
	}
	expectedActions = append(expectedActions, "close-deployment")
	assertConsoleActions(t, home, contextName, dseq, expectedActions...)

	// Console intentionally returns the same success envelope when its backend
	// observes an already-closed deployment and performs no transaction. The
	// CLI therefore cannot infer prior state from a 200 response; the second
	// successful command plus the action log and independent terminal-state
	// observation below are the idempotency proof.
	if err := waitForConsoleTerminalState(lifecycleCtx, observer, dseq); err != nil {
		t.Fatalf("repeated close lost terminal state for dseq %s: %v", dseq, err)
	}

	apiKeyBaseline, err := observer.listAPIKeys(lifecycleCtx)
	if err != nil {
		t.Fatalf("capture independent API-key baseline: %v", err)
	}
	if cliKeys := listConsoleAPIKeys(lifecycleCtx, t, home); !sameConsoleAPIKeyRecords(cliKeys, apiKeyBaseline) {
		t.Fatal("akt console apikey list disagrees with the independent baseline")
	}

	childName := runID + "-child"
	var childKey struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		APIKey  string `json:"apiKey"`
		Warning string `json:"warning"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "apikey", "create", childName),
		"akt console apikey create",
		&childKey,
	)
	childDeleted := false
	if strings.TrimSpace(childKey.ID) != "" {
		t.Cleanup(func() {
			if childDeleted {
				return
			}
			cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancelCleanup()
			result := runConsoleAkt(cleanupCtx, t, home, "console", "apikey", "delete", childKey.ID)
			if result.Exit != 0 || result.Err != nil || result.CredentialLeak {
				t.Errorf("cleanup child API key %s failed (%s)", childKey.ID, consoleCommandDiagnostic(result))
			}
		})
	}
	if strings.TrimSpace(childKey.ID) == "" || childKey.Name != childName || strings.TrimSpace(childKey.APIKey) == "" || childKey.APIKey == key ||
		childKey.Warning != "Store this key now. It will not be shown again." {
		t.Fatal("API-key creation did not return the exact requested identity, a distinct one-time secret, and its warning")
	}
	if _, existed := findConsoleAPIKey(apiKeyBaseline, childKey.ID); existed {
		t.Fatalf("API-key creation returned pre-existing ID %s", childKey.ID)
	}
	childSecret := childKey.APIKey
	t.Cleanup(func() { assertConsoleSecretAbsent(t, home, contextName, childSecret) })
	assertConsoleActionTail(t, home, contextName, len(expectedActions), "create-api-key")

	keyObserveCtx, cancelKeyObserve := context.WithTimeout(lifecycleCtx, 15*time.Second)
	var observedKeys []consoleAPIKeyObservation
	err = waitForConsoleCondition(keyObserveCtx, time.Second, func() (bool, string, error) {
		keys, err := observer.listAPIKeys(keyObserveCtx)
		if err != nil {
			return false, "", err
		}
		observed, found := findConsoleAPIKey(keys, childKey.ID)
		if !found || observed.Name != childName {
			return false, "created child API key is not listed yet", nil
		}
		if len(keys) != len(apiKeyBaseline)+1 || !sameConsoleAPIKeyRecords(consoleAPIKeysWithoutID(keys, childKey.ID), apiKeyBaseline) {
			return false, "API-key collection changed by more than the exact child identity", nil
		}
		observedKeys = keys
		return true, "exact child API key independently observed", nil
	})
	cancelKeyObserve()
	if err != nil {
		t.Fatalf("created child API key was not independently observable: %v", err)
	}
	if cliKeys := listConsoleAPIKeys(lifecycleCtx, t, home); !sameConsoleAPIKeyRecords(cliKeys, observedKeys) {
		t.Fatal("akt console apikey list disagrees with the independent post-create collection")
	}

	childObserver := newConsoleAPIObserver(config.APIURL, childSecret)
	childUsername, err := childObserver.username(lifecycleCtx)
	if err != nil || childUsername != identity.Username {
		t.Fatalf("created child API key did not authenticate the exact tenant through the independent observer: username_match=%t error=%v", childUsername == identity.Username, err)
	}
	t.Setenv(consoleRuntimeAPIKeyEnv, childSecret)
	var childIdentity struct {
		Username string `json:"username"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "whoami"),
		"akt console whoami with child API key",
		&childIdentity,
	)
	if childIdentity.Username != identity.Username {
		t.Fatal("created child API key authenticated as a different Console tenant")
	}
	t.Setenv(consoleRuntimeAPIKeyEnv, key)

	var deletedKey struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "apikey", "delete", childKey.ID),
		"akt console apikey delete",
		&deletedKey,
	)
	if deletedKey.ID != childKey.ID || !deletedKey.Deleted {
		t.Fatalf("API-key delete acknowledgement = id %q deleted %t, want exact ID and true", deletedKey.ID, deletedKey.Deleted)
	}
	assertConsoleActionTail(t, home, contextName, len(expectedActions), "create-api-key", "delete-api-key")

	keyDeleteCtx, cancelKeyDelete := context.WithTimeout(lifecycleCtx, 15*time.Second)
	err = waitForConsoleCondition(keyDeleteCtx, time.Second, func() (bool, string, error) {
		keys, err := observer.listAPIKeys(keyDeleteCtx)
		if err != nil {
			return false, "", err
		}
		if _, found := findConsoleAPIKey(keys, childKey.ID); found {
			return false, "deleted child API key is still listed", nil
		}
		if !sameConsoleAPIKeyRecords(keys, apiKeyBaseline) {
			return false, "API-key collection did not return to its exact baseline", nil
		}
		return true, "child API key independently absent", nil
	})
	cancelKeyDelete()
	if err != nil {
		t.Fatalf("deleted child API key remained independently observable: %v", err)
	}
	childDeleted = true
	if cliKeys := listConsoleAPIKeys(lifecycleCtx, t, home); !sameConsoleAPIKeyRecords(cliKeys, apiKeyBaseline) {
		t.Fatal("akt console apikey list did not return to the independent baseline")
	}

	var deletedAgain struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(lifecycleCtx, t, home, "console", "apikey", "delete", childKey.ID),
		"akt console apikey delete (idempotent repeat)",
		&deletedAgain,
	)
	if deletedAgain.ID != childKey.ID || !deletedAgain.Deleted {
		t.Fatalf("repeated API-key delete acknowledgement = id %q deleted %t, want exact ID and true", deletedAgain.ID, deletedAgain.Deleted)
	}
	assertConsoleActionTail(t, home, contextName, len(expectedActions), "create-api-key", "delete-api-key", "delete-api-key")

	if username, err := childObserver.username(lifecycleCtx); err == nil || username != "" {
		t.Fatalf("revoked child API key remained usable through the independent observer: username_present=%t error=%v", username != "", err)
	}
	t.Setenv(consoleRuntimeAPIKeyEnv, childSecret)
	revokedResult := runConsoleAkt(lifecycleCtx, t, home, "console", "whoami")
	if revokedResult.Exit == 0 || revokedResult.Err == nil || revokedResult.CredentialLeak {
		t.Fatalf("revoked child API key did not fail closed without disclosure (%s)", consoleCommandDiagnostic(revokedResult))
	}
	t.Setenv(consoleRuntimeAPIKeyEnv, key)
	if username, err := observer.username(lifecycleCtx); err != nil || username != identity.Username {
		t.Fatalf("parent API key stopped authenticating after child revocation: username_match=%t error=%v", username == identity.Username, err)
	}
	assertConsoleSecretAbsent(t, home, contextName, childSecret)
	assertConsoleSecretAbsent(t, home, contextName, key)

	t.Logf("completed real Console managed-wallet lifecycle run_id=%s dseq=%s requested_usd=%.2f",
		runID, dseq, budget.requestedUSD)
}

func parseConsoleDollarAmount(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "$") {
		return 0, fmt.Errorf("expected $-prefixed amount")
	}
	amount, err := strconv.ParseFloat(strings.TrimPrefix(value, "$"), 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return 0, fmt.Errorf("expected a finite non-negative dollar amount")
	}
	return amount, nil
}

func sameConsoleDeploymentRecords(left, right []consoleDeploymentObservation) bool {
	leftRecords := consoleDeploymentRecordFingerprints(left)
	rightRecords := consoleDeploymentRecordFingerprints(right)
	if len(leftRecords) != len(rightRecords) {
		return false
	}
	for index := range leftRecords {
		if leftRecords[index] != rightRecords[index] {
			return false
		}
	}
	return true
}

func consoleDeploymentRecordFingerprints(deployments []consoleDeploymentObservation) []string {
	records := make([]string, 0, len(deployments))
	for _, item := range deployments {
		leases := make([]string, 0, len(item.Leases))
		for _, lease := range item.Leases {
			denom := ""
			amount := ""
			if lease.Price != nil {
				denom = lease.Price.Denom
				amount = lease.Price.Amount.String()
			}
			leases = append(leases, fmt.Sprintf(
				"%q/%q/%d/%d/%q/%q/%q/%q",
				lease.ID.Owner,
				lease.ID.DSeq.String(),
				lease.ID.GSeq,
				lease.ID.OSeq,
				lease.ID.Provider,
				lease.State,
				denom,
				amount,
			))
		}
		sort.Strings(leases)
		records = append(records, fmt.Sprintf(
			"%q/%q/%q/%q/%q",
			item.Deployment.ID.Owner,
			item.Deployment.ID.DSeq.String(),
			item.Deployment.State,
			item.Deployment.Hash,
			strings.Join(leases, "|"),
		))
	}
	sort.Strings(records)
	return records
}

func waitForConsoleAutoTopUpDisabled(ctx context.Context, observer *consoleAPIObserver, dseq string) error {
	return waitForConsoleCondition(ctx, time.Second, func() (bool, string, error) {
		settings, err := observer.getDeploymentSettings(ctx, dseq)
		if err != nil {
			return false, "", err
		}
		if settings.DSeq.String() != dseq {
			return false, fmt.Sprintf("settings returned dseq %s", settings.DSeq), nil
		}
		if settings.AutoTopUpEnabled {
			return false, "auto-top-up is still enabled", nil
		}
		return true, "auto-top-up is disabled", nil
	})
}

func assertConsoleAutoTopUpDisabled(ctx context.Context, t *testing.T, home, dseq string) {
	t.Helper()
	var settings struct {
		DSeq             consoleFlexibleID `json:"dseq"`
		AutoTopUpEnabled *bool             `json:"autoTopUpEnabled"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(ctx, t, home, "console", "deployment", "settings", dseq, "false"),
		"akt console deployment settings <dseq> false",
		&settings,
	)
	if settings.DSeq.String() != dseq || settings.AutoTopUpEnabled == nil || *settings.AutoTopUpEnabled {
		t.Fatalf("deployment settings did not report dseq=%s with auto-top-up disabled", dseq)
	}
}

func assertConsoleAutoTopUpRead(ctx context.Context, t *testing.T, home, dseq string) {
	t.Helper()
	var settings struct {
		DSeq             consoleFlexibleID `json:"dseq"`
		AutoTopUpEnabled *bool             `json:"autoTopUpEnabled"`
	}
	requireConsoleJSON(t,
		runConsoleAkt(ctx, t, home, "console", "deployment", "settings", dseq),
		"akt console deployment settings <dseq>",
		&settings,
	)
	if settings.DSeq.String() != dseq || settings.AutoTopUpEnabled == nil || *settings.AutoTopUpEnabled {
		t.Fatalf("read-back deployment settings did not report dseq=%s with auto-top-up disabled", dseq)
	}
}

func waitForConsoleBid(
	parent context.Context,
	t *testing.T,
	home string,
	dseq string,
	observer *consoleAPIObserver,
	maxSpendUSD float64,
	paidRuntime time.Duration,
) consoleBidObservation {
	t.Helper()

	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()

	var selected consoleBidObservation
	err := waitForConsoleCondition(ctx, 3*time.Second, func() (bool, string, error) {
		result := runConsoleAkt(ctx, t, home, "console", "bid", "list", dseq)
		if result.Exit != 0 || result.Err != nil || result.CredentialLeak {
			return false, "", errors.New(consoleCommandDiagnostic(result))
		}
		if result.StdoutTruncated || result.StderrTruncated {
			return false, "", fmt.Errorf("bid list exceeded bounded capture (%s)", consoleCommandDiagnostic(result))
		}

		trimmed := strings.TrimSpace(result.Stdout)
		if strings.Contains(strings.ToLower(trimmed), "no bids yet") {
			return false, "Console reports no bids yet", nil
		}
		var bids []consoleBidObservation
		if err := decodeConsoleJSONDocument([]byte(trimmed), &bids); err != nil {
			return false, "", fmt.Errorf("decode bid list: %w", err)
		}

		directBids, err := observer.listBids(ctx, dseq)
		if err != nil {
			return false, "", err
		}
		candidate, projected, ok := selectConsoleBudgetSafeBid(bids, directBids, dseq, maxSpendUSD, paidRuntime)
		if ok {
			selected = candidate
			return true, fmt.Sprintf(
				"lowest corroborated provider bid projects to %s USD within the %.2f USD ceiling",
				projected.FloatString(6),
				maxSpendUSD,
			), nil
		}
		return false, fmt.Sprintf(
			"%d CLI bids and %d independent bids yielded no corroborated uact bid within the %.2f USD ceiling",
			len(bids),
			len(directBids),
			maxSpendUSD,
		), nil
	})
	if err != nil {
		t.Fatalf("bid phase blocked for dseq %s: the configured sandbox needs at least one provider bidding on the test SDL within 90s: %v", dseq, err)
	}
	return selected
}

func sameConsoleBid(left, right consoleBidObservation) bool {
	return left.ID.Owner == right.ID.Owner &&
		left.ID.DSeq.String() == right.ID.DSeq.String() &&
		left.ID.GSeq == right.ID.GSeq &&
		left.ID.OSeq == right.ID.OSeq &&
		left.ID.Provider == right.ID.Provider &&
		left.Price != nil && right.Price != nil &&
		left.Price.Denom == right.Price.Denom &&
		left.Price.Amount.String() == right.Price.Amount.String() &&
		validConsolePrice(right.Price) &&
		(strings.EqualFold(right.State, "open") || strings.EqualFold(right.State, "active"))
}

func hasExactActiveLease(detail consoleDeploymentObservation, want consoleLeaseID) bool {
	for _, lease := range detail.Leases {
		if lease.ID.Owner == want.Owner &&
			lease.ID.DSeq.String() == want.DSeq.String() &&
			lease.ID.GSeq == want.GSeq &&
			lease.ID.OSeq == want.OSeq &&
			lease.ID.Provider == want.Provider &&
			validConsolePrice(lease.Price) &&
			strings.EqualFold(lease.State, "active") {
			return true
		}
	}
	return false
}

func assertConsoleSecretAbsent(t *testing.T, home, contextName, secret string) {
	t.Helper()
	credentialFound := errors.New("credential found")
	foundPath := ""
	err := filepath.WalkDir(home, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		scan := consoleCappedBuffer{needle: []byte(secret)}
		_, copyErr := io.Copy(&scan, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if scan.containsNeedle {
			foundPath = path
			return credentialFound
		}
		return nil
	})
	if errors.Is(err, credentialFound) {
		t.Fatalf("Console API key leaked into %s", foundPath)
	}
	if err != nil {
		t.Fatalf("scan isolated akt home for credential leakage: %v", err)
	}
	credentialPath := filepath.Join(home, "contexts", contextName, "console-api-key")
	if _, err := os.Stat(credentialPath); err == nil {
		t.Fatalf("mutation suite persisted the environment-injected API key at %s", credentialPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect Console credential path: %v", err)
	}
}
