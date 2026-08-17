# Release tooling.
#
# goreleaser is run from the goreleaser-cross container image rather than as a
# local binary: the darwin targets need osxcross and the linux targets need the
# gnu cross toolchains, and we do not want either as a developer prerequisite.
# Docker is therefore required for every release-* target below.

# goreleaser-cross publishes one image per Go patch release, but not for every
# patch -- there is no v1.26.1 image even though go.mod pins go 1.26.1. Keep the
# readable tag and immutable index digest together, and update both only after
# inspecting the published multi-platform manifest.
GORELEASER_CROSS_VERSION ?= v1.26.2
GORELEASER_CROSS_DIGEST  ?= sha256:fadba0d4577866eb2588d46ea6b604c73ef45ee55f044acbc17cc49aa435fd04
GORELEASER_IMAGE         := ghcr.io/goreleaser/goreleaser-cross:$(GORELEASER_CROSS_VERSION)@$(GORELEASER_CROSS_DIGEST)

# The cross image is published for linux/amd64 only; be explicit so arm64 hosts
# emulate instead of failing to find a manifest.
GORELEASER_PLATFORM      ?= linux/amd64

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
	--platform $(GORELEASER_PLATFORM) \
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
# error diagnostic fails.
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

# Same as `release` but stops short of uploading. Needs a tag reachable from
# HEAD; `--skip=validate` allows running it from a dirty tree.
.PHONY: release-dry-run
release-dry-run: release-libs release-workspace-check $(GORELEASER_DOCKER_CONFIG)/config.json
	$(GORELEASER) release --clean --skip=publish,validate $(GORELEASER_ARGS)

# The real thing: builds the matrix and uploads to the GitHub release for the
# current tag. Driven by .github/workflows/release.yml on `v*` tag pushes;
# running it by hand needs a GITHUB_TOKEN with contents:write. Tokens are
# passed by name so they never land in the echoed command line.
#
# GORELEASER_ACCESS_TOKEN writes the Homebrew formula to
# akash-network/homebrew-tap, which GITHUB_TOKEN cannot reach. Needs
# contents:write. Only stable releases touch the tap (skip_upload: auto), so a
# prerelease succeeds without it. Stable releases reject a missing token before
# GoReleaser starts, so GitHub assets cannot be published before the Homebrew
# update discovers that it has no credential.
.PHONY: release-publish-preflight
release-publish-preflight: release-workspace-check
	@if [ -z "$${GITHUB_TOKEN}" ]; then \
		echo "GITHUB_TOKEN is required to publish a release"; \
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

.PHONY: release
release: release-libs release-publish-preflight $(GORELEASER_DOCKER_CONFIG)/config.json
	GORELEASER_ACCESS_TOKEN="$${GORELEASER_ACCESS_TOKEN:-}" \
	docker run $(GORELEASER_DOCKER_ARGS) -e GITHUB_TOKEN -e GORELEASER_ACCESS_TOKEN $(GORELEASER_IMAGE) release --clean $(GORELEASER_ARGS)

.PHONY: bins
bins: $(AKT)

.PHONY: $(AKT)
$(AKT):
	$(GO_BUILD) $(BUILD_FLAGS) -o $@ ./cmd/akt

.PHONY: akt
akt: $(AKT)
