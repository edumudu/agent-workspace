# agent-workspace

`agentws` is a terminal workspace for running Claude Code and Codex sessions in parallel. Each session works in git worktrees, and the tool gives it a local, PR-style review pane. It is written in Go as one binary that acts as the daemon, the TUI, the CLI and the hook handler.

- What it does: [FEATURES.md](FEATURES.md)
- How it's built, the layers, and the performance budgets: [ARCHITECTURE.md](ARCHITECTURE.md)
- Decisions: [docs/adr/](docs/adr/)
- Work items: the GitHub issues, milestones `v1` (P0) and `v1.1` (P1). Each issue lists what it depends on; build those first.

## Commands

- `make build`: produces `./bin/agentws`.
- `make test`: `go test ./...`.
- `make lint`: `golangci-lint` (including the `depguard` layer rules in `.golangci.yml`), then `scripts/lint-comments`.
- `make bench`: benchmarks that guard the performance budgets.
- `make mutate`: `scripts/mutate` runs `gremlins` on `domain` and `app`, failing below 80% efficacy. A package with no tests is skipped. Install with `go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0`.
- `make e2e`: the core e2e suite. It builds the binary once and runs the `test/e2e/testdata/script/*.txtar` scripts.
- `scripts/tdd-check <base> <head>`: what CI's `tdd` job runs. It covers added or changed `*_test.go` files and files under a `testdata/` dir (such as e2e `.txtar` scripts), each counted for the package that owns them. It runs them with `-tags integration`, so integration tests count too. Run it locally on a clean tree to check a branch before pushing; it leaves you on a detached base checkout.
- `make test`, `make bench` and `make e2e` pass `GO_TEST_FLAGS` (default `-p 2`) to keep local runs light.
- Integration tests use `-tags integration`. They need `git` and `tmux` installed, and use a temporary `AGENTWS_HOME`. The tmux ones also use a unique tmux socket each. Run them with `go test -p 2 -tags integration ./internal/adapters/...`; CI does too. The git adapter's tests build real repos in temp dirs with `GIT_CONFIG_GLOBAL=/dev/null` so the user's git config never leaks in.
- `agentws daemon [start|status|stop]` runs or controls the daemon. Tests that start one use a short `AGENTWS_HOME` under `/tmp`: macOS caps Unix socket paths at 104 bytes.
- `scripts/bench-hook.sh [bin]`: times 200 `agentws hook` runs with the daemon up and down, prints p50/p95, and fails over budget (20 ms / 60 ms p95). CI runs it with `BUDGET_SCALE=2` because its runners start processes about 3x slower. Keep `cmd/agentws` startup light: package init costs every hook, and `modernc.org/sqlite` init is already about 4.5 of the 5 ms locally.
- `agentws workspace add <path>|list|remove <path>` registers workspaces through the daemon (auto-started). `list` shows `-` for a repo's git facts until the first background refresh lands.
- `agentws` attaches to the client layout (creating it and the daemon if needed); `agentws tui` is what runs in its left pane. `agentws debug seed N` adds N fake sessions, with event logs for the session card, for trying the TUI.
- `AGENTWS_TMUX_SOCKET` points the daemon at another tmux server. Use it with a temporary `AGENTWS_HOME` when running the TUI by hand, so the real `agentws` server is untouched.
- TUI goldens live in `internal/tui/testdata/*.golden`; regenerate with `go test ./internal/tui/ -run Golden -update` and review the diff.
- `agentws debug session [--once] <id>` prints a session's state, harness, pane, model, effort, context left and limit used, then each change to it until interrupted (`--once` prints just the current line).
- `agentws setup codex [--remove]` merges (or removes) the agentws hooks in `$CODEX_HOME/hooks.json`. Tests of it, and of anything else that touches Codex config, use a temp `CODEX_HOME`; never point them at the real `~/.codex`.
- `agentws setup claude [--remove]` merges agentws hooks and the status-line wrapper into `$CLAUDE_CONFIG_DIR/settings.json` (default `~/.claude`). Tests set `CLAUDE_CONFIG_DIR` to a temp dir and never touch the real `~/.claude`.
- `agentws debug launch --harness claude --dir <dir> [--model m] [--effort e]` starts a session in a new pane.
- `agentws setup-worktree <path>` applies the `[setup]` recipe in the main checkout's `.agentws.toml` (`copy`, `link`, `run`, `deps = clone|link|install`) to a linked worktree, in-process (no daemon). Its integration tests are named `TestRecipe*`: `go test -tags integration -run Recipe ./internal/adapters/setup`. See [docs/adr/0013-setup-recipes.md](docs/adr/0013-setup-recipes.md).
- `domain`, `app`, `tui`, `rpc` and `daemon` may not import `os/exec` (depguard). `internal/adapters/tmux` is the only code that runs tmux, and only `internal/daemon` may import it.

