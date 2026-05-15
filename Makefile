GOCACHE ?= /tmp/godrive-gocache
GOFLAGS ?=

ifneq (,$(wildcard .env))
include .env
export
endif

GODRIVE_DEV_LATENCY ?= 10ms-25ms

ANDROID_SDK      ?= $(HOME)/Android/Sdk
EMULATOR         ?= $(ANDROID_SDK)/emulator/emulator
FLUTTER          ?= $(HOME)/develop/flutter/bin/flutter
AVD_NAME         ?= Pixel_10
ANDROID_DEVICE   ?= emulator-5554
# Android Studio Flatpak stores AVDs here instead of ~/.android/avd
ANDROID_AVD_HOME ?= $(HOME)/.var/app/com.google.AndroidStudio/config/.android/avd

.PHONY: fmt fmt-check vet test test-cover test-race tidy golangci lint check run \
        web-install web-dev web-check web-build web-test \
        mobile-install mobile-test mobile-build-android \
        emulator-start emulator-wait mobile-run mobile-dev

GO_PACKAGES := ./...
GO_FILES := $(shell find cmd internal -name '*.go' -type f)

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@test -z "$$(gofmt -l $(GO_FILES))" || (gofmt -l $(GO_FILES); exit 1)

vet:
	CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) go vet $(GOFLAGS) $(GO_PACKAGES)

test:
	CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) go test $(GOFLAGS) $(GO_PACKAGES)

test-cover:
	CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) go test -cover $(GOFLAGS) $(GO_PACKAGES)

test-race:
	CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) go test -race $(GOFLAGS) $(GO_PACKAGES)

tidy:
	go mod tidy

golangci:
	GOLANGCI_LINT_CACHE=/tmp/godrive-golangci-lint-cache golangci-lint run $(GO_PACKAGES)

lint: fmt-check vet golangci web-check

check: lint test web-test web-build

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

mobile-install:
	flutter pub get --directory mobile

mobile-test:
	flutter test --directory mobile

mobile-build-android:
	$(FLUTTER) build apk --debug --project-dir mobile

emulator-start:
	@echo "Starting emulator $(AVD_NAME)..."
	@ANDROID_AVD_HOME=$(ANDROID_AVD_HOME) $(EMULATOR) -avd $(AVD_NAME) -no-snapshot-save -no-audio &
	@echo "Waiting for device boot..."
	@adb wait-for-device
	@until adb -s $(ANDROID_DEVICE) shell getprop sys.boot_completed 2>/dev/null | grep -q "^1"; do sleep 2; done
	@echo "Emulator ready."

mobile-run:
	cd mobile && $(FLUTTER) run -d $(ANDROID_DEVICE)

# Start emulator + backend + app in one command.
# Backend runs without dev latency so mobile feels fast.
# If backend already running on 8121, skip start.
# Usage: make mobile-dev
mobile-dev: emulator-start
	@echo "Starting backend (no latency)..."
	@if ! lsof -ti:8121 > /dev/null 2>&1; then \
		GODRIVE_DEV_LATENCY= CCACHE_DISABLE=1 GOCACHE=$(GOCACHE) go run ./cmd/godrive & \
		sleep 2; \
	else \
		echo "Backend already running on :8121"; \
	fi
	cd mobile && $(FLUTTER) run -d $(ANDROID_DEVICE)
