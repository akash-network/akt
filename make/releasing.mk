# Release tooling.
#
# GoReleaser Pro is run from the goreleaser-cross-pro container image rather
# than as a local binary: the darwin targets need osxcross and the linux targets
# need the gnu cross toolchains, and we do not want either as a developer
# prerequisite. Docker is required for GoReleaser targets; changelog targets use
# local Go. Pro is needed for the split prepare/publish release (SPEC §12.7):
# release-dry-run, release-prepare and release-publish need the license key in
# GORELEASER_KEY, while release-check and release-snapshot run without it.

.PHONY: changelog-check changelog-assemble changelog-release-check
changelog-check:
	$(GO) run ./tools/changelog check $(if $(CHANGELOG_BASE),"$(CHANGELOG_BASE)")

changelog-assemble:
	$(GO) run ./tools/changelog assemble

changelog-release-check:
	$(GO) run ./tools/changelog release-check

# Maintainer release (SPEC §2.16): opens the release-preparation PR, waits for
# a human to merge it, then signs and pushes the tag on the merge commit.
# Prompts for the tag unless VERSION is set, either on the command line or in
# the environment. The recipe must not reference MAKE: that would make
# `make -n tag-release` run the release.
.PHONY: tag-release
tag-release:
	@VERSION="$(VERSION)" GOWORK=off "$(AKT_ROOT)/script/tools.sh" tag-release

# goreleaser-cross-pro publishes one image per Go patch release, but not for
# every patch -- there is no v1.26.1 image even though go.mod pins go 1.26.1.
# Keep the readable tag and immutable index digest together, and update both
# only after inspecting the published multi-platform manifest. v1.26.2 bundles
# GoReleaser Pro v2.15.2, the same GoReleaser version as the open-source image
# it replaced.
GORELEASER_CROSS_VERSION ?= v1.26.2
GORELEASER_CROSS_DIGEST  ?= sha256:c51bf484cb3419f17b51695050d74703f8c49c79a231ff99caff8e675a8eabf8
GORELEASER_IMAGE         := ghcr.io/goreleaser/goreleaser-cross-pro:$(GORELEASER_CROSS_VERSION)@$(GORELEASER_CROSS_DIGEST)

# The digest names a multi-arch index (linux/amd64 and linux/arm64), so Docker
# runs the host's native variant; no target pins a platform.

GORELEASER_VERBOSE       ?= false
GORELEASER_MOUNT_CONFIG  ?= false

# Extra comma-separated goreleaser steps to skip, on top of whatever each
# target already skips, e.g. `make release-dry-run GORELEASER_SKIP=nfpm`.
GORELEASER_SKIP          ?=

# Where the repo is mounted inside the container. Keeping it under /go/src with
# the canonical import path keeps GOPATH-shaped tooling happy.
GORELEASER_MOD_MOUNT     ?= $(shell cat $(AKT_ROOT)/.github/repo | tr -d '\n')
GORELEASER_WORKDIR       := /go/src/$(GORELEASER_MOD_MOUNT)

# Container image path akt would publish to. Nothing consumes this yet --
# .goreleaser.yaml has no `dockers:` section because akt is a client binary,
# not a node -- but this is the path to use when one is added.
RELEASE_DOCKER_IMAGE     ?= ghcr.io/akash-network/akt

GORELEASER_GOWORK        := off

ifneq ($(strip $(GOWORK)),)
ifneq ($(strip $(GOWORK)),off)
	GORELEASER_GOWORK    := $(GORELEASER_WORKDIR)/go.work
endif
endif

GORELEASER_ARGS :=

ifeq ($(GORELEASER_VERBOSE),true)
	GORELEASER_ARGS += --verbose
endif

ifneq (,$(GORELEASER_SKIP))
	GORELEASER_ARGS += --skip=$(GORELEASER_SKIP)
endif

# `goreleaser publish` accepts no pipe skips (its --skip takes only `after`),
# so GORELEASER_SKIP is not forwarded to it.
GORELEASER_PUBLISH_ARGS :=

ifeq ($(GORELEASER_VERBOSE),true)
	GORELEASER_PUBLISH_ARGS += --verbose
endif

# The image entrypoint runs `docker login ghcr.io` whenever GITHUB_TOKEN is set
# and /root/.docker/config.json is absent, and it runs under `set -e`, so that
# login failing kills the release before goreleaser even starts. It does fail:
# the workflow token is scoped to contents only and akt publishes no images.
# Handing the container a docker config that already exists skips that whole
# block. GORELEASER_MOUNT_CONFIG=true swaps in the real one for anyone who does
# need to push images.
GORELEASER_DOCKER_CONFIG := $(AKT_DEVCACHE)/goreleaser-docker

