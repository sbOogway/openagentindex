# Open Agent Index

Finds, verifies and indexes [A2A](https://a2a-protocol.org) agents, and serves search over them.

Scope: A2A agents only — something is listed only if it serves a valid agent card. No MCP servers, and
the search engine is not itself an agent: its interfaces are a REST API and a static website.

## Architecture (MVP)

```
crawler image ──► SQLite ◄── api image (REST) ◄── frontend (static, Cloudflare Pages)
```

Written in Go. Both images come from this codebase and run on one VPS, sharing the SQLite file on a volume.

- **Crawler** — discovers candidate domains (a2aregistry API, certificate logs, GitHub), fetches
  `/.well-known/agent-card.json` from each agent's own domain, validates v1.0 / v0.3 cards, and
  re-checks known agents (card refresh, liveness, latency). Other registries are only used as leads;
  card data always comes from the agent itself.
- **API** — `GET /search`, `GET /agents/{domain}`; ranks by text relevance boosted by quality signals
  (uptime, valid card, responds, signed).
- **Frontend** — static search page calling the API.

Out of scope for the MVP: Nostr sync, LLM-based skill testing, owner claims, signature verification,
accounts, payments, large-scale domain scanning.

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
