# Phase 1 Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the 5 remaining Phase 1 (User Story 1) items: tx result pretty formatters, tx formatter tests, shell completion command, bootstrap wizard keyring selection, and offline E2E tests.

**Architecture:** Wire `pretty.PrintTxResult` into all 63 tx command call sites (replacing `cl.PrintMessage`), register ~60 per-message `TxPrettyFormatter` implementations via an explicit `RegisterAllTxFormatters()` function, add standard cobra completion subcommand, extend the bootstrap wizard with keyring backend selection, and create an offline E2E test suite for context/network/key management.

**Tech Stack:** Go, Cobra, Lipgloss, Cosmos SDK protobuf types, Akash chain-sdk types, charmbracelet/golden (test snapshots)

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `internal/output/pretty/tx_formatters.go` | All ~60 `TxPrettyFormatter` registrations + `RegisterAllTxFormatters()` |
| `internal/output/pretty/tx_test.go` | Tests for `PrintTxResult` dispatch and per-message formatters |
| `internal/output/pretty/testdata/Test*.golden` | Golden files for tx formatter tests (auto-generated on first run with `-update`) |
| `e2e/cli_test.go` | Offline E2E tests for context, network, keys, completion |
| `e2e/helpers_test.go` | E2E test helpers (run akt binary, temp home, assertions) |

### Modified files

| File | Change |
|------|--------|
| `internal/cli/chain/flags/flags.go:359` | Change `--output` default from `"json"` to `OutputPretty` for tx commands |
| `internal/cli/chain/*_tx.go` (20 files, 63 sites) | Replace `cl.PrintMessage(resp)` with `pretty.PrintTxResult(cmd, cl.ClientContext(), resp)` |
| `internal/cli/chain/auth_tips.go:73` | Same replacement (uses `res` not `resp`) |
| `internal/cli/root.go` | Add `completionCmd()` subcommand + call `pretty.RegisterAllTxFormatters()` |
| `internal/bootstrap/bootstrap.go` | Add keyring backend selection step, fix `Defaults.Output` from `"table"` to `"pretty"` |
| `Makefile` | Add `test` and `test-e2e` targets |
| `AICHANGELOG.md` | Changelog entries for all changes |

---

## Task 1: Wire PrintTxResult into tx commands

**Files:**
- Modify: `internal/cli/chain/flags/flags.go:359`
- Modify: `internal/cli/chain/bank_tx.go`, `deployment_tx.go`, `market_tx.go`, `staking_tx.go`, `distribution_tx.go`, `gov_tx.go`, `authz_tx.go`, `feegrant_tx.go`, `cert_tx.go`, `audit_tx.go`, `provider_tx.go`, `escrow_tx.go`, `slashing_tx.go`, `vesting_tx.go`, `upgrade_tx.go` (not present but check), `crisis_tx.go`, `wasm_tx.go`, `oracle_tx.go`, `bme_tx.go`, `params_tx.go`, `auth_tips.go`

- [ ] **Step 1: Fix the output flag default for tx commands**

In `internal/cli/chain/flags/flags.go`, change line 359 from:
```go
f.StringP(FlagOutput, "o", "json", "Output format (text|json)")
```
to:
```go
f.StringP(FlagOutput, "o", OutputPretty, "Output format (pretty|json|yaml)")
```

- [ ] **Step 2: Replace all `cl.PrintMessage(resp)` calls with `pretty.PrintTxResult`**

In every `*_tx.go` file under `internal/cli/chain/`, replace:
```go
return cl.PrintMessage(resp)
```
with:
```go
return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
```

And in `auth_tips.go` line 73, replace:
```go
return cl.PrintMessage(res)
```
with:
```go
return pretty.PrintTxResult(cmd, cl.ClientContext(), res)
```

Each file needs the import added:
```go
"pkg.akt.dev/akt/internal/output/pretty"
```

There are 63 call sites across 21 files. All follow the same mechanical pattern.

- [ ] **Step 3: Verify the project compiles**

Run: `make akt`
Expected: Clean build with no errors.

- [ ] **Step 4: Run existing tests**

Run: `go test ./internal/output/pretty/... ./internal/cli/...`
Expected: All existing tests pass. The tx commands now route through `PrintTxResult`, which falls back to highlighted JSON for unregistered message types (no formatters registered yet).

---

## Task 2: Implement tx result pretty formatters

