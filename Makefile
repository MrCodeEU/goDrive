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

# ─── iOS sideload via GitHub Actions + xtool ───────────────────────────────
# xtool distributes as an AppImage — works on Fedora Atomic without rpm-ostree.
# APPIMAGE_EXTRACT_AND_RUN=1 skips FUSE, which may be unavailable on atomic distros.
# usbmuxd must be running on the host (socket at /var/run/usbmuxd).
#   Fedora Atomic: rpm-ostree install usbmuxd && systemctl reboot
XTOOL_VERSION  := 1.16.1
XTOOL_BIN      := $(HOME)/.local/bin/xtool.AppImage
XTOOL          := APPIMAGE_EXTRACT_AND_RUN=1 $(XTOOL_BIN)
IOS_BRANCH     := ios-dev
IOS_ARTIFACT   := godrive-ios
IOS_IPA        := /tmp/godrive-ios/godrive.ipa

.PHONY: fmt fmt-check vet test test-cover test-race tidy golangci lint check api-contract run \
        security security-go security-web security-osv security-docker \
        web-install web-dev web-check web-build web-test \
        mobile-install mobile-test mobile-build-android mobile-build-android-release \
        emulator-start emulator-wait mobile-run mobile-dev \
        xtool-setup xtool-auth ios-push ios-deploy ios-refresh ios-devices \
        install-hooks

GO_PACKAGES := ./...
GO_FILES := $(shell find cmd internal -name '*.go' -type f)

# Install git pre-commit hook (symlink so script updates apply immediately).
install-hooks:
	ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit
	chmod +x scripts/pre-commit.sh
	@echo "Pre-commit hook installed."

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

check: lint test web-test web-build api-contract

api-contract:
	ruby scripts/check-openapi-routes.rb

security: security-go security-web security-osv

security-go:
	go run golang.org/x/vuln/cmd/govulncheck@latest $(GO_PACKAGES)

security-web:
	npm audit --prefix web --audit-level=moderate

security-osv:
	@if ! command -v osv-scanner >/dev/null 2>&1; then \
		echo "osv-scanner not found. Install from https://google.github.io/osv-scanner/ or rely on the GitHub security workflow."; \
		exit 127; \
	fi
	osv-scanner --lockfile=web/package-lock.json --lockfile=mobile/pubspec.lock

security-docker:
	@if ! command -v grype >/dev/null 2>&1; then \
		echo "grype not found. Install from https://github.com/anchore/grype or rely on the GitHub security workflow."; \
		exit 127; \
	fi
	docker build -t godrive:security .
	grype godrive:security --fail-on high

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
	cd mobile && $(FLUTTER) pub get

mobile-test:
	cd mobile && $(FLUTTER) test

mobile-build-android:
	cd mobile && $(FLUTTER) build apk --debug

mobile-build-android-release:
	cd mobile && $(FLUTTER) build appbundle --release
	cd mobile && $(FLUTTER) build apk --release

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

# ─── iOS targets ────────────────────────────────────────────────────────────

# One-time system setup for iPhone sideloading on Fedora Atomic.
# Requires: rpm-ostree install usbmuxd (+ reboot) if not already installed.
xtool-setup:
	@echo "→ Downloading xtool $(XTOOL_VERSION) AppImage..."
	@mkdir -p $(HOME)/.local/bin
	curl -fSL "https://github.com/xtool-org/xtool/releases/download/$(XTOOL_VERSION)/xtool-x86_64.AppImage" \
		-o $(XTOOL_BIN)
	chmod +x $(XTOOL_BIN)
	@echo "→ Adding Apple USB udev rule (sudo required)..."
	@echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="05ac", MODE="0666"' | \
		sudo tee /etc/udev/rules.d/99-apple-usb.rules > /dev/null
	sudo udevadm control --reload-rules
	@echo ""
	@echo "→ Checking usbmuxd..."
	@if ! systemctl is-active --quiet usbmuxd && ! rpm -q usbmuxd &>/dev/null; then \
		echo "  usbmuxd not found. Install it:"; \
		echo "    rpm-ostree install usbmuxd && systemctl reboot"; \
		echo "  Then re-plug iPhone and run: make xtool-auth"; \
	else \
		echo "  usbmuxd OK"; \
		echo ""; \
		echo "→ Next: plug in iPhone, tap Trust, then run: make xtool-auth"; \
	fi

# Authenticate with Apple ID (once — stored in keychain).
xtool-auth:
	$(XTOOL) auth login

# List connected iOS devices (quick connectivity check).
ios-devices:
	$(XTOOL) devices

# Force-push current HEAD to scratch branch (no history accumulation on main).
ios-push:
	git push origin HEAD:refs/heads/$(IOS_BRANCH) --force

# Full pipeline: push → trigger CI → wait → download → sign + install.
# iPhone must be connected via USB and trusted before running.
ios-deploy: ios-push
	@set -e; \
	echo "→ Triggering iOS build on branch $(IOS_BRANCH)..."; \
	GH_OUT=$$(gh workflow run ios.yml --ref $(IOS_BRANCH) 2>&1); echo "$$GH_OUT"; \
	RUN_ID=$$(echo "$$GH_OUT" | grep -oE 'runs/[0-9]+' | grep -oE '[0-9]+' | head -1); \
	if [ -z "$$RUN_ID" ]; then \
		echo "→ Waiting for run to register..."; \
		sleep 6; \
		RUN_ID=$$(gh run list --workflow=ios.yml --branch=$(IOS_BRANCH) \
			--limit 5 --json databaseId,status \
			-q '.[] | select(.status != "completed") | .databaseId' 2>/dev/null | head -1); \
	fi; \
	[ -z "$$RUN_ID" ] && echo "ERROR: could not get run ID" && exit 1; \
	echo "→ Watching run $$RUN_ID (typically 8-12 min)..."; \
	gh run watch $$RUN_ID --exit-status; \
	echo "→ Downloading IPA..."; \
	rm -rf /tmp/godrive-ios; \
	gh run download $$RUN_ID -n $(IOS_ARTIFACT) --dir /tmp/godrive-ios; \
	echo "→ Signing and installing on device..."; \
	$(XTOOL) install $(IOS_IPA); \
	echo "✓ Done. App installed on iPhone."

# Re-sign and reinstall the last downloaded IPA without rebuilding.
# Use when the 7-day free-account cert expires — skips the full CI build.
ios-refresh:
	@[ -f $(IOS_IPA) ] || (echo "No IPA at $(IOS_IPA) — run make ios-deploy first" && exit 1)
	$(XTOOL) install $(IOS_IPA)
