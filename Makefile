# Go is not always installed on the host. Then use the toolchain that prek downloads.
GO ?= $(or $(shell command -v go),$(lastword $(sort $(wildcard $(HOME)/.cache/prek/tools/go/*/bin/go))))

OAI_DB_PATH    ?= data/oai.db
OAI_SEEDS_PATH ?= seeds.txt
export OAI_DB_PATH OAI_SEEDS_PATH OAI_CONTACT_URL OAI_LISTEN_ADDR

.PHONY: help run-crawler run-api test build lint check clean go-check

help: ## Show the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

run-crawler: go-check ## Run one crawl (OAI_DB_PATH, OAI_SEEDS_PATH)
	@mkdir -p $(dir $(OAI_DB_PATH))
	$(GO) run ./cmd/crawler

run-api: go-check ## Run the API
	$(GO) run ./cmd/api

test: go-check ## Run the unit tests with the race detector
	$(GO) test -race ./...

build: go-check ## Build the binaries into bin/
	$(GO) build -o bin/ ./cmd/...

lint: ## Run the pre-commit checks on all files
	prek run --all-files

check: ## Run the pre-push checks on all files
	prek run --all-files --hook-stage pre-push

clean: ## Remove bin/ and data/
	rm -rf bin data

go-check:
	@test -n "$(GO)" || { echo "Go not found. Install Go, or run 'make lint' once so that prek downloads it."; exit 1; }
