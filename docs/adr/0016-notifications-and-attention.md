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