**Files:**
- Create: `internal/output/pretty/tx_formatters.go`
- Modify: `internal/cli/root.go` (call `RegisterAllTxFormatters`)

- [ ] **Step 1: Create tx_formatters.go with RegisterAllTxFormatters()**

Create `internal/output/pretty/tx_formatters.go`. This file contains:
1. `RegisterAllTxFormatters()` — public function that registers all formatters
2. Per-module formatter functions, each following the same pattern

The formatters use `KV()`, `FormatCoin()`, `FormatCoins()`, `Bold()`, `Section()` from `helpers.go`.

Each formatter is registered via:
```go
RegisterTx((*msgType)(nil), TxPrettyFormatterFunc{
    TitleStr: "Title",
    FormatFn: formatFuncName,
})
```

The complete file implements formatters for all message types specified in SPEC.md §10.11.5:

**Bank** (2): MsgSend ("Send"), MsgMultiSend ("Multi Send")
**Deployment** (6): MsgCreateDeployment ("Deployment Created"), MsgUpdateDeployment ("Deployment Updated"), MsgCloseDeployment ("Deployment Closed"), MsgCloseGroup ("Group Closed"), MsgPauseGroup ("Group Paused"), MsgStartGroup ("Group Started")
**Market** (5): MsgCreateBid ("Bid Created"), MsgCloseBid ("Bid Closed"), MsgCreateLease ("Lease Created"), MsgWithdrawLease ("Lease Withdrawn"), MsgCloseLease ("Lease Closed")
**Provider** (2): MsgCreateProvider ("Provider Created"), MsgUpdateProvider ("Provider Updated")
**Cert** (2): MsgCreateCertificate ("Certificate Published"), MsgRevokeCertificate ("Certificate Revoked")
**Audit** (2): MsgSignProviderAttributes ("Attributes Signed"), MsgDeleteProviderAttributes ("Attributes Deleted")
**Staking** (6): MsgCreateValidator ("Validator Created"), MsgEditValidator ("Validator Edited"), MsgDelegate ("Delegate"), MsgBeginRedelegate ("Redelegate"), MsgUndelegate ("Undelegate"), MsgCancelUnbondingDelegation ("Cancel Unbonding")
**Distribution** (5): MsgWithdrawDelegatorReward ("Withdraw Rewards"), MsgWithdrawValidatorCommission ("Withdraw Commission"), MsgSetWithdrawAddress ("Set Withdraw Address"), MsgFundCommunityPool ("Fund Community Pool"), MsgDepositValidatorRewardsPool ("Fund Validator Rewards")
**Gov** (4): MsgSubmitProposal ("Proposal Submitted"), MsgDeposit ("Proposal Deposit"), MsgVote ("Vote"), MsgVoteWeighted ("Weighted Vote")
**Authz** (3): MsgGrant ("Authorization Granted"), MsgRevoke ("Authorization Revoked"), MsgExec ("Authz Exec")
**Feegrant** (2): MsgGrantAllowance ("Fee Allowance Granted"), MsgRevokeAllowance ("Fee Allowance Revoked")
**Escrow** (1): MsgAccountDeposit ("Escrow Deposit")
**Slashing** (1): MsgUnjail ("Unjail")
**Vesting** (3): MsgCreateVestingAccount ("Vesting Account Created"), MsgCreatePermanentLockedAccount ("Permanent Locked Account Created"), MsgCreatePeriodicVestingAccount ("Periodic Vesting Account Created")
**Upgrade** (2): MsgSoftwareUpgrade ("Software Upgrade"), MsgCancelUpgrade ("Upgrade Cancelled")
**Crisis** (1): MsgVerifyInvariant ("Verify Invariant")
**WASM** (9): MsgStoreCode ("Code Stored"), MsgInstantiateContract ("Contract Instantiated"), MsgInstantiateContract2 ("Contract Instantiated"), MsgExecuteContract ("Contract Executed"), MsgMigrateContract ("Contract Migrated"), MsgUpdateAdmin ("Contract Admin Updated"), MsgClearAdmin ("Contract Admin Cleared"), MsgUpdateInstantiateConfig ("Instantiate Config Updated"), MsgSetContractLabel ("Contract Label Set")
**Oracle** (1): MsgAddPriceEntry ("Price Feed")
**BME** (3): MsgBurnMint ("Burn Mint"), MsgMintACT ("Mint ACT"), MsgBurnACT ("Burn ACT")

