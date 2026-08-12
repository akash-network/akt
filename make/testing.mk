# Coverage collection for SPEC.md section 12.
#
# coverage/packages.tsv is the denominator contract. Unit tests instrument the
# entire repository package set in one invocation, while E2E jobs execute an
# instrumented akt binary and write subprocess counters to their own shard.
# Raw covdata is merged before any percentage is calculated: percentages from
# separate jobs are never averaged.

COVERAGE_TOOL              := $(GO) run ./tools/coverage
COVERAGE_PACKAGES_FILE     := coverage/packages.tsv
COVERAGE_EXCEPTIONS_FILE   := coverage/exceptions.tsv
COVERAGE_CACHE_ROOT        := $(or $(AKT_DEVCACHE),$(AKT_ROOT)/.cache)/coverage
COVERAGE_DATA_ROOT         := $(COVERAGE_CACHE_ROOT)/covdata
COVERAGE_REPORT_ROOT       := $(COVERAGE_CACHE_ROOT)/reports
COVERAGE_UNIT_DATA         := $(COVERAGE_DATA_ROOT)/unit
COVERAGE_OFFLINE_DATA      := $(COVERAGE_DATA_ROOT)/e2e-offline
COVERAGE_LOCALNET_DATA     := $(COVERAGE_DATA_ROOT)/e2e-localnet
COVERAGE_LIVE_DATA         := $(COVERAGE_DATA_ROOT)/e2e-live
COVERAGE_E2E_DATA          := $(COVERAGE_DATA_ROOT)/e2e
COVERAGE_UNION_DATA        := $(COVERAGE_DATA_ROOT)/union
COVERAGE_UNION_LIVE_DATA   := $(COVERAGE_DATA_ROOT)/union-live
COVERAGE_BINARY            := $(or $(AKT_DEVCACHE_BIN),$(AKT_ROOT)/.cache/bin)/akt
COVERAGE_BINARY_MANIFEST   := $(COVERAGE_CACHE_ROOT)/binary-source-manifest.tsv
COVERAGE_BINARY_IDENTITY   := $(COVERAGE_CACHE_ROOT)/binary-identity.tsv
COVERAGE_BINARY_PRE_MANIFEST := $(COVERAGE_CACHE_ROOT)/binary-source-manifest.pre.tsv
COVERAGE_BINARY_POST_MANIFEST := $(COVERAGE_CACHE_ROOT)/binary-source-manifest.post.tsv
COVERAGE_BINARY_CANDIDATE  := $(COVERAGE_BINARY).coverage-candidate
COVERAGE_BINARY_IDENTITY_CANDIDATE := $(COVERAGE_BINARY_IDENTITY).candidate
COVERAGE_CURRENT_MANIFEST  := $(COVERAGE_REPORT_ROOT)/current-source-manifest.tsv

COVERAGE_REPOSITORY_PROFILE := $(COVERAGE_REPORT_ROOT)/repository-union.out
COVERAGE_ACTIVE_PROFILE     := $(COVERAGE_REPORT_ROOT)/active-union.out
COVERAGE_TUI_PROFILE        := $(COVERAGE_REPORT_ROOT)/experimental-tui-union.out
COVERAGE_TOOLING_PROFILE    := $(COVERAGE_REPORT_ROOT)/tooling-unit.out
COVERAGE_UNIT_PROFILE       := $(COVERAGE_REPORT_ROOT)/unit.out
COVERAGE_OFFLINE_PROFILE    := $(COVERAGE_REPORT_ROOT)/e2e-offline.out
COVERAGE_LOCALNET_PROFILE   := $(COVERAGE_REPORT_ROOT)/e2e-localnet.out
COVERAGE_LIVE_PROFILE       := $(COVERAGE_REPORT_ROOT)/live.out
COVERAGE_LIVE_ACTIVE_PROFILE := $(COVERAGE_REPORT_ROOT)/live-active.out
COVERAGE_E2E_PROFILE        := $(COVERAGE_REPORT_ROOT)/e2e.out
COVERAGE_UNION_LIVE_PROFILE := $(COVERAGE_REPORT_ROOT)/active-union-live.out
COVERAGE_ACTIVE_BASELINE    := coverage/baseline-active-union.tsv
COVERAGE_TUI_BASELINE       := coverage/baseline-experimental-tui-union.tsv
COVERAGE_TOOLING_BASELINE   := coverage/baseline-tooling-unit.tsv