## Rules

- **Layers:** `domain` has no IO and no imports from other `internal/*` packages. `app` depends only on `domain` and defines the ports. Adapters implement those ports. `tui` talks only to `rpc`. Lint enforces this, so fix the design rather than the lint config.
- **Rules live in `domain`:** state transitions, naming and cleanup decisions go there as pure, table-tested functions, not in adapters or the TUI.
- **Nothing slow on hot paths:** no exec, disk or network calls in `agentws hook`, in TUI rendering, or in the daemon event loop. Heavy work goes to workers. Keep the budgets in ARCHITECTURE.md; if a change risks one, add or update a benchmark.
- **Shell out, don't reimplement:** use the `git`, `gh` and `tmux` CLIs. Never use a Go git library.
- **Never destroy user work:** cleanup code backs up uncommitted changes before removing anything, never deletes branches, and never touches a worktree that a process is using. Any change to cleanup needs tests for those cases.
- **Leave the user's setup alone:** the tmux adapter uses only the `agentws` tmux server. Setup commands merge into `~/.claude` and `~/.codex` config idempotently, back the file up first, and can be undone.

## Tests: TDD, enforced by CI

- **Test first, always:** write the failing test, see it fail, then write the code. Never add a test after the behavior already exists.
- **Commit order proves it:** the test commit comes before the implementation commit. CI's `tdd` job runs the PR's new and changed tests against the base branch. They must fail there (a compile error counts as failing). A PR whose new tests pass on base fails the check. The check is per package: if any package with an added or changed `*_test.go` passes on base, the job fails, so keep test-only refactors in their own PR.
- **Protect the core concepts:** the domain state machine, naming, cleanup decisions, discovery, review scopes and the prompt format. Test through public behavior: inputs and outputs, not internals.
- **No useless tests:** no tests of getters, constructors, framework code or mocks calling mocks. CI runs mutation testing (`gremlins`) on `internal/domain` and `internal/app`; the mutation score must stay ≥ 80%. A test that kills no mutants gets deleted.
- **Fakes, not mocks:** unit tests use in-memory fakes of the ports. Integration tests use real temporary git repos and a real tmux server.
- **Port fakes live in `fakes_test.go`:** put in-memory fakes of ports in `fakes_test.go` (or `*_fake_test.go`) in their package. `scripts/tdd-check` skips those files, so adding a port method doesn't fail the `tdd` job. Behavior tests never go in them; they must still fail on base. See ADR 0009.
- **Domain tests are in-package** (`package domain`): depguard bans `domain` from importing `internal/...`, and that includes an external `domain_test` package importing `domain`.
- **Small core e2e suite:** `test/e2e` uses `testscript` with fake `claude`, `codex` and `gh` binaries. It covers: start a session → hook events → state; an agent-created worktree gets attached; a merged PR gets its worktree cleaned; review comments get sent. Keep it small and fast (< 60 s). It is required in CI.

## Comments

- Code explains itself through names. A comment only says *why*, never *what*.
- Inside function bodies, the only comments allowed are ones starting with `// why:` and tool directives (`//go:`, `//nolint:` with a reason). `scripts/lint-comments` enforces this in CI.
- Doc comments only where they add information a caller needs. No boilerplate docs that restate the name.
- A TODO must reference an issue: `// TODO(#12): ...`.
- A `//nolint:` directive needs a reason after it: `//nolint:gosec // why: ...`.
- `scripts/lint-comments` fixtures live in its `testdata/` as `*.go.txt` so no other tool compiles them.

## Working an issue

1. Read the issue, the FEATURES.md and ARCHITECTURE.md sections it links, and the issues it depends on.
2. Make a branch named `<issue-number>-<slug>`.
3. For each behavior: commit a failing test, then commit the code that makes it pass, then refactor.
4. Meet every acceptance criterion. Run the commands in the issue's **Validate** section and paste their output into the PR body.
5. Open one PR per issue that says `Closes #<n>`. Title it with a conventional prefix (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`).
6. If a criterion turns out wrong or impossible, don't quietly drop it. Say so in the PR and on the issue.
7. main is protected: merge only via PR with build, tdd and mutate green and the branch up to date with main.

## Repo hygiene

- This repo is public. Never commit employer names, internal repo or service names, internal URLs, issue keys, tokens, or real usage data. Use generic examples such as `api`, `web`, `#42`.
- Commits use the GitHub noreply email already set in this clone's git config.