Each formatter extracts fields from the message and renders using `KV()`. Fields, titles, and formatting rules match SPEC.md §10.11.5 exactly. Some formatters extract additional data from `TxResponse` events (e.g., DSEQ from deployment create events, proposal ID from submit proposal events, code ID from wasm store events).

- [ ] **Step 2: Call RegisterAllTxFormatters from root command setup**

In `internal/cli/root.go`, add the import and call `pretty.RegisterAllTxFormatters()` at the top of `NewRootCmd()`, after `encCfg := aktcodec.MakeEncodingConfig()`:
```go
pretty.RegisterAllTxFormatters()
```

Add to imports:
```go
"pkg.akt.dev/akt/internal/output/pretty"
```

- [ ] **Step 3: Verify the project compiles**

Run: `make akt`
Expected: Clean build.

- [ ] **Step 4: Run existing tests**

Run: `go test ./internal/output/pretty/...`
Expected: All existing tests pass.

---

## Task 3: Tx formatter tests

**Files:**
- Create: `internal/output/pretty/tx_test.go`
- Create: `internal/output/pretty/testdata/TestPrintTxResult_*.golden` (auto-generated)

- [ ] **Step 1: Create tx_test.go with test cases**

Create `internal/output/pretty/tx_test.go` following the existing golden-file pattern used by `bank_test.go`, `deployment_test.go`, etc.

The test file needs helper functions to construct mock `*sdk.TxResponse` objects with embedded `Tx` bodies. Tests cover:

1. **TestPrintTxResult_SingleMsgSuccess** — Bank MsgSend with code=0, verifies two-section layout (Transaction summary + Send detail)
2. **TestPrintTxResult_SingleMsgFailed** — Bank MsgSend with code!=0, verifies "failed: ..." status
3. **TestPrintTxResult_MultiMsg** — Two messages (MsgWithdrawDelegatorReward + MsgDelegate), verifies numbered "Message 1: ..." / "Message 2: ..." format
4. **TestPrintTxResult_UnregisteredMsg** — A message type with no registered formatter, verifies JSON fallback
5. **TestPrintTxResult_DeploymentCreate** — MsgCreateDeployment, verifies Akash-specific fields (Owner, DSEQ, Deposit, Groups)
6. **TestPrintTxResult_Delegate** — MsgDelegate, verifies staking fields (Delegator, Validator, Amount)
7. **TestPrintTxResult_Vote** — MsgVote, verifies governance fields (Voter, Proposal ID, Option)
8. **TestPrintTxResult_WasmExecute** — MsgExecuteContract, verifies wasm fields (Sender, Contract, Funds)

Each test constructs a `*sdk.TxResponse`, captures stdout via `bytes.Buffer`, calls the formatter's render function directly (not through `PrintTxResult` which writes to `os.Stdout`), and compares against a golden file.

- [ ] **Step 2: Run tests with -update to generate golden files**

Run: `go test ./internal/output/pretty/... -run TestPrintTxResult -update`
Expected: Tests pass, golden files are created in `testdata/`.

- [ ] **Step 3: Review golden files for correctness**

Manually inspect the generated golden files to verify output matches SPEC.md §10.11 examples.

- [ ] **Step 4: Run tests without -update to verify golden matching**

Run: `go test ./internal/output/pretty/... -run TestPrintTxResult -v`
Expected: All tests pass.

---

## Task 4: Shell completion command

**Files:**
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Add completionCmd function to root.go**

Add a `completionCmd()` function that returns a `*cobra.Command` with four subcommands: `bash`, `zsh`, `fish`, `powershell`. Each subcommand uses cobra's built-in generation functions (`GenBashCompletionV2`, `GenZshCompletion`, `GenFishCompletion`, `GenPowerShellCompletion`).

The parent command includes an `Example` field with usage examples per SPEC.md help-text requirement:

```
  # Bash
  akt completion bash > /etc/bash_completion.d/akt

  # Zsh
  akt completion zsh > "${fpath[1]}/_akt"

  # Fish
  akt completion fish > ~/.config/fish/completions/akt.fish
```

- [ ] **Step 2: Register the completion command**

In `NewRootCmd()`, after the existing `root.AddCommand(versionCmd(bi))` line, add:
```go
root.AddCommand(completionCmd())
```

- [ ] **Step 3: Verify it works**

Run: `.cache/bin/akt completion bash | head -5`
Expected: Outputs bash completion script header.

