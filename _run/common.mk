OPTIONS ?=

SKIP_BUILD := false

# check for nostrip option
ifneq (,$(findstring nobuild,$(OPTIONS)))
	SKIP_BUILD := true
endif

include ../common-base.mk

AKT_INIT                     := $(AKT_RUN_DIR)/.akt-init

$(AKT_RUN_DIR):
	mkdir -p $@

$(AKT_CONTEXT):
	mkdir -p $@

$(AKT_INIT): $(AKT_CONTEXT)
	touch $@

.INTERMEDIATE: akt-init
akt-init: $(AKT_INIT)

.PHONY: clean
clean: clean-$(AKT_RUN_NAME)
	rm -rf "$(AKT_RUN)/$(AKT_RUN_NAME)"
