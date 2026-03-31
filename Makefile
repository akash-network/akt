GOLANGCI_LINT_VERSION        ?= v2.3.0
GIT_CHGLOG_VERSION           ?= v0.15.

GIT_CHGLOG_VERSION_FILE          := $(AKT_DEVCACHE_VERSIONS)/git-chglog/$(GIT_CHGLOG_VERSION)
GOLANGCI_LINT_VERSION_FILE       := $(AKT_DEVCACHE_VERSIONS)/golangci-lint/$(GOLANGCI_LINT_VERSION)

GIT_CHGLOG                       := $(AKT_DEVCACHE_BIN)/git-chglog
GOLANGCI_LINT                    := $(AKT_DEVCACHE_BIN)/golangci-lint

GOMOD                  ?= readonly

UNAME_OS              := $(shell uname -s)
UNAME_ARCH            := $(shell uname -m)

NULL  :=
SPACE := $(NULL)
WHITESPACE := $(NULL) $(NULL)
COMMA := ,

ifneq ($(UNAME_OS),Darwin)
BUILD_OPTIONS          ?= static-link
endif

BUILD_TAGS             := osusergo netgo ledger muslc gcc

ifneq (,$(findstring cgotrace,$(BUILD_OPTIONS)))
	BUILD_TAGS += cgotrace
endif

build_tags    := $(strip $(BUILD_TAGS))
build_tags_cs := $(subst $(WHITESPACE),$(COMMA),$(build_tags))

GORELEASER_LDFLAGS := $(ldflags)

ldflags :=

ifneq (,$(findstring static-link,$(BUILD_OPTIONS)))
	ldflags += -extldflags "-L$(AKT_DEVCACHE_LIB) -lm -Wl,-z,muldefs"
else
	ldflags +=  -linkmode=external -extldflags "-L$(AKT_DEVCACHE_LIB)"
endif

ldflags += -X github.com/cosmos/cosmos-sdk/version.Name=akt \
-X github.com/cosmos/cosmos-sdk/version.AppName=akt \
-X github.com/cosmos/cosmos-sdk/version.BuildTags="$(build_tags_cs)" \
-X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags | sed 's/^v//') \
-X github.com/cosmos/cosmos-sdk/version.Commit=$(GIT_HEAD_COMMIT_LONG)

# check for nostrip option
ifeq (,$(findstring nostrip,$(BUILD_OPTIONS)))
	ldflags     += -s -w
	BUILD_FLAGS += -trimpath
endif

ifeq (delve,$(findstring delve,$(BUILD_OPTIONS)))
	BUILD_FLAGS += -gcflags "all=-N -l"
endif

ldflags += $(LDFLAGS)
ldflags := $(strip $(ldflags))

GORELEASER_TAGS  := $(BUILD_TAGS)
GORELEASER_FLAGS := $(BUILD_FLAGS) -mod=$(GOMOD) -tags='$(build_tags)'

BUILD_FLAGS += -mod=$(GOMOD) -tags='$(build_tags_cs)' -ldflags '$(ldflags)'

GO                           := GO111MODULE=$(GO111MODULE) go
GO_BUILD                     := $(GO) build -mod=$(GOMOD)
GO_TEST                      := $(GO) test -mod=$(GOMOD)
GO_VET                       := $(GO) vet -mod=$(GOMOD)
GO_MOD_NAME                  := $(shell go list -m 2>/dev/null)

include $(AKT_ROOT)/make/setup-cache.mk
include $(AKT_ROOT)/make/releasing.mk
