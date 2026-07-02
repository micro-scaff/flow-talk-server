GO_CACHE_DIR := $(CURDIR)/.gocache
GO_ENV := GOCACHE=$(GO_CACHE_DIR)
GO_PATH := $(shell go env GOPATH)
FRESH_BIN := $(GO_PATH)/bin/fresh

.PHONY: run test vet fresh cache-env

run:
	$(GO_ENV) go run .

test:
	$(GO_ENV) go test ./...

vet:
	$(GO_ENV) go vet ./...

fresh:
	$(GO_ENV) $(FRESH_BIN)

cache-env:
	go env -w GOCACHE=$(GO_CACHE_DIR)
