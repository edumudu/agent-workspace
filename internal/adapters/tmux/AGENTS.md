# internal/adapters/tmux

The only code that runs tmux; only `internal/daemon` may import it. See [ADR 0003](../../../docs/adr/0003-tmux-terminal-host.md).

- It drives a dedicated server (`tmux -L agentws`, or `AGENTWS_TMUX_SOCKET`) with its own config, never the user's server. Panes are parked in their own windows and `swap-pane` puts one in the client's main slot.
- Keys: `ctrl+\` (`FocusSidebarKey`) moves focus from an agent pane to the sidebar, bound on the `agentws` server only ([ADR 0025](../../../docs/adr/0025-focus-return-key.md)). The config binds `M-t` (closes the shell popup); `C-h/j/k/l` are not bound so they reach every pane (the nvim plugin navigates at nvim's edge).
- Pane title strips are set through `SetTitle` ([ADR 0038](../../../docs/adr/0038-pane-title-strips.md)).
- Tests are `-tags integration` against real tmux, each with a unique socket. `go test ./... -run Shell -tags integration` covers the split, popup and key pass-through.
