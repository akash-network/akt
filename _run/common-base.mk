#include $(abspath $(CURDIR)/../../make/init.mk)

ifeq ($(AKT_RUN_NAME),)
$(error "AKT_RUN_NAME is not set")
endif

ifeq ($(AKT_RUN_DIR),)
$(error "AKT_RUN_DIR is not set")
endif

ifneq ($(AKT_HOME),)
ifneq ($(DIRENV_FILE),$(CURDIR)/.envrc)
$(error "AKT_HOME is set by the upper dir (probably in ~/.bashrc|~/.zshrc), \
but direnv does not seem to be configured. \
Ensure direnv is installed and hooked to your shell profile. Refer to the documentation for details. \
")
endif
else
$(error "AKT_HOME is not set")
endif

.PHONY: akt
akt:
ifneq ($(SKIP_BUILD), true)
	make -C $(AKT_ROOT) akt
endif

.PHONY: bins
bins: akt
