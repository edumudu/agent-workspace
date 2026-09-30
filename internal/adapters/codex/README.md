# Codex adapter

## Hooks

`agentws setup codex` merges one group per event into `$CODEX_HOME/hooks.json` (default `~/.codex`). Each runs `agentws hook --harness codex --event <Name>`.

| Codex hook | Event | State |
|---|---|---|
| `SessionStart`, `SessionEnd` | `session_start`, `session_end` | `idle` |
| `UserPromptSubmit` | `user_prompt_submit` | `running` |
| `PreToolUse`, `PostToolUse` | `pre_tool_use`, `post_tool_use` | `running`, only while a turn is on |
| `PermissionRequest` | `permission_request` | `permission` |
| `Stop`, `Interrupt` | `stop` | `done` |

Not mapped: `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`. A subagent stopping must not mark its parent done. Codex has no hook for "waiting for input", so `waiting` never comes from Codex today.

Codex also has a legacy `notify` program that gets a JSON argument. `ParseNotify` maps `agent-turn-complete` to `stop`. Nothing installs it: hooks cover the same ground.

The hook stdin JSON carries `session_id`, `transcript_path`, `cwd` and `model`. `transcript_path` is the rollout file below.

## Trust

Codex runs a hook only after the user trusts it. It records a hash per hook in `config.toml` under `[hooks.state]`. Setup does not write those hashes; it prints the step: start codex and accept the review prompt, or use `/hooks`.

## Setup file handling

- Other hooks and unknown keys stay, in their order.
- A second run changes nothing and takes no backup.
- A run that changes an existing file first saves it as `hooks.json.agentws-<UTC time>.bak`.
- `--remove` takes out only our hooks, and deletes `hooks.json` if nothing else was in it.
- A `hooks.json` that is not valid JSON is left alone and the command fails.
- Our hooks are recognised by the `hook --harness codex --event` in their command, so a moved binary is rewritten, not duplicated.

## Model, effort, context and limits

None of these are in the hook payload except the model. They come from the rollout file (`transcript_path`), a JSON line per event:

- `turn_context`: `model` and `effort`.
- `event_msg` with `payload.type == "token_count"`:
  - `info.last_token_usage.total_tokens` and `info.model_context_window` give context.
  - `rate_limits.primary` and `rate_limits.secondary` give `used_percent`, `window_minutes` and `resets_at`. Which window is 5 h and which weekly depends on the plan (`window_minutes` says).

Context left is `100 * (window - 12000 - max(used - 12000, 0)) / (window - 12000)`, rounded. The 12000 is the baseline system prompt Codex leaves out of the number it shows. This follows Codex's own rule as I know it, and has not been compared with `/status` against a live session yet.

`Usage.LimitUsedPercent` is the highest window. The daemon reads the last 512 KiB of the rollout in a worker after a hook (forced on `Stop`, `SessionStart` and `UserPromptSubmit`, at most every 2 s otherwise), never on the event loop.

Other places rate limits show up, not used: the TUI `/status` and status line, and the app-server `account/rateLimits` API. The rollout needs no running process and no credentials.

## Panes

`ResolvePane` takes `$TMUX_PANE`. Without it, it walks up the process tree from a pid until it meets a pane's process. `ProcessParent` reads a parent with `ps`. The walk is built and tested but nothing calls it yet: the daemon needs the hook's pid and each pane's pid.

## Launch

`Launch` creates a pane running `codex --model <m> -c model_reasoning_effort="<e>" [-- <prompt>]` in the given directory, through the `TerminalHost` port.
