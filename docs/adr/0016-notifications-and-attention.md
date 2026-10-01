# ADR 0016: Notifications and attention

Status: accepted, 2026-09-30.

## Decision

- `Session.Apply` already returns `EffectNotify`. The daemon now performs it. On the loop, `domain.BannerFor` turns the effect into a `Banner` (title is the session name from `NameFor`, falling back to the harness; body is `needs permission`, `waiting` or `done`). Muted sessions give no banner. `domain.Coalescer` lets one banner per session through each 10 s.
- The banner goes to a bounded queue read by one worker. The worker calls the `app.Notifier` port. If the session is focused, it first asks the `app.Foreground` port whether a terminal app is in front, and drops the banner if so. Both ports run `osascript`, so neither runs on the loop. A full queue drops the banner.
- `adapters/notify.Osascript` implements both ports. Title, body and sound reach AppleScript as argv (`on run argv`), never spliced into the script. The frontmost check reads the process name from System Events and matches it, lowercased, against a list of terminal apps. A failed check counts as not in front, so a banner is never lost on a guess.
- Sound per event is optional: `$AGENTWS_HOME/notify.json`, `{"sounds":{"permission":"Glass","done":"Hero"}}`, read at daemon start. A macOS sound name, per `permission`, `waiting` or `done`.
- `m` in the TUI toggles `Session.Muted` through `session.mute`. Mute is stored with the session and silences banners only. The unread marker still appears, since `Apply` sets it independently.
- `Session.Focused` is now set. `enter` in the TUI calls `session.focus`, which focuses that session, blurs the one that had focus, and clears its unread marker. Focus is cleared for every session when the daemon starts.
- Claude and Codex go through the same path: their hooks become harness events, `Apply` yields the effects, and one code path turns them into banners. A test runs the same fixture for both.

## Why

- The state machine already decided when to notify. Keeping mute, coalescing and wording in `domain` keeps them table-tested, and leaves the daemon to wire ports.
- Coalescing sits on the loop, not in the worker, so a burst never fills the queue.

## Limits

- Focus is sticky. A session stays focused until another one is focused with `enter`, so leaving the TUI on session A with the terminal in front hides A's banners. Moving the selection alone does not focus.
- A coalesced banner is lost, not delayed. A `done` right after a `permission` on the same session, within 10 s, shows no second banner. The sidebar still shows the state.
- Codex has no waiting event: its hooks map to permission and done only.
- The frontmost check needs macOS Automation access for System Events. Without it the check fails and banners show even when the terminal is in front.

## Amendment, 2026-10-01 (#131): banner content

- `domain.BannerFor(BannerInput)` builds the banner from what the loop already holds: the session, its name, its worktrees and its recent `SessionEvent`s (the last 20, already kept for the session card), plus the time. Nothing new is read from disk or exec'd.
- Title: the name (or harness), then ` · repo@branch` from the first worktree (repo as its last path element, branch cut to 29 runes), `+N` for more worktrees; 80 runes at most.
- Body, 120 runes at most, from the current turn (events since the last prompt): permission gives `needs permission: <tool>: <target>` or the hook's message; waiting gives `asks: <question>` (the last paragraph of the last message, when it ends in `?`) or `waiting: <message>`; done gives the first line of the last assistant message and the time since the prompt, `usage limit: …` or `error: …` when that line reports a limit or an API error, or `done in 4m12s` with no message. With nothing known it falls back to the bare state word.
- Secrets are masked before the cut: `token=…`, `password: …`, `API_KEY=…` and well-known token prefixes (`sk-`, `ghp_`, `github_pat_`, `xoxb-`, `AKIA`). Only the first line of a message is shown, never a prompt.
- `internal/domain/testdata/banners.golden` pins a set of realistic banners.
- Tests never post a real banner: the e2e suite puts a fake `osascript` first on `PATH` that logs its argv to `$AGENTWS_E2E/osascript.log`, and `session_states.txtar` asserts banners reached it. Only the e2e suite runs `daemon.Run`; every other test injects a fake notifier.

Limits of the amendment:

- "N files changed" is not shown: the loop holds no per-turn file count (turn snapshots live in git). Done shows the message line and elapsed time instead.
- `osascript`'s `display notification` cannot group banners or open the session on click. That needs another notifier (such as `terminal-notifier`), left for a later issue.
- There is no error or limit agent state; those are recognised from the last message text only.