$(GORELEASER_DOCKER_CONFIG)/config.json:
	@mkdir -p $(@D)
	@echo '{}' > $@

# Named volumes keep the module and build caches between runs without writing
# root-owned files into the developer's GOPATH.
GORELEASER_DOCKER_ARGS := \
	--rm \
	-e GOWORK=$(GORELEASER_GOWORK) \
	-v akt-goreleaser-gomod:/go/pkg/mod \
	-v akt-goreleaser-gobuild:/root/.cache/go-build \
	-v $(AKT_ROOT):$(GORELEASER_WORKDIR) \
	-w $(GORELEASER_WORKDIR)

ifeq ($(GORELEASER_MOUNT_CONFIG),true)
	GORELEASER_DOCKER_ARGS += -v $(HOME)/.docker/config.json:/root/.docker/config.json
else
	GORELEASER_DOCKER_ARGS += -v $(GORELEASER_DOCKER_CONFIG):/root/.docker
endif

# The image entrypoint is goreleaser itself, so arguments are appended directly.
GORELEASER := docker run $(GORELEASER_DOCKER_ARGS) $(GORELEASER_IMAGE)

# akt builds with nolink_libwasmvm, so the CosmWasm VM is never linked and none
# of its static archives are fetched. release-libs is kept as a no-op so the
# release targets and any external caller keep working.
.PHONY: release-libs
release-libs:

