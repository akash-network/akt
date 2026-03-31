$(AKT_DEVCACHE):
	@echo "creating .cache dir structure..."
	mkdir -p $@
	mkdir -p $(AKT_DEVCACHE_BIN)
	mkdir -p $(AKT_DEVCACHE_LIB)
	mkdir -p $(AKT_DEVCACHE_INCLUDE)
	mkdir -p $(AKT_DEVCACHE_VERSIONS)
	mkdir -p $(AKT_DEVCACHE_NODE_MODULES)
	mkdir -p $(AKT_RUN_BIN)
cache: $(AKT_DEVCACHE)

$(GIT_CHGLOG_VERSION_FILE): $(AKT_DEVCACHE)
	@echo "installing git-chglog $(GIT_CHGLOG_VERSION) ..."
	rm -f $(GIT_CHGLOG)
	GOBIN=$(AKT_DEVCACHE_BIN) go install github.com/git-chglog/git-chglog/cmd/git-chglog@$(GIT_CHGLOG_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(GIT_CHGLOG): $(GIT_CHGLOG_VERSION_FILE)

GOLANGCI_LINT_MAJOR=$(shell $(SEMVER) get major $(GOLANGCI_LINT_VERSION))
$(GOLANGCI_LINT_VERSION_FILE): $(AKT_DEVCACHE)
	@echo "installing golangci-lint $(GOLANGCI_LINT_VERSION) ..."
	rm -f $(GOLANGCI_LINT)
	GOBIN=$(AKT_DEVCACHE_BIN) go install github.com/golangci/golangci-lint/v$(GOLANGCI_LINT_MAJOR)/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(GOLANGCI_LINT): $(GOLANGCI_LINT_VERSION_FILE)

cache-clean:
	rm -rf $(AKT_DEVCACHE)

$(AKT_DEVCACHE_LIB)/%:
