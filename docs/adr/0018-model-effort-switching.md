# ADR 0018: Model and effort switching

Status: accepted, 2026-09-30.

## Decision

- `M` and `E` in the TUI open a picker of models or effort levels for the selected session's harness (`domain.SwitchChoices`). `j`/`k`, a digit, `enter` and `esc` drive it. The choice goes to the daemon as `session.switch` (`{"session_id","kind","value"}` → `Session`).
- The rules are pure functions on `domain.Session`. `RequestSwitch` queues a switch, replacing an unsent one of the same kind. `Dispatch(now)` marks the queued ones as sent and returns them, but only when the state is `idle`, `done` or `waiting`. In `running` and `permission` they stay queued: typing then would land in a tool run or answer a permission prompt.
- The daemon calls `Dispatch` after `session.switch` and after every hook, so a switch queued while `running` goes out on the next `Stop` or waiting notification. The typing runs on a worker: `app.SendSwitches` pastes the command with bracketed paste, waits 150 ms, then presses Enter. A failure drops the switch and raises the warning.
- The command is `/model <value>` or `/effort <value>` (`domain.SwitchCommand`), for both harnesses.
- Confirmation is event based, not timed. `Session.Report` checks each sent switch against the next report that says something about its kind. A match clears it. A report that still shows the old value keeps the switch and sets `SwitchWarning`, and a later matching report clears both. A model matches when the reported name contains the requested one, since Claude reports `Opus 4.7` for `opus`; an effort matches exactly, ignoring case.
- The sidebar shows `→ value` on the right of the detail row for each unconfirmed switch, and `!` when `SwitchWarning` is set. The new value itself appears as soon as the report lands, so it follows the harness's confirmation by one diff.
- Per-harness defaults are `[defaults.claude]` and `[defaults.codex]` tables (`model`, `effort`) in `$AGENTWS_HOME/config.toml`, read by `tui.LoadDefaults`.

## Why

- Sending the harness's own slash command keeps agentws out of the harness's config and works on a live session. The cost is that success is only visible through the harness's own report.
- Warning on the next report, rather than on a timer, matches when each harness can tell us. Claude refreshes its status line right after the command. Codex writes the new model and effort to the rollout on its next turn, so a timer would raise false warnings.
- Queued switches live on the session so they are stored and shown like any other session state, and survive a TUI restart.

## Limits

- The new-session dialog (#11) does not exist yet, so nothing reads the defaults; `tui.LoadDefaults` is ready for it.
- The pick lists are fixed in `domain.SwitchChoices`, not read from the harness.
- Codex `/model` opens an interactive picker in the versions checked (0.159) and Codex has no `/effort` command that we could verify without a signed-in account. The Codex commands are the ones the issue names; if Codex ignores them, the warning glyph is how it shows.
- A draft typed in the harness's input box is not cleared, so the command is appended to it.
- Checked by hand in isolated tmux servers against a fake `claude` that reports through the real `agentws hook` and `agentws statusline`. Real Claude and Codex were not driven: both need credentials, and the rule for this repo is temporary config dirs only.
