SHELL := /bin/sh

KO ?= ko

KO_DOCKER_REPO ?= ghcr.io/riftonix
IMPORT_PATH ?= ./cmd/kelm

.DEFAULT_GOAL := help

.PHONY: help build check-version publish-version release

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build and publish :latest with ko
	KO_DOCKER_REPO=$(KO_DOCKER_REPO) $(KO) build $(IMPORT_PATH) --base-import-paths --tags latest

check-version:
	@test -n "$(VERSION)" || (echo "VERSION is required. Example: make release VERSION=0.1.3"; exit 1)

release: check-version ## Publish :$(VERSION) and :latest in one ko push
	KO_DOCKER_REPO=$(KO_DOCKER_REPO) $(KO) build $(IMPORT_PATH) --base-import-paths --tags $(VERSION) --tags latest