COVERAGE_TEST_PACKAGES       := $(shell awk -F '\t' 'NR > 1 { print $$1 }' $(COVERAGE_PACKAGES_FILE))
COVERAGE_REPOSITORY_PACKAGES := $(shell awk -F '\t' 'NR > 1 && $$2 != "support" { print $$1 }' $(COVERAGE_PACKAGES_FILE))
COVERAGE_ACTIVE_PACKAGES     := $(shell awk -F '\t' 'NR > 1 && $$2 == "active" { print $$1 }' $(COVERAGE_PACKAGES_FILE))
COVERAGE_PACKAGE_CSV         := $(subst $(WHITESPACE),$(COMMA),$(strip $(COVERAGE_REPOSITORY_PACKAGES)))
COVERAGE_BINARY_PACKAGES     := $(shell awk -F '\t' 'NR > 1 && $$2 != "support" && $$2 != "tooling" { print $$1 }' $(COVERAGE_PACKAGES_FILE))
COVERAGE_BINARY_PACKAGE_CSV  := $(subst $(WHITESPACE),$(COMMA),$(strip $(COVERAGE_BINARY_PACKAGES)))
COVERAGE_MANIFEST_FLAGS      := -build-tags '$(build_tags_cs)' -build-options '$(strip $(BUILD_OPTIONS))'

BASE_REF ?= HEAD^
HEAD_REF ?= WORKTREE

.PHONY: test-coverage-validate
test-coverage-validate:
	$(COVERAGE_TOOL) validate -packages $(COVERAGE_PACKAGES_FILE) \
		-exceptions $(COVERAGE_EXCEPTIONS_FILE) \
		-release-tags '$(build_tags_cs)'

.PHONY: test-race-active
test-race-active: test-coverage-validate
	$(GO_TEST) -race -count=1 -tags='$(build_tags_cs)' $(COVERAGE_ACTIVE_PACKAGES)

# Cross-package unit coverage. -test.gocoverdir is intentional: setting only
# GOCOVERDIR does not make `go test` retain each generated test binary's raw
# counters. The explicit test flag gives covdata one duplicate-free statement
# map that can later be merged with instrumented CLI subprocess counters.
.PHONY: test-coverage-unit
test-coverage-unit: test-coverage-validate
	rm -rf $(COVERAGE_UNIT_DATA)
	mkdir -p $(COVERAGE_UNIT_DATA) $(COVERAGE_REPORT_ROOT)
	$(COVERAGE_TOOL) source-manifest $(COVERAGE_MANIFEST_FLAGS) \
		-out $(COVERAGE_UNIT_DATA)/source-manifest.tsv
	$(GO_TEST) -count=1 -cover -covermode=atomic -tags='$(build_tags_cs)' \
		-coverpkg=$(COVERAGE_PACKAGE_CSV) \
		$(COVERAGE_TEST_PACKAGES) \
		-args -test.gocoverdir=$(abspath $(COVERAGE_UNIT_DATA))
	$(GO) tool covdata textfmt -i=$(COVERAGE_UNIT_DATA) -o=$(COVERAGE_UNIT_PROFILE)
	$(COVERAGE_TOOL) filter -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_UNIT_PROFILE) -class tooling \
		-out $(COVERAGE_TOOLING_PROFILE)
	$(COVERAGE_TOOL) report -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_UNIT_PROFILE) -class repository \
		-out $(COVERAGE_REPORT_ROOT)/unit.tsv
	$(COVERAGE_TOOL) report -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_TOOLING_PROFILE) -class tooling \
		-out $(COVERAGE_REPORT_ROOT)/tooling-unit.tsv
	$(COVERAGE_TOOL) ratchet -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_TOOLING_PROFILE) -class tooling \
		-baseline $(COVERAGE_TOOLING_BASELINE) -base "$(BASE_REF)"
	@tail -1 $(COVERAGE_REPORT_ROOT)/unit.tsv
	@tail -1 $(COVERAGE_REPORT_ROOT)/tooling-unit.tsv

