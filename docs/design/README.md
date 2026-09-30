# Design sources

The v1 TUI mockups: Catppuccin Latte, JetBrains Mono, 1440×900 boards. The PNGs in [`../images`](../images) are exports of these.

- `Main.dc.html`: sessions sidebar, agent pane, shell.
- `Review.dc.html`: review pane with scopes, worktree tabs, diff and draft comments.
- `NewSession.dc.html`: new-session dialog with the low-quota warning.
- `Worktrees.dc.html`: worktrees and disk view.
- `canvas.json`: board layout and titles.

They are the source files of a Claude Design canvas. Each board's markup is plain HTML with inline styles, and its sample data is in the `renderVals()` script at the bottom. Names in them are placeholders (`ACME-142`, `api`, `web`, `infra`).
