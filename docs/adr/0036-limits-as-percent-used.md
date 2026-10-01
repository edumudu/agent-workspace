# ADR 0036: Limits shown as percent used, with a reset clock

Status: accepted, 2026-09-30. Changes how [ADR 0017](0017-usage-and-limits-bar.md) displays quotas; its derivation and thresholds still hold.

## Decision

- The top bar and the new-session warning show percent **used**, as claude.ai and Codex do: `CC 5h 57% ↻23:52  7d 71% ↻Sat`. `domain.Quota` keeps `LeftPercent`, because the warning (25) and red (20) thresholds and the `[fallback] threshold` in `config.toml` are set in percent left; the TUI converts for display.
- A reset shows as a wall-clock time in the local zone, or as a weekday when it is a day or more away.
- `domain.Current(quotas, now)` drops a window whose reset time has passed, as Claude Code's status line does. A window with no reset time is kept.
- Claude Code's status line sends `five_hour`, `seven_day` and, behind a gateway, `spend_limit` (https://code.claude.com/docs/en/statusline); it has no per-model window. `debug seed` no longer invents one. A window the TUI does not know still shows under its own name.

## Why

- On the first manual run, `5h 43%` read as wrong next to claude.ai's `8% used`: the two sides counted opposite ways, and the seeded per-model figure had no real counterpart.
- A countdown (`2h9m`) has to be added to the current time to compare with claude.ai's "Resets at 2:20 PM"; a clock time does not.

## Limits

- Three windows no longer fit the 48-column bar with clock times; a third is truncated. Claude Code sends two.
