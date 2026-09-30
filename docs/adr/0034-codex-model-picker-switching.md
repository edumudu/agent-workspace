# ADR 0034: Codex model and effort switching through its picker

Status: accepted, 2026-09-30. Lifts the "Codex is not supported" limit of [ADR 0018](0018-model-effort-switching.md); everything else there still holds.

## Decision

- `M` and `E` work for Codex sessions. `domain.SwitchChoices` lists fixed Codex models (`gpt-6.1-sol`, `gpt-6-astra`, `gpt-6-sol`, `gpt-6-luna`, `gpt-5.5`) and the same efforts as Claude. Queueing, `Dispatch`, the recheck and the 150 ms paste settle are the ones from ADR 0018.
- Codex 0.159 has no inline `/model <x>` and no `/effort`. Its `/model` opens a model popup and then a "Select Reasoning Level for <model>" popup. So `domain.SwitchCommand` is `/model` for Codex, and after Enter `app.SendSwitches` walks the picker: capture the pane, `domain.ParsePicker`, `domain.CodexPickerKeys`, send the keys, repeat until no popup is left (at most 4 steps). After each press it polls (up to 20 captures, 150 ms apart) until the parsed picker differs from the one it acted on, because tmux returns before Codex redraws. Any failure, including a failed capture, sends `Escape` with its own short context so no picker is left open for later input.
- `ParsePicker` takes the rows after the last line starting with `Select `. A row is `N. label`, with `›` on the highlighted one; the label ends at the first run of two spaces. A `›` line that is not a row (Codex's input prompt) under the title means that picker has closed and is only scrollback.
- `CodexPickerKeys` presses `Up`/`Down` from the highlighted row to the target, then `Enter`. Labels match ignoring case and a trailing `(…)` tag, and `Extra high` is `xhigh`. In the model popup the target is the new model, or the current model for an effort switch; when it is not listed it opens an `All models` row. In the reasoning popup the target is the new effort, or the current effort for a model switch, falling back to the highlighted row. A target that is not listed closes the picker with `Escape` and fails the switch, which raises the `!` warning.
- Codex writes the new model and effort to the rollout only when the next turn starts. `Snapshot.TurnAt` is the time of the latest `turn_context`, and the daemon feeds rollout reads through `Session.Report` with `At = TurnAt`. A report stamped before a switch was sent can still confirm it but never warns; one from a later turn that shows the old value warns, as for Claude.

## Why

- Reading the popup off the screen, rather than counting key presses from a remembered list, survives Codex reordering or adding models: the highlight and rows are whatever Codex shows now.
- Keeping the parse and the key choice in `domain` makes the Codex-specific part table-tested; the app loop only captures and sends.

## Limits

- Real Codex was not driven: it needs credentials. The picker format is inferred from strings in the 0.159 binary and its model cache, and checked against a fake picker in a real tmux pane (`TestModelSwitchCodexPickerIsWalkedInARealPane`). The assumptions to verify with real Codex are listed in the PR.
- A popup whose title does not start with `Select ` (for example Plan mode's "Choose where to apply") is treated as closed, so the switch is left to the rollout to confirm or warn.
- The model list is fixed, like Claude's.
