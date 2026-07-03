GO_CACHE_DIR := $(CURDIR)/.gocache
GO_MODCACHE_DIR := $(CURDIR)/.gomodcache
GO_BIN_DIR := $(CURDIR)/.bin
GO_PROXY ?= https://goproxy.cn,direct
GO_TOOLCHAIN ?= local
GO_ENV := GOCACHE=$(GO_CACHE_DIR) GOMODCACHE=$(GO_MODCACHE_DIR) GOBIN=$(GO_BIN_DIR) GOPROXY=$(GO_PROXY) GOTOOLCHAIN=$(GO_TOOLCHAIN)
FRESH_BIN := $(GO_BIN_DIR)/fresh
FRESH_VERSION := v0.0.0-20240621171608-8d1fef547a99

.PHONY: run tidy test vet fresh install-fresh

run:
	$(GO_ENV) go run .

tidy:
	$(GO_ENV) go mod tidy

test:
	$(GO_ENV) go test ./...

vet:
	$(GO_ENV) go vet ./...

fresh:
	@if [ -x "$(FRESH_BIN)" ]; then \
		$(GO_ENV) "$(FRESH_BIN)"; \
	elif command -v fresh >/dev/null 2>&1; then \
		$(GO_ENV) fresh; \
	else \
		$(MAKE) install-fresh; \
		$(GO_ENV) "$(FRESH_BIN)"; \
	fi

install-fresh:
	@if [ -x "$(FRESH_BIN)" ]; then \
		echo "fresh already installed at $(FRESH_BIN)"; \
	elif command -v fresh >/dev/null 2>&1; then \
		echo "fresh already available at $$(command -v fresh)"; \
	else \
		mkdir -p $(GO_BIN_DIR); \
		$(GO_ENV) go install github.com/pilu/fresh@$(FRESH_VERSION); \
	fi
