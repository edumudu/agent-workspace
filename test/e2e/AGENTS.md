# test/e2e

The core e2e suite (`make e2e`, about 5 s). It is required in CI. See [ADR 0032](../../docs/adr/0032-core-e2e-suite.md).

- **Scope.** `testscript` with fake `claude`, `codex` and `gh` binaries. It covers: start a session → hook events → state; an agent-created worktree gets attached; a merged PR gets its worktree cleaned; review comments get sent. Keep it small and fast (< 60 s).
- **How it runs.** It builds the binary once and runs the `testdata/script/*.txtar` scripts, each with its own temp `AGENTWS_HOME`, temp repos and `tmux -L` server, so it needs `tmux`.
- **Fakes.** `testdata/bin` is on `PATH`: `claude` and `codex` replay `$AGENTWS_E2E/<harness>.steps` (`hook`, `run`, `gate`, `read`; see the script header), and `gh` answers `gh api graphql` from `$AGENTWS_E2E/prs.json`. Stub `osascript` and `terminal-notifier` log their argv. `make dev FAKES=1` puts the fake harnesses on `PATH` for manual runs.
- **No sleeps.** `eventually <regexp> <cmd>` polls with a 15 s deadline, `capture VAR <regexp>` reads the last stdout, `repo <name>` makes a clone whose origin names github.com, and a fake waiting on `gate x` goes on after `mkdir $AGENTWS_E2E/gates/x`.
- **Daemon test hooks.** `AGENTWS_TEST_CLOCK` (a duration such as `+5h`) moves the cleanup clock past the 4 h grace, and `AGENTWS_TEST_PR_POLL` shortens the PR poll.
