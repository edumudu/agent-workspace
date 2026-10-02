# internal/adapters/onboard

The first-run walkthrough's detection and installs, behind `app.Onboarder`. The full design is [ADR 0040](../../../docs/adr/0040-first-run-walkthrough.md).

- **Tests.** `go test ./... -run 'Onboard|Setup|FirstRun|Installed|Nvim'` runs the domain, adapter (temp `CLAUDE_CONFIG_DIR`, `CODEX_HOME` and XDG dirs), daemon and TUI tests; the screen's goldens are `internal/tui/testdata/TestGoldenSetup/*.golden`.
- **When it shows.** `agentws setup` with no arguments (or `S` in the sidebar) opens it; it shows on its own until `$AGENTWS_HOME/onboarded` exists, and only when `domain.OnboardingNeeded` finds something left.
- **nvim.** `agentws setup nvim [--remove]` writes or deletes `$XDG_CONFIG_HOME/nvim/plugin/agentws.lua`; its tests use a temp `XDG_CONFIG_HOME`, never `~/.config/nvim`.
- **By hand.** Use a temp `AGENTWS_HOME`, `AGENTWS_TMUX_SOCKET`, `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `XDG_CONFIG_HOME` and `XDG_DATA_HOME`. To try a release build, see `install.sh` in [scripts/](../../../scripts/AGENTS.md).
