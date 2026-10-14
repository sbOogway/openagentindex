# AGENTS.md

## Project

- Open Agent Index finds, verifies and indexes A2A agents and serves search over them. `spec.md` defines the design. Change the spec before you change the design.
- Index A2A agents only. Do not index MCP servers.
- The search engine is not an agent. Its interfaces are the REST API and the static frontend. Do not add an A2A or MCP endpoint.
- Get agent card data from the agent's own domain. Use other registries only as a source of leads.

## Communication

- Use ASD-STE100 Simplified Technical English for all user-facing text.
- Be brief in all user-facing text, pull requests, reports and documentation.

## Comments

- Do not add unnecessary comments to the source code.
- Write a comment only for what the code cannot say itself: a non-obvious reason, a workaround, or an external constraint.
- Do not comment what the code already makes clear.
- Do not narrate changes in comments ("added X", "now uses Y").
- Do not leave commented-out code.
- Write doc comments for exported identifiers only when the name and signature do not say enough.

## Functions

- Keep functions short. When a function does more than one thing, split it.
- Split a function whenever a part of it can be extracted and named for what it does.
- Keep each function at a single level of abstraction.
- Give a function at most 3 arguments. A leading `context.Context` does not count.
- When a function needs more than 3 arguments, group the ones that belong together into a struct and pass that instead.
- Do not use boolean flag arguments to switch behaviour; write separate functions.

## Packages

- Make packages deep: a small exported API hiding as much functionality as possible.
- Do not write shallow packages or pass-through functions that only forward calls.
- Put shared code under `internal/`. Keep `cmd/` binaries thin: they parse configuration and wire packages together.
- Packages talk to each other only through their exported API.
- Keep coupling loose: depend on small interfaces, not on concrete types. Define an interface in the package that uses it.
- Split functions inside a package freely, but do not grow its exported API to do so.

## Testing

- Before you create a branch, write a test plan: the behaviours to test and how to test each one.
- Write table-driven tests where cases differ only in their data.
- Unit tests must not use the network. Use `net/http/httptest` for HTTP.
- prek runs the checks in `.pre-commit-config.yaml`. At each commit: golangci-lint and `go mod tidy -diff`. At each push: also `go test -race` and `govulncheck`.
- Install the hooks once in each clone: `prek install`. Do not commit or push with `--no-verify`.
- Go is not installed on the host. prek downloads the Go toolchain and golangci-lint.
- Do not merge a pull request with failed checks. Do not disable a linter to make a check pass; fix the code or explain the exception in the pull request.

## Workflow

- Start every feature or issue on a new branch from an up-to-date `main`. Never commit to `main` directly.
- Name the branch after the change type and topic, e.g. `feat/card-validation`, `fix/crawler-timeout`.
- Before creating the branch, settle the design questions and prepare the test plan. Treat the test plan as approved without asking the user.
- Keep each pull request small, so that the user can review it in a few minutes. Try to stay below 200 changed lines, tests not included.
- Write as little code as possible for the change. Do not add code for future work.
- When a task needs more code, split it into a sequence of pull requests. Each pull request builds and passes the tests on its own.
- Open a draft pull request on GitHub as soon as the branch has its first commit, linking the issue it addresses and including the approved test plan.
- Do the work on that branch, then mark the pull request ready for review.
- Stop and wait for the user's code review. Do not merge the pull request yourself.
- Address every review comment with new commits on the same branch, then ask for review again.
- Repeat until the user approves. Only the user decides when the work is done.

## Commits

- Write commit messages in the Conventional Commits format: `type(scope): summary`. Keep the title at most 72 characters. The commit-msg hook checks this.
- Use `feat` for a new feature, `fix` for a bug fix, and `refactor`, `perf`, `test`, `docs`, `build`, `ci`, `chore`, `style` or `revert` for everything else.
- Use the package or component as the scope, e.g. `feat(crawler): ...`, `fix(api): ...`.
- Mark breaking changes with `!` after the type or a `BREAKING CHANGE:` footer.
- Make every commit atomic: one logical change per commit, which builds and passes the tests on its own.
- Do not mix unrelated changes, such as a refactor and a feature, in one commit. Split them.
- Do not rewrite the commits of a branch under review; answer review comments with new commits.
- You can rebase a branch onto `main`.