# The container can see only the repository mount. Default an unset GOWORK to
# off, and reject every non-off value except this repository's own go.work.
# In particular, `auto` and a parent workspace must not become a nonexistent
# $(GORELEASER_WORKDIR)/go.work inside the container.
.PHONY: release-workspace-check
release-workspace-check:
	@set -eu; \
	host_gowork="$${GOWORK:-off}"; \
	if [ -z "$$host_gowork" ] || [ "$$host_gowork" = off ]; then \
		exit 0; \
	fi; \
	case "$$host_gowork" in \
		/*) ;; \
		*) host_gowork="$$PWD/$$host_gowork" ;; \
	esac; \
	host_dir=$$(cd "$$(dirname "$$host_gowork")" 2>/dev/null && pwd -P) || { \
		echo "GOWORK must name the repository-root go.work or be off" >&2; \
		exit 1; \
	}; \
	host_gowork="$$host_dir/$$(basename "$$host_gowork")"; \
	repo_root=$$(cd "$(AKT_ROOT)" && pwd -P); \
	expected="$$repo_root/go.work"; \
	if [ "$$host_gowork" != "$$expected" ] || [ ! -f "$$expected" ]; then \
		echo "GOWORK must name the existing repository-root go.work or be off" >&2; \
		echo "got: $$host_gowork" >&2; \
		exit 1; \
	fi

# Validate .goreleaser.yaml. Cheap, needs no build.
#
# `goreleaser check` exits non-zero on a deprecated property even when the
# config is otherwise valid, and akt uses one deliberately: `brews`, to publish
# a Homebrew formula rather than a cask (see .goreleaser.yaml for why). Failing
# the gate on that would mean either dropping the gate or shipping a cask, so
# the deprecation-only outcome is tolerated -- and only that one. GoReleaser's
# exit status 2 and its two exact error diagnostics distinguish that result
# from a config that is both invalid and deprecated. Any other status or extra
# error diagnostic fails. GoReleaser Pro v2.15.2 reports the same deprecation as
# a warning and exits 0, which passes before any of that is consulted.
.PHONY: release-check
release-check: release-workspace-check $(GORELEASER_DOCKER_CONFIG)/config.json
	@out=$$($(GORELEASER) check $(GORELEASER_ARGS) 2>&1); status=$$?; \
	printf '%s\n' "$$out"; \
	if [ $$status -eq 0 ]; then exit 0; fi; \
	deprecated_count=$$(printf '%s\n' "$$out" | grep -F -c \
		'error=configuration is valid, but uses deprecated properties' || true); \
	summary_count=$$(printf '%s\n' "$$out" | grep -F -c \
		'error=1 out of 1 configuration file(s) have issues' || true); \
	error_count=$$(printf '%s\n' "$$out" | grep -F -c 'error=' || true); \
	if [ $$status -eq 2 ] && [ "$$deprecated_count" -eq 1 ] && \
		[ "$$summary_count" -eq 1 ] && [ "$$error_count" -eq 2 ]; then \
		echo "release-check: passing on deprecation warnings only"; \
		exit 0; \
	fi; \
	exit $$status

# Full cross-compile with a synthetic version, no tag and no publishing.
# Artifacts land in .cache/goreleaser.
.PHONY: release-snapshot
release-snapshot: release-libs release-workspace-check $(GORELEASER_DOCKER_CONFIG)/config.json
	$(GORELEASER) release --clean --snapshot --skip=publish,validate $(GORELEASER_ARGS)

# The same prepare step as release-prepare, without its publication preflight:
# it needs only GORELEASER_KEY and a tag reachable from HEAD, publishes
# nothing, and `--skip=validate` allows running it from a dirty tree.
.PHONY: release-dry-run
release-dry-run: release-libs release-workspace-check $(GORELEASER_DOCKER_CONFIG)/config.json
	docker run $(GORELEASER_DOCKER_ARGS) -e GORELEASER_KEY $(GORELEASER_IMAGE) release --clean --prepare --skip=validate $(GORELEASER_ARGS)

# The real thing, in the two steps .github/workflows/release.yml runs on `v*`
# tag pushes (SPEC §12.7). The repository's GitHub releases are immutable --
# sealed when published -- so everything a release ships has to go out in one
# publication. release-prepare builds the release into $(GORELEASER_DIST)
# without touching GitHub; release.yml then generates, dry-runs and attests
# install.sh; release-publish uploads the assets and install.sh together and
# only then updates the Homebrew formula. There is deliberately no one-shot
# target that could publish a release without its installer.
#
# Both steps need GORELEASER_KEY and run the publication preflight, so a
# missing credential fails before the build. Only release-publish hands
# GitHub and tap tokens to its container. Running it by hand needs a
# GITHUB_TOKEN with contents:write. Tokens are passed by name so they never
# land in the echoed command line.
#
# GORELEASER_ACCESS_TOKEN writes the Homebrew formula to
# akash-network/homebrew-tap, which GITHUB_TOKEN cannot reach. Needs
# contents:write. Only stable releases touch the tap (skip_upload: auto), so a
# prerelease succeeds without it. Stable releases reject a missing token before
# GoReleaser starts, so GitHub assets cannot be published before the Homebrew
# update discovers that it has no credential.
.PHONY: release-publish-preflight
release-publish-preflight: release-workspace-check changelog-release-check
	@if [ -z "$${GITHUB_TOKEN}" ]; then \
		echo "GITHUB_TOKEN is required to publish a release"; \
		exit 1; \
	fi
	@if [ -z "$${GORELEASER_KEY:-}" ]; then \
		echo "GORELEASER_KEY (the GoReleaser Pro license) is required to publish a release"; \
		exit 1; \
	fi
	@set -eu; \
	exact_tags=$$(git tag --points-at HEAD | LC_ALL=C sort); \
	tag_count=$$(printf '%s\n' "$$exact_tags" | sed '/^$$/d' | wc -l | tr -d ' '); \
	if [ "$$tag_count" -ne 1 ]; then \
		echo "release commit must have exactly one tag; found $$tag_count" >&2; \
		printf '%s\n' "$$exact_tags" >&2; \
		exit 1; \
	fi; \
	tag="$$exact_tags"; \
	if [ "$$($(SEMVER) validate "$$tag")" != valid ] || [ "$${tag#v}" = "$$tag" ]; then \
		echo "release tag is not a lowercase-v semantic version: $$tag" >&2; \
		exit 1; \
	fi; \
	prerelease=$$($(SEMVER) get prerel "$$tag"); \
	if [ -z "$$prerelease" ] && [ -z "$${GORELEASER_ACCESS_TOKEN:-}" ]; then \
		echo "GORELEASER_ACCESS_TOKEN is required before publishing stable release $$tag" >&2; \
		exit 1; \
	fi

.PHONY: release-prepare
release-prepare: release-libs release-publish-preflight $(GORELEASER_DOCKER_CONFIG)/config.json
	docker run $(GORELEASER_DOCKER_ARGS) -e GORELEASER_KEY $(GORELEASER_IMAGE) release --clean --prepare $(GORELEASER_ARGS)

# `publish` loads the prepared context from --dist, whose default is ./dist
# rather than the `dist` in .goreleaser.yaml, and re-reads the environment.
# GoReleaser resolves release.extra_files here, so the installer is checked
# first (release-installer-verify): a missing one would fail only after the
# release had been created, and a stale one would be sealed into it. Whether
# the installer is attested is not checked here; release.yml runs this only
# after its attestation step succeeds.
.PHONY: release-publish
release-publish: release-publish-preflight release-installer-verify $(GORELEASER_DOCKER_CONFIG)/config.json
	GORELEASER_ACCESS_TOKEN="$${GORELEASER_ACCESS_TOKEN:-}" \
	docker run $(GORELEASER_DOCKER_ARGS) -e GITHUB_TOKEN -e GORELEASER_ACCESS_TOKEN -e GORELEASER_KEY $(GORELEASER_IMAGE) publish --dist ./.cache/goreleaser $(GORELEASER_PUBLISH_ARGS)

# Script installation (SPEC §2.15, §12.7). .config/binstaller.yml is the only
# installer source: install.sh at the repository root is binst's generic
# output for it, and release-installer derives the per-release installer the
# publish job attests -- pinned to one tag, with that release's archive
# checksums embedded.
BINSTALLER_CONFIG     := $(AKT_ROOT)/.config/binstaller.yml
INSTALLER_SCRIPT      := $(AKT_ROOT)/install.sh
# Fixed paths under the checkout, not $(AKT_DEVCACHE): `dist` and
# `release.extra_files` in .goreleaser.yaml, release-publish's --dist, and
# release.yml's dry-run and attest steps name them.
GORELEASER_DIST       := $(AKT_ROOT)/.cache/goreleaser
RELEASE_INSTALLER_DIR := $(AKT_ROOT)/.cache/installer
# Tag the release installer must pin. Empty accepts the tag of the single
# checksum manifest in GoReleaser's dist directory (snapshot and dry runs).
RELEASE_TAG           ?=

.PHONY: installer
installer: $(BINST)
	$(BINST) gen --config "$(BINSTALLER_CONFIG)" -o "$(INSTALLER_SCRIPT)" --quiet

# Fails unless install.sh is exactly what the pinned binst generates from the
# config, so neither can change without the other.
.PHONY: installer-check
installer-check: $(BINST)
	@set -eu; \
	generated="$(AKT_DEVCACHE)/installer-check/install.sh"; \
	mkdir -p "$$(dirname "$$generated")"; \
	$(BINST) gen --config "$(BINSTALLER_CONFIG)" -o "$$generated" --quiet; \
	if ! diff -u "$(INSTALLER_SCRIPT)" "$$generated"; then \
		echo "install.sh does not match .config/binstaller.yml; run 'make installer' and commit the result" >&2; \
		exit 1; \
	fi

# Writes $(RELEASE_INSTALLER_DIR)/install.sh and attestation-subjects.txt from
# the checksum manifest of the preceding GoReleaser run.
.PHONY: release-installer
release-installer: $(BINST)
	"$(AKT_ROOT)/script/release-installer.sh" "$(BINST)" "$(BINSTALLER_CONFIG)" \
		"$(GORELEASER_DIST)" "$(RELEASE_INSTALLER_DIR)" "$(RELEASE_TAG)"

# Fails unless install.sh and attestation-subjects.txt are what
# release-installer generated from the checksum manifest now in
# $(GORELEASER_DIST): the subject list must carry both the manifest's and the
# installer's digest. An installer left over from an earlier release, or edited
# since, would otherwise be sealed into this one.
.PHONY: release-installer-verify
release-installer-verify:
	@set -eu; \
	sha256() { if command -v sha256sum >/dev/null; then sha256sum "$$1"; else shasum -a 256 "$$1"; fi | cut -d' ' -f1; }; \
	installer="$(RELEASE_INSTALLER_DIR)/install.sh"; \
	subjects="$(RELEASE_INSTALLER_DIR)/attestation-subjects.txt"; \
	set -- "$(GORELEASER_DIST)"/akt_*_checksums.txt; \
	if [ $$# -ne 1 ] || [ ! -f "$$1" ]; then \
		echo "expected exactly one akt_<version>_checksums.txt in $(GORELEASER_DIST)" >&2; \
		exit 1; \
	fi; \
	manifest=$$1; \
	if [ ! -f "$$installer" ] || [ ! -f "$$subjects" ]; then \
		echo "$$installer or its attestation subject list is missing; run make release-installer" >&2; \
		exit 1; \
	fi; \
	for subject in "$$(sha256 "$$manifest")  $${manifest##*/}" "$$(sha256 "$$installer")  install.sh"; do \
		if ! grep -qxF "$$subject" "$$subjects"; then \
			echo "$$installer does not belong to the prepared release; run make release-installer" >&2; \
			exit 1; \
		fi; \
	done

.PHONY: bins
bins: $(AKT)

.PHONY: $(AKT)
$(AKT):
	$(GO_BUILD) $(BUILD_FLAGS) -o $@ ./cmd/akt

.PHONY: akt
akt: $(AKT)
