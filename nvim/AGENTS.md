# nvim

The Lua plugin. It talks to the daemon only through `agentws` CLI calls; the daemon drives a long-lived nvim per session over `nvim --listen`. See [ADR 0029](../docs/adr/0029-shell-and-nvim.md) and the shell and nvim notes in [internal/daemon/](../internal/daemon/AGENTS.md).

- **Layout.** `lua/agentws` and `plugin/agentws.lua`: `:AgentwsComment`, `:AgentwsDiff [scope]`, `setup{bin, tmux, navigate}`. It calls `agentws review comment` and `agentws review scope` (JSON of each worktree's path, base commit and files) and opens `:DiffviewOpen`.
- **Navigation.** tmux does not bind `C-h/j/k/l`: they reach every pane, and the plugin moves to the neighbouring tmux pane at nvim's edge.
- **Tests.** `TestNvimPluginSpecs` (`test/integration`) runs `test/spec.lua` with `nvim --clean` and throwaway XDG dirs: never point plugin tests at `~/.config/nvim`. The nvim-only tests skip without `nvim`; CI installs it.
- **By hand.** Use a temporary `AGENTWS_HOME`, `AGENTWS_TMUX_SOCKET` and `XDG_CONFIG_HOME` whose `nvim/init.lua` prepends `nvim/` to the runtimepath and calls `require('agentws').setup({})`.
- **Releases** ship `nvim/lua` and `nvim/plugin`; `scripts/install.sh` copies them under `$XDG_DATA_HOME/agentws/nvim`, and `agentws setup nvim` points the user's config at them (see [internal/adapters/onboard/](../internal/adapters/onboard/AGENTS.md)).
- Lua comments follow the comment rule with `-- why:`.