# Build exactly the binary exercised by coverage-labelled E2E jobs. The child
# processes inherit GOCOVERDIR from the Go E2E harness.
.PHONY: test-coverage-binary
test-coverage-binary: test-coverage-validate
	mkdir -p $(dir $(COVERAGE_BINARY)) $(dir $(COVERAGE_BINARY_MANIFEST))
	rm -f $(COVERAGE_BINARY_MANIFEST) $(COVERAGE_BINARY_IDENTITY) \
		$(COVERAGE_BINARY_PRE_MANIFEST) $(COVERAGE_BINARY_POST_MANIFEST) \
		$(COVERAGE_BINARY_CANDIDATE) $(COVERAGE_BINARY_IDENTITY_CANDIDATE)
	$(COVERAGE_TOOL) source-manifest $(COVERAGE_MANIFEST_FLAGS) \
		-out $(COVERAGE_BINARY_PRE_MANIFEST)
	$(GO_BUILD) -trimpath -cover -covermode=atomic \
		-coverpkg=$(COVERAGE_BINARY_PACKAGE_CSV) \
		-tags='$(build_tags_cs)' \
		-ldflags '$(ldflags)' \
		-o $(COVERAGE_BINARY_CANDIDATE) ./cmd/akt
	$(COVERAGE_TOOL) source-manifest $(COVERAGE_MANIFEST_FLAGS) \
		-out $(COVERAGE_BINARY_POST_MANIFEST)
	@cmp -s $(COVERAGE_BINARY_PRE_MANIFEST) $(COVERAGE_BINARY_POST_MANIFEST) || \
		{ rm -f $(COVERAGE_BINARY_CANDIDATE) $(COVERAGE_BINARY_PRE_MANIFEST) \
			$(COVERAGE_BINARY_POST_MANIFEST) $(COVERAGE_BINARY_IDENTITY_CANDIDATE); \
			echo "source changed while building the coverage binary; rebuild from a stable tree" >&2; \
			exit 1; }
	$(COVERAGE_TOOL) binary-identity -binary $(COVERAGE_BINARY_CANDIDATE) \
		-source-manifest $(COVERAGE_BINARY_POST_MANIFEST) \
		-out $(COVERAGE_BINARY_IDENTITY_CANDIDATE)
	mv $(COVERAGE_BINARY_CANDIDATE) $(COVERAGE_BINARY)
	mv $(COVERAGE_BINARY_POST_MANIFEST) $(COVERAGE_BINARY_MANIFEST)
	mv $(COVERAGE_BINARY_IDENTITY_CANDIDATE) $(COVERAGE_BINARY_IDENTITY)
	rm -f $(COVERAGE_BINARY_PRE_MANIFEST)

# Prepare a unique E2E shard. The caller must export GOCOVERDIR as the printed
# absolute path while running the suite. Supported values are deliberately
# closed so a typo cannot erase an unrelated cache directory.
.PHONY: test-coverage-e2e-prepare
test-coverage-e2e-prepare:
	@case "$(COVERAGE_SHARD)" in \
		e2e-offline|e2e-localnet|e2e-live) ;; \
		*) echo "COVERAGE_SHARD must be e2e-offline, e2e-localnet, or e2e-live" >&2; exit 2 ;; \
	esac
	@test -f $(COVERAGE_BINARY_MANIFEST) || \
		{ echo "missing instrumented-binary source manifest: $(COVERAGE_BINARY_MANIFEST)" >&2; exit 1; }
	$(COVERAGE_TOOL) verify-binary-identity -binary $(COVERAGE_BINARY) \
		-source-manifest $(COVERAGE_BINARY_MANIFEST) \
		-identity $(COVERAGE_BINARY_IDENTITY)
	rm -rf $(COVERAGE_DATA_ROOT)/$(COVERAGE_SHARD)
	mkdir -p $(COVERAGE_DATA_ROOT)/$(COVERAGE_SHARD)
	cp $(COVERAGE_BINARY_MANIFEST) $(COVERAGE_DATA_ROOT)/$(COVERAGE_SHARD)/source-manifest.tsv
	cp $(COVERAGE_BINARY_IDENTITY) $(COVERAGE_DATA_ROOT)/$(COVERAGE_SHARD)/binary-identity.tsv
	@echo "GOCOVERDIR=$(abspath $(COVERAGE_DATA_ROOT)/$(COVERAGE_SHARD))"

