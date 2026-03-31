AKT_INIT                     := $(AKT_RUN_DIR)/.akt-init

$(AKT_RUN_DIR):
	mkdir -p $@

$(AKT_CONTEXT):
	mkdir -p $@

$(AKT_INIT): $(AKT_CONTEXT)
	touch $@

.INTERMEDIATE: akt-init
akt-init: $(AKT_INIT)
