# agentws (working name)

A terminal workspace for running Claude Code and Codex sessions in parallel. It creates and cleans up git worktrees for you, and lets you review what agents changed in a local, PR-style pane.

**Status:** alpha. Builds are published as pre-releases on [GitHub Releases](https://github.com/giovaniif/agent-workspace/releases); expect rough edges and breaking changes between alphas. The screens below are design mockups; their sources are in [docs/design](docs/design).

## Install

Builds exist for macOS (arm64, amd64) and Linux (amd64); no Go toolchain is needed. You need `tmux`, `git` and the GitHub CLI (`gh`, signed in), plus Claude Code and/or the Codex CLI.

Install the newest release (alphas included):

```sh
curl -fsSL https://raw.githubusercontent.com/giovaniif/agent-workspace/main/scripts/install.sh | sh
```

The script downloads the archive for your OS and CPU from GitHub Releases, checks it against `checksums.txt`, puts the `agentws` binary in `~/.local/bin`, and puts the nvim plugin in `~/.local/share/agentws/nvim` (`$XDG_DATA_HOME/agentws/nvim` when that is set). Two environment variables change that:

```sh
# a specific release
curl -fsSL https://raw.githubusercontent.com/giovaniif/agent-workspace/main/scripts/install.sh | AGENTWS_VERSION=v0.1.0-alpha.1 sh
# another directory
curl -fsSL https://raw.githubusercontent.com/giovaniif/agent-workspace/main/scripts/install.sh | AGENTWS_INSTALL_DIR=/usr/local/bin sh
```

If the install directory is not on your `PATH`, the script says so. Add it in your shell profile (`~/.zshrc` or `~/.bashrc`):

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Check the install:

```sh
agentws version   # agentws v0.1.0-alpha.1 (commit abc1234)
```

## Upgrade

`agentws version` prints the installed version and, at most once a day, checks GitHub for a newer release (set `AGENTWS_NO_UPDATE_CHECK=1` to turn that off). It never updates itself. To upgrade, run the install script again, then stop the old daemon so the new binary starts its own:

```sh
curl -fsSL https://raw.githubusercontent.com/giovaniif/agent-workspace/main/scripts/install.sh | sh
agentws daemon stop
agentws version
agentws
```

## What it does

- **Sessions side by side.** Claude Code and Codex run next to each other, grouped by task and named after the issue or PR they work on. Each shows its state (running, waiting, done), model, effort, and how much context is left.
- **Notifications.** You get one when an agent needs permission, is waiting on you, or finishes, whichever harness it is.
- **Limits.** The Claude 5h/7d windows and the Codex limits sit in one bar, with a warning before you start a session on a nearly exhausted quota.
- **Worktrees handled for you.** The agent creates worktrees, and each one is attached to the session that made it. When its PR merges, it is removed automatically if it has no uncommitted changes. If it does, they are backed up and you're asked first. Branches are never deleted.
- **Ready-to-work worktrees.** A `[setup]` table in the repo's `.agentws.toml` copies env templates, links caches, runs commands and shares `node_modules` through APFS clones. `agentws setup-worktree <path>` applies it.
- **Local review.** Diff the last agent turn, the uncommitted changes, or the whole branch. Comment on lines and send all the comments to the agent as one prompt.
- **Shell and nvim one key away.** Both open in the right worktree. Diffs open in nvim through diffview.
- **Single repo or multi-repo.** Point it at a single repo or at a folder that holds several service repos.

## Screens

| Review | Worktrees and disk |
|---|---|
| ![Review pane](docs/images/review.png) | ![Worktrees](docs/images/worktrees.png) |

![New session](docs/images/new-session.png)

## How it works

- One Go binary acts as the daemon, the TUI, the CLI, and the hook handler.
- Agents run in panes on a separate tmux server (`tmux -L agentws`). Your own tmux setup is left alone, and sessions survive closing the terminal. Press `ctrl+\` in an agent pane to return to the sidebar.
- Claude Code and Codex hooks report state to the daemon over a local socket. The hook handler exits in under 20 ms, so agents never wait on it.
- git, gh and tmux are called as command-line tools; nothing reimplements them.

The details are in [ARCHITECTURE.md](ARCHITECTURE.md), and the full scope is in [FEATURES.md](FEATURES.md).

## Getting started

Run `agentws`. The first time, a setup screen opens over the sidebar:

1. **Agents.** Pick Claude Code, Codex or both (`↑/↓`, `space`, `⏎`). Each one shows whether its hooks are already set up.
2. **Claude Code.** The screen shows the file it changes (`~/.claude/settings.json`, or `$CLAUDE_CONFIG_DIR`), what it adds (the agentws hooks and a status-line wrapper that still runs yours), where the backup goes, and how to undo it (`agentws setup claude --remove`). `⏎` installs, `s` skips.
3. **Codex.** The same for `~/.codex/hooks.json` (or `$CODEX_HOME`). Codex then needs a one-time trust step: start `codex` and accept the review prompt for the new hooks, or trust them in `/hooks`.
4. **Neovim (optional).** If `nvim` is on `PATH` and the plugin isn't configured anywhere in your nvim config yet, the screen shows the file it will write, `~/.config/nvim/plugin/agentws.lua`, which loads the plugin the install script put in `${XDG_DATA_HOME:-~/.local/share}/agentws/nvim`. `⏎` writes it, `s` skips. Your own files (init.lua, init.vim, lazy.nvim specs) are not edited; `agentws setup nvim --remove` deletes the file. Without nvim, the step says how to install it (`brew install neovim`) and `⏎` goes on; everything but `e` and `o` works without it.
5. **Done.** Press `n` to start your first session.

`esc` skips the rest at any step. Either way the screen doesn't come back on its own, and it never opens when a harness is already set up and nvim has nothing left to configure. Open it again with `S` in the sidebar or with `agentws setup`. `agentws setup claude`, `agentws setup codex` and `agentws setup nvim` do the same installs without the screen.

## Requirements

macOS or Linux, tmux, git, the GitHub CLI (`gh`, signed in), and Claude Code and/or the Codex CLI. For the nvim integration: Neovim 0.10+ and, for `:AgentwsDiff`, [diffview.nvim](https://github.com/sindrets/diffview.nvim).

## Build and test

Needs Go (version in `go.mod`), `golangci-lint` v2, and for `make mutate` `gremlins`.

```sh
make build             # ./bin/agentws
./bin/agentws version
./bin/agentws debug seed 3 && ./bin/agentws   # the sidebar with 3 fake sessions
./bin/agentws workspace add ~/src/api && ./bin/agentws new "fix the flaky test"   # or press n in the TUI
make test              # go test ./...
make lint              # golangci-lint + scripts/lint-comments
make e2e               # testscript suite in test/e2e
make bench             # benchmarks for the performance budgets
make mutate            # gremlins on internal/domain and internal/app
```

Integration tests use `-tags integration` and need `git` and `tmux`.

## Shell and nvim

In the sidebar, `t` shows a shell below the selected session's agent pane in its worktree (and hides it again), `T` opens it as a popup (`M-t` closes it), and `e` swaps the session's nvim into the main area and back. `o` in the review opens the file at the diff's top line in that nvim. In nvim, `C-h/j/k/l` move between its splits and, at the edge, to the neighbouring pane; every other pane receives those keys untouched.

The setup screen shows the exact lines for your install. By hand: put the plugin dir (`${XDG_DATA_HOME:-~/.local/share}/agentws/nvim` from a release, or the repo's `nvim/` in a checkout) on the runtimepath (with your plugin manager, or `vim.opt.rtp:prepend('<dir>')`) and call `require('agentws').setup({})`. It needs `agentws` on `PATH`, and works in the nvim the `e` key starts, which carries `AGENTWS_SESSION`.

- `:AgentwsDiff [last_turn|uncommitted|branch]` opens the session's review scope in diffview (by default the one the review pane has open).
- `:'<,'>AgentwsComment [text]` adds the selected lines as a draft review comment.
- `setup{bin = 'agentws', tmux = 'tmux', navigate = true}`; `navigate = false` leaves `C-h/j/k/l` alone in nvim.

## Codex setup

`agentws setup codex` adds the hooks Codex needs to report state, and prints the one-time trust step Codex requires. `agentws setup codex --remove` takes them out. It backs up an existing `hooks.json` first and leaves your other hooks alone.

## Contributing

The build is test-driven and CI enforces it. Read [AGENTS.md](AGENTS.md) before opening a PR: it covers the layer rules, tests first, no low-value tests or comments, and one PR per issue.
