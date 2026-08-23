SHELL := /bin/bash

ROOT_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
SERVER_DIR := $(ROOT_DIR)seven-framework-server
WEB_DIR := $(ROOT_DIR)seven-framework-web
VERSION ?= dev

.PHONY: help server-check web-install web-check check build release security-scan clean

help:
	@echo "Seven Framework monorepo"
	@echo "  make server-check   Format, vet, and test the Go service"
	@echo "  make web-install    Install frozen frontend dependencies"
	@echo "  make web-check      Type-check, lint, and build the Web app"
	@echo "  make check          Run server and Web checks"
	@echo "  make release        Build a local release package"

server-check:
	$(MAKE) -C "$(SERVER_DIR)" fmt-check
	$(MAKE) -C "$(SERVER_DIR)" vet
	$(MAKE) -C "$(SERVER_DIR)" test
	"$(ROOT_DIR)scripts/check-ddd.sh"

web-install:
	cd "$(WEB_DIR)" && pnpm install --frozen-lockfile

web-check:
	cd "$(WEB_DIR)" && pnpm exec tsc -b
	cd "$(WEB_DIR)" && pnpm lint
	cd "$(WEB_DIR)" && pnpm build

check: server-check web-check

build:
	$(MAKE) -C "$(SERVER_DIR)" build-release VERSION="$(VERSION)"
	cd "$(WEB_DIR)" && pnpm build

release:
	"$(ROOT_DIR)scripts/release/build.sh" "$(VERSION)"

security-scan:
	gitleaks detect --source "$(ROOT_DIR)" --no-banner --redact

clean:
	$(MAKE) -C "$(SERVER_DIR)" clean
	rm -rf "$(WEB_DIR)dist" "$(ROOT_DIR)dist"
