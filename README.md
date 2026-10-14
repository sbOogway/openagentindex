# Open Agent Index

Finds, verifies and indexes [A2A](https://a2a-protocol.org) agents, and serves search over them.

Scope: A2A agents only — something is listed only if it serves a valid agent card. No MCP servers, and
the search engine is not itself an agent: its interfaces are a REST API and a static website.

## Design

See [`spec.md`](spec.md) for the scope, the C4 diagrams, the data model and the milestones.

## Layout

```
cmd/crawler/   crawler binary (discover, probe/validate, health checks)
cmd/api/       REST API binary
internal/      shared packages (agent card parsing/validation, storage, search)
```

## Development

There is no hosted CI. [prek](https://github.com/j178/prek) runs the checks as git hooks. It downloads the Go toolchain and golangci-lint, so Go does not need to be installed.

```sh
uv tool install prek    # once per machine
prek install            # once per clone
```

| Hook stage | Checks |
|---|---|
| commit-msg | Conventional Commits title, at most 72 characters ([git-hooks](https://github.com/sbOogway/git-hooks)) |
| pre-commit | golangci-lint (config verify, fmt, run), `go mod tidy -diff` |
| pre-push | the pre-commit checks, `go test -race ./...`, `govulncheck ./...` |
| post-merge | delete local branches whose upstream branch is gone |

Build, run and test with `make` (`make help` shows all targets). If Go is not installed, `make` uses the Go toolchain of prek.

The binaries read `oai.toml` (local development values). Each `OAI_*` environment variable overrides one key, for example `OAI_LISTEN_ADDR=:9090 make run-api`. See spec section 11.

```sh
make run-crawler    # one crawl of seeds.txt into data/oai.db
make run-api        # API on :8080, reads data/oai.db
make run-web        # website on http://localhost:8000 (needs run-api)
make test
```

Run the checks by hand:

```sh
prek run --all-files
prek run --all-files --hook-stage pre-push
```
