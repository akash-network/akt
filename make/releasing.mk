# Release tooling.
#
# goreleaser is run from the goreleaser-cross container image rather than as a
# local binary: the darwin targets need osxcross and the linux targets need the
# gnu cross toolchains, and we do not want either as a developer prerequisite.
# Docker is therefore required for every release-* target below.

# goreleaser-cross publishes one image per Go patch release, but not for every
# patch -- there is no v1.26.1 image even though go.mod pins go 1.26.1 -- so the
# tag is pinned here instead of being derived from GOTOOLCHAIN_SEMVER. Bump it
# together with the go directive in go.mod, after checking the tag exists:
#   docker manifest inspect ghcr.io/goreleaser/goreleaser-cross:vX.Y.Z
GORELEASER_CROSS_VERSION ?= v1.26.2
GORELEASER_IMAGE         := ghcr.io/goreleaser/goreleaser-cross:$(GORELEASER_CROSS_VERSION)

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

GORELEASER_GOWORK        := $(GOWORK)

ifneq ($(GOWORK), off)
	GORELEASER_GOWORK    := $(GORELEASER_WORKDIR)/go.work
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

# The Go module for libwasmvm ships only shared objects (.so/.dylib) that are
# linked with an rpath into the module cache, which is useless in a released
# binary. The static archives that the `muslc` (linux) and `static_wasm`
# (darwin) build tags link against are published on the wasmvm release page
# instead, so fetch them into .cache/lib -- that is the directory every
# `-extldflags "-L./.cache/lib ..."` in .goreleaser.yaml points at.
WASMVM_VERSION  = $(shell go list -m -f '{{ .Version }}' github.com/CosmWasm/wasmvm/v3)
WASMVM_LIBS     := libwasmvm_muslc.x86_64.a libwasmvm_muslc.aarch64.a libwasmvmstatic_darwin.a
RELEASE_LIBS    := $(patsubst %,$(AKT_DEVCACHE_LIB)/%,$(WASMVM_LIBS))

# More specific than the catch-all $(AKT_DEVCACHE_LIB)/% rule in setup-cache.mk,
# so make prefers this one for the libwasmvm archives.
$(AKT_DEVCACHE_LIB)/libwasmvm%: | $(AKT_DEVCACHE)
	@echo "fetching $(@F) ($(WASMVM_VERSION)) ..."
	curl -sSfL -o $@ \
		https://github.com/CosmWasm/wasmvm/releases/download/$(WASMVM_VERSION)/$(@F)

.PHONY: release-libs
release-libs: $(RELEASE_LIBS)

# Validate .goreleaser.yaml. Cheap, needs no build.
.PHONY: release-check
release-check: $(GORELEASER_DOCKER_CONFIG)/config.json
	$(GORELEASER) check $(GORELEASER_ARGS)

# Full cross-compile with a synthetic version, no tag and no publishing.
# Artifacts land in .cache/goreleaser.
.PHONY: release-snapshot
release-snapshot: release-libs $(GORELEASER_DOCKER_CONFIG)/config.json
	$(GORELEASER) release --clean --snapshot --skip=publish,validate $(GORELEASER_ARGS)

# Same as `release` but stops short of uploading. Needs a tag reachable from
# HEAD; `--skip=validate` allows running it from a dirty tree.
.PHONY: release-dry-run
release-dry-run: release-libs $(GORELEASER_DOCKER_CONFIG)/config.json
	$(GORELEASER) release --clean --skip=publish,validate $(GORELEASER_ARGS)

# The real thing: builds the matrix and uploads to the GitHub release for the
# current tag. Driven by .github/workflows/release.yml on `v*` tag pushes;
# running it by hand needs a GITHUB_TOKEN with contents:write. Tokens are
# passed by name so they never land in the echoed command line.
#
# HOMEBREW_TAP_TOKEN publishes the cask to akash-network/homebrew-tap, which is
# a separate repository that GITHUB_TOKEN cannot reach. It is defaulted to the
# empty string rather than passed through as-is: `token: "{{ .Env.X }}"` fails
# to render when X is absent from the environment entirely, which would abort a
# release that was never going to touch the tap. Empty renders fine, and
# `skip_upload: auto` means a prerelease skips the upload regardless — so an
# alpha releases without the secret, while a stable release without it fails at
# the cask step with an auth error rather than a template error.
.PHONY: release
release: release-libs $(GORELEASER_DOCKER_CONFIG)/config.json
	@if [ -z "$${GITHUB_TOKEN}" ]; then \
		echo "GITHUB_TOKEN is required to publish a release"; \
		exit 1; \
	fi
	@if [ -z "$${HOMEBREW_TAP_TOKEN}" ]; then \
		echo "warning: HOMEBREW_TAP_TOKEN unset -- a stable release will fail at the Homebrew cask step"; \
	fi
	HOMEBREW_TAP_TOKEN="$${HOMEBREW_TAP_TOKEN:-}" \
	docker run $(GORELEASER_DOCKER_ARGS) -e GITHUB_TOKEN -e HOMEBREW_TAP_TOKEN $(GORELEASER_IMAGE) release --clean $(GORELEASER_ARGS)

.PHONY: bins
bins: $(AKT)

.PHONY: $(AKT)
$(AKT):
	$(GO_BUILD) $(BUILD_FLAGS) -o $@ ./cmd/akt

.PHONY: akt
akt: $(AKT)
