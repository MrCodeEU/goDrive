GOCACHE ?= /tmp/godrive-gocache
GOFLAGS ?=

ifneq (,$(wildcard .env))
include .env
export
endif

GODRIVE_DEV_LATENCY ?= 10ms-25ms

.PHONY: test test-cover test-race tidy run web-install web-dev web-check web-build web-test

test:
	CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) go test $(GOFLAGS) ./...

test-cover:
	CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) go test -cover $(GOFLAGS) ./...

test-race:
	CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) go test -race $(GOFLAGS) ./...

tidy:
	go mod tidy

run:
	CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) GODRIVE_DEV_LATENCY=$(GODRIVE_DEV_LATENCY) go run ./cmd/godrive

web-install:
	npm install --prefix web

web-dev:
	npm run dev --prefix web

web-check:
	npm run check --prefix web

web-build:
	npm run build --prefix web

web-test:
	npm run test --prefix web
