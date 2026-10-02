# test/integration

`-tags integration` tests that run the daemon with its real adapters (the daemon package itself may not exec). CI runs them with every other integration-tagged package.

- `go test -tags integration -run CodexPicker ./test/integration/` drives a fake Codex model picker (`testdata/fake-codex-picker.py`, needs `python3`) in a real tmux pane. See [ADR 0034](../../docs/adr/0034-codex-model-picker-switching.md).
- `TestNvimPluginSpecs` runs `nvim/test/spec.lua` headless with `nvim --clean` and throwaway XDG dirs: never point plugin tests at `~/.config/nvim`. See [nvim/](../../nvim/AGENTS.md).
- `CleanupExec`, `Ports`, `ReviewSend` (one pastes into a real tmux pane) and worktree detection also have end-to-end cases here; their feature notes are in [internal/daemon/](../../internal/daemon/AGENTS.md).