# Validate a collected shard before artifact upload. The shard's manifest is
# compared to a freshly generated checkout manifest. E2E shards also recheck
# the exact executable identity after the suite, and the validator rejects any
# entry outside the shard-specific allowlist.
.PHONY: test-coverage-shard-ready
test-coverage-shard-ready:
	@case "$(COVERAGE_SHARD)" in \
		unit|e2e-offline|e2e-localnet|e2e-live) ;; \
		*) echo "COVERAGE_SHARD must be unit, e2e-offline, e2e-localnet, or e2e-live" >&2; exit 2 ;; \
	esac
	mkdir -p $(COVERAGE_REPORT_ROOT)
	$(COVERAGE_TOOL) source-manifest $(COVERAGE_MANIFEST_FLAGS) \
		-out $(COVERAGE_CURRENT_MANIFEST)
	$(COVERAGE_TOOL) verify-shard -dir $(COVERAGE_DATA_ROOT)/$(COVERAGE_SHARD) \
		-name $(COVERAGE_SHARD) -source-manifest $(COVERAGE_CURRENT_MANIFEST) $(if $(filter e2e-offline e2e-localnet e2e-live,$(COVERAGE_SHARD)),-require-binary-identity -binary $(COVERAGE_BINARY))

# Merge the three blocking lanes and derive the separately named denominators.
# Artifact download in CI restores each raw shard at these exact paths.
.PHONY: test-coverage-report
test-coverage-report: test-coverage-validate
	mkdir -p $(COVERAGE_REPORT_ROOT)
	$(COVERAGE_TOOL) source-manifest $(COVERAGE_MANIFEST_FLAGS) \
		-out $(COVERAGE_CURRENT_MANIFEST)
	@test -d $(COVERAGE_UNIT_DATA) || \
		{ echo "missing unit covdata: $(COVERAGE_UNIT_DATA)" >&2; exit 1; }
	@test -d $(COVERAGE_OFFLINE_DATA) || \
		{ echo "missing offline E2E covdata: $(COVERAGE_OFFLINE_DATA)" >&2; exit 1; }
	@test -d $(COVERAGE_LOCALNET_DATA) || \
		{ echo "missing localnet E2E covdata: $(COVERAGE_LOCALNET_DATA)" >&2; exit 1; }
	$(COVERAGE_TOOL) verify-shard -dir $(COVERAGE_UNIT_DATA) -name unit \
		-source-manifest $(COVERAGE_CURRENT_MANIFEST)
	$(COVERAGE_TOOL) verify-shard -dir $(COVERAGE_OFFLINE_DATA) -name e2e-offline \
		-source-manifest $(COVERAGE_CURRENT_MANIFEST) -require-binary-identity
	$(COVERAGE_TOOL) verify-shard -dir $(COVERAGE_LOCALNET_DATA) -name e2e-localnet \
		-source-manifest $(COVERAGE_CURRENT_MANIFEST) -require-binary-identity
	$(GO) tool covdata textfmt -i=$(COVERAGE_UNIT_DATA) -o=$(COVERAGE_UNIT_PROFILE)
	$(COVERAGE_TOOL) filter -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_UNIT_PROFILE) -class tooling \
		-out $(COVERAGE_TOOLING_PROFILE)
	$(GO) tool covdata textfmt -i=$(COVERAGE_OFFLINE_DATA) -o=$(COVERAGE_OFFLINE_PROFILE)
	$(GO) tool covdata textfmt -i=$(COVERAGE_LOCALNET_DATA) -o=$(COVERAGE_LOCALNET_PROFILE)
	rm -rf $(COVERAGE_E2E_DATA)
	mkdir -p $(COVERAGE_E2E_DATA)
	$(GO) tool covdata merge \
		-i=$(COVERAGE_OFFLINE_DATA),$(COVERAGE_LOCALNET_DATA) \
		-o=$(COVERAGE_E2E_DATA)
	$(GO) tool covdata textfmt -i=$(COVERAGE_E2E_DATA) -o=$(COVERAGE_E2E_PROFILE)
	rm -rf $(COVERAGE_UNION_DATA)
	mkdir -p $(COVERAGE_UNION_DATA)
	$(GO) tool covdata merge \
		-i=$(COVERAGE_UNIT_DATA),$(COVERAGE_OFFLINE_DATA),$(COVERAGE_LOCALNET_DATA) \
		-o=$(COVERAGE_UNION_DATA)
	$(GO) tool covdata textfmt -i=$(COVERAGE_UNION_DATA) \
		-o=$(COVERAGE_REPORT_ROOT)/union-raw.out
	$(COVERAGE_TOOL) filter -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_REPORT_ROOT)/union-raw.out -class repository \
		-out $(COVERAGE_REPOSITORY_PROFILE)
	$(COVERAGE_TOOL) filter -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_REPORT_ROOT)/union-raw.out -class active \
		-out $(COVERAGE_ACTIVE_PROFILE)
	$(COVERAGE_TOOL) filter -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_REPORT_ROOT)/union-raw.out -class experimental-tui \
		-out $(COVERAGE_TUI_PROFILE)
	$(COVERAGE_TOOL) report -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_REPOSITORY_PROFILE) -class repository \
		-out $(COVERAGE_REPORT_ROOT)/repository-union.tsv
	$(COVERAGE_TOOL) report -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_ACTIVE_PROFILE) -class active \
		-out $(COVERAGE_REPORT_ROOT)/active-union.tsv
	$(COVERAGE_TOOL) report -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_TUI_PROFILE) -class experimental-tui \
		-out $(COVERAGE_REPORT_ROOT)/experimental-tui-union.tsv
	$(COVERAGE_TOOL) report -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_TOOLING_PROFILE) -class tooling \
		-out $(COVERAGE_REPORT_ROOT)/tooling-unit.tsv
	$(GO) tool cover -func=$(COVERAGE_ACTIVE_PROFILE) > $(COVERAGE_REPORT_ROOT)/active-union-func.txt
	$(GO) tool cover -html=$(COVERAGE_ACTIVE_PROFILE) -o $(COVERAGE_REPORT_ROOT)/active-union.html
	@echo "repository union:      $$(tail -1 $(COVERAGE_REPORT_ROOT)/repository-union.tsv)"
	@echo "active union:          $$(tail -1 $(COVERAGE_REPORT_ROOT)/active-union.tsv)"
	@echo "experimental TUI union:$$(tail -1 $(COVERAGE_REPORT_ROOT)/experimental-tui-union.tsv)"
	@echo "tooling unit:          $$(tail -1 $(COVERAGE_REPORT_ROOT)/tooling-unit.tsv)"
	$(MAKE) test-coverage-ratchet

