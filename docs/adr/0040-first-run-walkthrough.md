# ADR 0040: First-run walkthrough

Status: accepted, 2026-10-01.

## Decision

- **When.** The sidebar asks the daemon for `onboarding.status` once, on its first snapshot. Until `$AGENTWS_HOME/onboarded` exists, it opens the walkthrough as a centred popup (`agentws tui --setup`, run through `client.popup` like the new-session dialog in ADR 0037). On the first run the sidebar can start before tmux has attached a client, so a failed popup is retried twice, 400 ms apart, and then the walkthrough opens inline in the sidebar. `S` in the sidebar and `agentws setup` with no arguments open it again at any time. Finishing it, or pressing `esc` at any step, writes the marker.
- **Steps.** Agents (pick Claude Code, Codex or both; the ones already set up are picked) → Claude Code → Codex → Neovim → Done. `domain.OnboardSteps`, `NextOnboardStep`, `DefaultOnboardPicks`, `HarnessOffer` and `NvimOfferFor` decide the steps and what each one offers. They are pure and table-tested.
- **Harness steps.** Each one shows the state, the file it changes, what changes, the backup and the undo command (`agentws setup <harness> --remove`). `⏎` installs and `s` skips. The Codex step also shows the one-time trust step (`domain.CodexTrustStep`, which `agentws setup codex` prints too). A config the adapter cannot parse is reported and gets no install offer.
- **Same check as `agentws setup`.** `claude.Installed` and `codex.Installed` run Setup's merge in memory and report whether it would change nothing, so the walkthrough and the CLI cannot disagree. Install calls the same `claude.Setup` and `codex.Setup`, so it stays idempotent, backs up first and can be undone.
- **nvim.** `adapters/onboard` looks `nvim` up on `PATH` and scans `$XDG_CONFIG_HOME/nvim` (`*.lua`, `*.vim`, at most 400 files) for `require('agentws')` or an `agentws/nvim` path. The plugin dir is the first of `$XDG_DATA_HOME/agentws/nvim` (default `~/.local/share/agentws/nvim`, where the install script puts it) and a source checkout's `nvim/` next to `bin/`. When the plugin is not configured, the step shows `domain.NvimSnippet` for `init.lua` (or a `lua << EOF` block for `init.vim`). It never edits the nvim config. The snippet is drawn without a frame and never truncated, so selecting it copies exactly the code.
- **Layers.** Detection and installs run in `adapters/onboard`, behind the `app.Onboarder` port. The daemon serves `onboarding.status`, `onboarding.install` and `onboarding.finish` on the asking connection, never on the loop. The TUI talks only to rpc and renders from the reply.
- **Releases.** Archives ship `nvim/`, and `scripts/install.sh` copies it to `${XDG_DATA_HOME:-~/.local/share}/agentws/nvim`.

## Why

- A release user has no source checkout, so the snippet has to point at a path the install creates.
- The marker is a file in `AGENTWS_HOME`, so a temp home (tests, `make dev`) always starts fresh. The walkthrough never needs the state database.
- Showing the snippet instead of editing the config leaves the user's nvim setup and plugin manager alone.

## Limits

- Existing users see the walkthrough once after upgrading, since they have no marker.
- Hooks are written for the daemon's own binary path. A binary that moves later shows as "not set up" until setup runs again.
- The nvim check is a text search, so a config that loads the plugin some other way shows the snippet anyway. It can be skipped.
