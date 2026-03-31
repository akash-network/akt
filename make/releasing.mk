GORELEASER_VERBOSE       ?= false
GORELEASER_IMAGE         := ghcr.io/goreleaser/goreleaser-cross:$(GOTOOLCHAIN_SEMVER)
GORELEASER_RELEASE       ?= false
GORELEASER_MOUNT_CONFIG  ?= false
GORELEASER_SKIP          := $(subst $(COMMA),$(SPACE),$(GORELEASER_SKIP))
RELEASE_DOCKER_IMAGE     ?= ghcr.io/akash-network/node
GORELEASER_MOD_MOUNT     ?= $(shell cat $(AKT_ROOT)/.github/repo | tr -d '\n')

RELEASE_DOCKER_IMAGE     ?= ghcr.io/akash-network/node

GORELEASER_GOWORK        := $(GOWORK)

ifneq ($(GOWORK), off)
	GORELEASER_GOWORK    := /go/src/$(GORELEASER_MOD_MOUNT)/go.work
endif

ifneq ($(GORELEASER_RELEASE),true)
	ifeq (,$(findstring publish,$(GORELEASER_SKIP)))
		GORELEASER_SKIP += publish
	endif

	GITHUB_TOKEN=
endif

ifneq (,$(GORELEASER_SKIP))
	GORELEASER_SKIP := --skip=$(subst $(SPACE),$(COMMA),$(strip $(GORELEASER_SKIP)))
endif

ifeq ($(GORELEASER_MOUNT_CONFIG),true)
	GORELEASER_IMAGE := -v $(HOME)/.docker/config.json:/root/.docker/config.json $(GORELEASER_IMAGE)
endif

.PHONY: bins
bins: $(AKT)

.PHONY: $(AKT)
$(AKT):
	$(GO_BUILD) $(BUILD_FLAGS) -o $@ ./cmd/akt

.PHONY: akt
akt: $(AKT)
