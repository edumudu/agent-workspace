# agent-workspace

`agentws` is a terminal workspace for running Claude Code and Codex sessions in parallel. Each session works in git worktrees, and the tool gives it a local, PR-style review pane. It is written in Go as one binary that acts as the daemon, the TUI, the CLI and the hook handler.

- What it does: [FEATURES.md](FEATURES.md)
- How it's built, the layers, and the performance budgets: [ARCHITECTURE.md](ARCHITECTURE.md)
- Decisions: [docs/adr/](docs/adr/)
- Work items: the GitHub issues, milestones `v1` (P0) and `v1.1` (P1). Each issue lists what it depends on; build those first.

## Commands

- `make build`: produces `./bin/agentws`.
- `make test`: `go test ./...`.
- `make lint`: `golangci-lint`, including the layer rules.
- `make bench`: benchmarks that guard the performance budgets.
- Integration tests use `-tags integration`. They need `git` and `tmux` installed, and use a temporary `AGENTWS_HOME`.

## Rules

- **Layers:** `domain` has no IO and no imports from other `internal/*` packages. `app` depends only on `domain` and defines the ports. Adapters implement those ports. `tui` talks only to `rpc`. Lint enforces this, so fix the design rather than the lint config.
- **Rules live in `domain`:** state transitions, naming and cleanup decisions go there as pure, table-tested functions, not in adapters or the TUI.
- **Nothing slow on hot paths:** no exec, disk or network calls in `agentws hook`, in TUI rendering, or in the daemon event loop. Heavy work goes to workers. Keep the budgets in ARCHITECTURE.md; if a change risks one, add or update a benchmark.
- **Shell out, don't reimplement:** use the `git`, `gh` and `tmux` CLIs. Never use a Go git library.
- **Never destroy user work:** cleanup code backs up uncommitted changes before removing anything, never deletes branches, and never touches a worktree that a process is using. Any change to cleanup needs tests for those cases.
- **Leave the user's setup alone:** the tmux adapter uses only the `agentws` tmux server. Setup commands merge into `~/.claude` and `~/.codex` config idempotently, back the file up first, and can be undone.
## Tests: TDD, enforced by CI

- **Test first, always:** write the failing test, see it fail, then write the code. Never add a test after the behavior already exists.
- **Commit order proves it:** the test commit comes before the implementation commit. CI's `tdd` job runs the PR's new and changed tests against the base branch. They must fail there (a compile error counts as failing). A PR whose new tests pass on base fails the check.
- **Protect the core concepts:** the domain state machine, naming, cleanup decisions, discovery, review scopes and the prompt format. Test through public behavior: inputs and outputs, not internals.
- **No useless tests:** no tests of getters, constructors, framework code or mocks calling mocks. CI runs mutation testing (`gremlins`) on `internal/domain` and `internal/app`; the mutation score must stay ≥ 80%. A test that kills no mutants gets deleted.
- **Fakes, not mocks:** unit tests use in-memory fakes of the ports. Integration tests use real temporary git repos and a real tmux server.
- **Small core e2e suite:** `test/e2e` uses `testscript` with fake `claude`, `codex` and `gh` binaries. It covers: start a session → hook events → state; an agent-created worktree gets attached; a merged PR gets its worktree cleaned; review comments get sent. Keep it small and fast (< 60 s). It is required in CI.

## Comments

- Code explains itself through names. A comment only says *why*, never *what*.
- Inside function bodies, the only comments allowed are ones starting with `// why:` and tool directives (`//go:`, `//nolint:` with a reason). `scripts/lint-comments` enforces this in CI.
- Doc comments only where they add information a caller needs. No boilerplate docs that restate the name.
- A TODO must reference an issue: `// TODO(#12): ...`.

## Working an issue

1. Read the issue, the FEATURES.md and ARCHITECTURE.md sections it links, and the issues it depends on.
2. Make a branch named `<issue-number>-<slug>`.
3. For each behavior: commit a failing test, then commit the code that makes it pass, then refactor.
4. Meet every acceptance criterion. Run the commands in the issue's **Validate** section and paste their output into the PR body.
5. Open one PR per issue that says `Closes #<n>`. Title it with a conventional prefix (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`).
6. If a criterion turns out wrong or impossible, don't quietly drop it. Say so in the PR and on the issue.

## Repo hygiene

- This repo is public. Never commit employer names, internal repo or service names, internal URLs, issue keys, tokens, or real usage data. Use generic examples such as `api`, `web`, `#42`.
- Commits use the GitHub noreply email already set in this clone's git config.