Run: `.cache/bin/akt completion --help`
Expected: Shows help with examples.

---

## Task 5: Bootstrap wizard keyring backend selection

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`

- [ ] **Step 1: Add keyring backend selection after network selection**

After the `multiSelect(networks)` call (line 73) and before building the config (line 88), add a keyring backend selection step.

Use the same raw-terminal interactive pattern as `multiSelect` but simpler — a single-select list with three options:
- `os` — System keyring (default, recommended for production)
- `file` — File-based encrypted keyring
- `test` — Unencrypted test keyring (for development only)

Default selection: `os`.

Add a function `selectKeyringBackend() string` that presents the selection and returns the chosen backend string.

- [ ] **Step 2: Use the selected backend in config construction**

Replace the hardcoded keyring on line 92-94:
```go
Keyrings: []aktctx.Keyring{
    {Name: "default", Backend: "os"},
},
```
with:
```go
Keyrings: []aktctx.Keyring{
    {Name: "default", Backend: backend},
},
```
where `backend` is the return value of `selectKeyringBackend()`.

- [ ] **Step 3: Fix the Defaults.Output value**

On line 96, change:
```go
Output: "table",
```
to:
```go
Output: "pretty",
```
This aligns with `DefaultConfig()` in the context package and SPEC.md §1.2.

- [ ] **Step 4: Verify it compiles**

Run: `make akt`
Expected: Clean build.

---

## Task 6: E2E test suite (offline)

**Files:**
- Create: `e2e/cli_test.go`
- Create: `e2e/helpers_test.go`
- Modify: `Makefile`

- [ ] **Step 1: Create E2E test helpers**

Create `e2e/helpers_test.go` with:
- `aktBinary()` — returns path to the compiled `akt` binary (`.cache/bin/akt`)
- `runAkt(t, home, args...)` — runs the akt binary with `--home` pointing to a temp dir, captures stdout/stderr, returns exit code
- `setupHome(t)` — creates a temp dir, runs `akt context network create --template mainnet` to bootstrap a config
- `requireSuccess(t, stdout, stderr, exitCode)` — assertion helper
- `requireContains(t, output, substr)` — assertion helper

- [ ] **Step 2: Create E2E CLI tests**

Create `e2e/cli_test.go` with test cases:

1. **TestVersion** — `akt version` exits 0 and outputs version info
2. **TestHelp** — `akt --help` exits 0 and contains "Akash Network CLI"
3. **TestContextLifecycle** — Create network from template, create context, list contexts (verify it appears), use context (switch), show context (verify details), rename context, delete context
4. **TestNetworkTemplates** — Create mainnet/testnet/sandbox from templates, list networks (verify all 3), show each (verify chain-id, endpoints)
5. **TestKeyManagement** — Create a context with test keyring, add a key (`akt context keys add testkey --keyring-backend test`), list keys (verify it appears), show key (verify address), delete key
6. **TestCompletionGeneration** — `akt completion bash` exits 0 and outputs non-empty script, same for `zsh` and `fish`
7. **TestOutputFormats** — `akt version -o json` outputs JSON, `-o yaml` outputs YAML
8. **TestUnknownCommand** — `akt nonexistent` exits non-zero with "unknown command"

All tests use `t.TempDir()` for isolation and the built binary.

- [ ] **Step 3: Add Makefile targets**

Add to `Makefile`:
```makefile
.PHONY: test test-e2e

test:
	$(GO_TEST) ./...

test-e2e: akt
	$(GO_TEST) ./e2e/... -v -count=1
```

- [ ] **Step 4: Build and run E2E tests**

Run: `make test-e2e`
Expected: All tests pass.

---

## Task 7: Changelog and final verification

**Files:**
- Modify: `AICHANGELOG.md`

- [ ] **Step 1: Add AICHANGELOG entries**

Add entries for:
- Tx result pretty formatters (T033): 60 per-message formatters, PrintTxResult wiring, output flag default fix
- Tx formatter tests (T014): golden-file tests for PrintTxResult dispatch
- Shell completion (T034): `akt completion bash/zsh/fish/powershell` subcommand
- Bootstrap keyring (T036): keyring backend selection in first-run wizard
- E2E tests (T037): offline E2E test suite for context, network, keys, completion

- [ ] **Step 2: Run full test suite**

Run: `go test ./...`
Expected: All tests pass.

- [ ] **Step 3: Build final binary**

Run: `make akt`
Expected: Clean build.
