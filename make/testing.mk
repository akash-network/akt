# Test coverage.
#
# ./e2e is excluded from every coverage target below: it drives a built `akt`
# binary as a subprocess, so the coverage counters live in that child process
# and including the package only adds a 0%-covered entry. Run `make test-e2e`
# for that suite.
#
# Three package sets are reported, narrowest last:
#
#   COVER_PACKAGES       every non-e2e package -- the honest repo-wide figure.
#
#   COVER_OWN_PACKAGES   everything except internal/cli/chain/..., which is
#                        clean-copied from akash-network/chain-sdk and
#                        maintained by re-copying (DESIGN.md 5.1). Tests
#                        written against that tree would be clobbered by the
#                        next re-copy, and at ~7.9k statements it is over a
#                        third of the repo, so a repo-wide threshold would
#                        mostly measure code akt does not own.
#
#   COVER_CORE_PACKAGES  the akt-authored packages that carry the risk:
#                        money handling, credentials, capability gating,
#                        the action log, the transport translation layer,
#                        and the command surfaces built on them. This is the
#                        set the threshold applies to.

COVER_DIR             := $(AKT_DEVCACHE)/coverage
COVER_PROFILE         := $(COVER_DIR)/coverage.out
COVER_OWN_PROFILE     := $(COVER_DIR)/coverage-own.out
COVER_CORE_PROFILE    := $(COVER_DIR)/coverage-core.out
COVER_HTML            := $(COVER_DIR)/coverage.html
COVER_FUNC            := $(COVER_DIR)/coverage-func.txt

# Minimum statement coverage for COVER_CORE_PACKAGES, in percent. Raise it as
# coverage improves; never lower it to make a build pass.
COVER_THRESHOLD       ?= 65

COVER_PACKAGES         = $(shell $(GO) list ./... | grep -v '/e2e')
COVER_OWN_PACKAGES     = $(shell $(GO) list ./... | grep -v '/e2e' | grep -v '/internal/cli/chain')

COVER_CORE_PACKAGES   := \
	$(GO_MOD_NAME)/internal/actionlog \
	$(GO_MOD_NAME)/internal/bootstrap \
	$(GO_MOD_NAME)/internal/capability \
	$(GO_MOD_NAME)/internal/cli/console \
	$(GO_MOD_NAME)/internal/cli/context \
	$(GO_MOD_NAME)/internal/cli/provider \
	$(GO_MOD_NAME)/internal/cli/sdl \
	$(GO_MOD_NAME)/internal/cli/workflow \
	$(GO_MOD_NAME)/internal/console \
	$(GO_MOD_NAME)/internal/context \
	$(GO_MOD_NAME)/internal/output/pretty \
	$(GO_MOD_NAME)/internal/transport \
	$(GO_MOD_NAME)/internal/workflow

$(COVER_DIR):
	mkdir -p $@

# Full unit-test run with a coverage profile, plus a per-function report and a
# browsable HTML report. Everything lands under .cache (gitignored).
.PHONY: test-coverage
test-coverage: $(COVER_DIR)
	$(GO_TEST) -covermode=atomic -coverprofile=$(COVER_PROFILE) $(COVER_PACKAGES)
	$(GO) tool cover -func=$(COVER_PROFILE) > $(COVER_FUNC)
	$(GO) tool cover -html=$(COVER_PROFILE) -o $(COVER_HTML)
	@echo
	@tail -1 $(COVER_FUNC)
	@echo "per-function report: $(COVER_FUNC)"
	@echo "html report:         $(COVER_HTML)"

# Repo minus the clean-copied chain-sdk tree.
.PHONY: test-coverage-own
test-coverage-own: $(COVER_DIR)
	$(GO_TEST) -covermode=atomic -coverprofile=$(COVER_OWN_PROFILE) $(COVER_OWN_PACKAGES)
	@echo
	@$(GO) tool cover -func=$(COVER_OWN_PROFILE) | tail -1

# The risk-carrying core. This is the number the threshold is applied to.
.PHONY: test-coverage-core
test-coverage-core: $(COVER_DIR)
	$(GO_TEST) -covermode=atomic -coverprofile=$(COVER_CORE_PROFILE) $(COVER_CORE_PACKAGES)
	@echo
	@$(GO) tool cover -func=$(COVER_CORE_PROFILE) | tail -1

# Gate: fail when the core packages fall below COVER_THRESHOLD.
# This is the target CI should run.
.PHONY: test-coverage-check
test-coverage-check: test-coverage-core
	@total=$$($(GO) tool cover -func=$(COVER_CORE_PROFILE) | tail -1 | awk '{print $$NF}' | tr -d '%'); \
	echo "core coverage: $${total}% (threshold $(COVER_THRESHOLD)%)"; \
	awk -v t="$${total}" -v min="$(COVER_THRESHOLD)" \
		'BEGIN { if (t+0 < min+0) { printf "coverage %.1f%% is below the %s%% threshold\n", t, min; exit 1 } }'

# Open the HTML report in a browser.
.PHONY: test-coverage-html
test-coverage-html: test-coverage
	$(GO) tool cover -html=$(COVER_PROFILE)

.PHONY: test-coverage-clean
test-coverage-clean:
	rm -rf $(COVER_DIR)