# Merge a successful externally hosted Console shard into a separate
# informational profile. This never changes the blocking active union or its
# ratchets; external availability cannot hide a hermetic coverage gap.
.PHONY: test-coverage-live-report
test-coverage-live-report: test-coverage-report
	mkdir -p $(COVERAGE_REPORT_ROOT)
	$(COVERAGE_TOOL) source-manifest $(COVERAGE_MANIFEST_FLAGS) \
		-out $(COVERAGE_CURRENT_MANIFEST)
	@test -d $(COVERAGE_LIVE_DATA) || \
		{ echo "missing live Console covdata: $(COVERAGE_LIVE_DATA)" >&2; exit 1; }
	$(COVERAGE_TOOL) verify-shard -dir $(COVERAGE_LIVE_DATA) -name live \
		-source-manifest $(COVERAGE_CURRENT_MANIFEST) -require-binary-identity
	$(GO) tool covdata textfmt -i=$(COVERAGE_LIVE_DATA) -o=$(COVERAGE_LIVE_PROFILE)
	$(COVERAGE_TOOL) filter -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_LIVE_PROFILE) -class active \
		-out $(COVERAGE_LIVE_ACTIVE_PROFILE)
	$(COVERAGE_TOOL) report -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_LIVE_ACTIVE_PROFILE) -class active \
		-out $(COVERAGE_REPORT_ROOT)/live-active.tsv
	rm -rf $(COVERAGE_UNION_LIVE_DATA)
	mkdir -p $(COVERAGE_UNION_LIVE_DATA)
	$(GO) tool covdata merge \
		-i=$(COVERAGE_UNION_DATA),$(COVERAGE_LIVE_DATA) \
		-o=$(COVERAGE_UNION_LIVE_DATA)
	$(GO) tool covdata textfmt -i=$(COVERAGE_UNION_LIVE_DATA) \
		-o=$(COVERAGE_REPORT_ROOT)/union-live-raw.out
	$(COVERAGE_TOOL) filter -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_REPORT_ROOT)/union-live-raw.out -class active \
		-out $(COVERAGE_UNION_LIVE_PROFILE)
	$(COVERAGE_TOOL) report -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_UNION_LIVE_PROFILE) -class active \
		-out $(COVERAGE_REPORT_ROOT)/active-union-live.tsv
	@echo "live active:        $$(tail -1 $(COVERAGE_REPORT_ROOT)/live-active.tsv)"
	@echo "active union + live:$$(tail -1 $(COVERAGE_REPORT_ROOT)/active-union-live.tsv)"

