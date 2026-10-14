# Go is not always installed on the host. Then use the toolchain that prek downloads.
GO ?= $(or $(shell command -v go),$(lastword $(sort $(wildcard $(HOME)/.cache/prek/tools/go/*/bin/go))))

.PHONY: help run-crawler run-api run-web build-web test build lint check clean go-check

help: ## Show the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

run-crawler: go-check ## Run one crawl (config: oai.toml, OAI_* variables)
	@mkdir -p data
	$(GO) run ./cmd/crawler

run-api: go-check ## Run the API (config: oai.toml, OAI_* variables)
	$(GO) run ./cmd/api

run-web: web/node_modules ## Run the website on http://localhost:8000 (OAI_WEB_PORT). Needs run-api.
	npm --prefix web run dev

build-web: web/node_modules ## Build the website into web/dist (needs VITE_API_URL)
	npm --prefix web run build

web/node_modules: web/package-lock.json
	npm --prefix web ci
	@touch $@

test: go-check ## Run the unit tests with the race detector
	$(GO) test -race ./...

build: go-check ## Build the binaries into bin/
	$(GO) build -o bin/ ./cmd/...

lint: ## Run the pre-commit checks on all files
	prek run --all-files

check: ## Run the pre-push checks on all files
	prek run --all-files --hook-stage pre-push

clean: ## Remove bin/, data/ and web/dist/
	rm -rf bin data web/dist

go-check:
	@test -n "$(GO)" || { echo "Go not found. Install Go, or run 'make lint' once so that prek downloads it."; exit 1; }
