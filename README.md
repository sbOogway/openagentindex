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

Go isn't required on the host; the toolchain runs from the official image:

```sh
podman run --rm -v "$PWD":/src:Z -w /src docker.io/library/golang:1.27 go build ./...
```