.PHONY: test-coverage-ratchet
test-coverage-ratchet:
	$(COVERAGE_TOOL) ratchet -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_ACTIVE_PROFILE) -class active \
		-baseline $(COVERAGE_ACTIVE_BASELINE) -base "$(BASE_REF)"
	$(COVERAGE_TOOL) ratchet -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_TUI_PROFILE) -class experimental-tui \
		-baseline $(COVERAGE_TUI_BASELINE) -base "$(BASE_REF)"
	$(COVERAGE_TOOL) ratchet -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_TOOLING_PROFILE) -class tooling \
		-baseline $(COVERAGE_TOOLING_BASELINE) -base "$(BASE_REF)"

.PHONY: test-coverage-patch
test-coverage-patch:
	$(COVERAGE_TOOL) patch -packages $(COVERAGE_PACKAGES_FILE) \
		-exceptions $(COVERAGE_EXCEPTIONS_FILE) \
		-profile $(COVERAGE_ACTIVE_PROFILE) \
		-edge-profile $(COVERAGE_REPORT_ROOT)/union-raw.out \
		-base "$(BASE_REF)" -head "$(HEAD_REF)"

# Generate reviewable candidates under .cache. Updating a checked-in baseline
# remains an explicit reviewed change; this target never overwrites it.
.PHONY: test-coverage-baseline-candidates
test-coverage-baseline-candidates:
	$(COVERAGE_TOOL) baseline -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_ACTIVE_PROFILE) -class active \
		-out $(COVERAGE_REPORT_ROOT)/baseline-active-union.candidate.tsv
	$(COVERAGE_TOOL) baseline -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_TUI_PROFILE) -class experimental-tui \
		-out $(COVERAGE_REPORT_ROOT)/baseline-experimental-tui-union.candidate.tsv
	$(COVERAGE_TOOL) baseline -packages $(COVERAGE_PACKAGES_FILE) \
		-profile $(COVERAGE_TOOLING_PROFILE) -class tooling \
		-out $(COVERAGE_REPORT_ROOT)/baseline-tooling-unit.candidate.tsv
	@echo "baseline candidates are in $(COVERAGE_REPORT_ROOT)"

# Compatibility aliases. `test-coverage` remains the useful local unit report;
# `test-coverage-check` is the complete local gate. CI invokes its report and
# changed-line phases as separate named steps for clearer diagnostics.
.PHONY: test-coverage test-coverage-check test-coverage-own test-coverage-core
test-coverage: test-coverage-unit
test-coverage-check: test-coverage-report
	$(MAKE) test-coverage-patch
test-coverage-own: test-coverage-unit
	@echo "test-coverage-own is deprecated; use the named active-union report"
test-coverage-core: test-coverage-unit
	@echo "test-coverage-core is deprecated; the active-union ratchet is authoritative"

.PHONY: test-coverage-html
test-coverage-html: test-coverage-report
	$(GO) tool cover -html=$(COVERAGE_ACTIVE_PROFILE)

.PHONY: test-coverage-clean
test-coverage-clean:
	rm -rf $(COVERAGE_CACHE_ROOT)
