import { describe, expect, it } from "vitest";
import { groupSessions, limitRows, sessionMeta, timeInState } from "./sessions";
import type { Quota, Session } from "./stream";

const now = Date.parse("2026-10-03T12:00:00Z");

function session(over: Partial<Session>): Session {
  return {
    ID: "s",
    TaskID: "t",
    Harness: "claude",
    Pane: "",
    Model: "",
    Effort: "",
    State: "idle",
    Ended: false,
    Unread: false,
    Focused: false,
    Muted: false,
    WorktreeIDs: null,
    ResumeID: "",
    Transcript: "",
    Dir: "",
    Usage: { ContextLeftPercent: 0, HasContext: false, LimitUsedPercent: 0 },
    Limits: null,
    LimitsAt: "0001-01-01T00:00:00Z",
    Switches: null,
    SwitchWarning: false,
    name: "",
    where: "",
    banner: "",
    since: null,
    ...over,
  };
}

const ago = (minutes: number) => new Date(now - minutes * 60000).toISOString();

describe("groupSessions", () => {
  it("groups by what each session needs, in attention order, and leaves out empty groups and ended sessions", () => {
    const groups = groupSessions([
      session({ ID: "idle", State: "idle" }),
      session({ ID: "run", State: "running" }),
      session({ ID: "wait", State: "waiting" }),
      session({ ID: "ended", State: "done", Ended: true }),
      session({ ID: "perm", State: "permission" }),
    ]);
    expect(groups.map((g) => [g.title, g.sessions.map((s) => s.ID)])).toEqual([
      ["Needs you", ["perm", "wait"]],
      ["Working", ["run"]],
      ["Idle", ["idle"]],
    ]);
  });

  it("puts permission before waiting, then whoever has waited longest", () => {
    const groups = groupSessions([
      session({ ID: "w-new", State: "waiting", since: ago(1) }),
      session({ ID: "p-new", State: "permission", since: ago(2) }),
      session({ ID: "w-old", State: "waiting", since: ago(30) }),
      session({ ID: "p-old", State: "permission", since: ago(20) }),
    ]);
    expect(groups[0].sessions.map((s) => s.ID)).toEqual(["p-old", "p-new", "w-old", "w-new"]);
  });

  it("shows unread done sessions first, then the most recent", () => {
    const groups = groupSessions([
      session({ ID: "read-new", State: "done", since: ago(1) }),
      session({ ID: "unread-old", State: "done", Unread: true, since: ago(50) }),
      session({ ID: "read-old", State: "done", since: ago(40) }),
      session({ ID: "unread-new", State: "done", Unread: true, since: ago(5) }),
    ]);
    expect(groups.map((g) => [g.title, g.sessions.map((s) => s.ID)])).toEqual([
      ["Done", ["unread-new", "unread-old", "read-new", "read-old"]],
    ]);
  });

  it("orders working and idle sessions newest first, with an unknown time last", () => {
    const groups = groupSessions([
      session({ ID: "a", State: "running" }),
      session({ ID: "b", State: "running", since: ago(9) }),
      session({ ID: "c", State: "running", since: ago(2) }),
    ]);
    expect(groups[0].sessions.map((s) => s.ID)).toEqual(["c", "b", "a"]);
  });
});

describe("timeInState", () => {
  it.each([
    [null, ""],
    [ago(0.5), "now"],
    [ago(12), "12m"],
    [ago(60), "1h"],
    [ago(125), "2h5m"],
    [ago(60 * 24 * 3 + 7), "3d"],
    [new Date(now + 60000).toISOString(), "now"],
  ])("%s reads %s", (since, want) => {
    expect(timeInState(since, now)).toBe(want);
  });
});

describe("sessionMeta", () => {
  it("joins the worktree label and the harness", () => {
    expect(sessionMeta(session({ where: "api@login", Harness: "codex" }))).toBe("api@login · codex");
    expect(sessionMeta(session({ Harness: "claude" }))).toBe("claude");
  });
});

describe("limitRows", () => {
  const local = new Date(2026, 9, 3, 12, 0).getTime();
  function quota(over: Partial<Quota>): Quota {
    return {
      Harness: "claude",
      Window: "five_hour",
      LeftPercent: 58,
      ResetsAt: 0,
      ReportedAt: new Date(local).toISOString(),
      label: "5h",
      low: false,
      stale_at: new Date(local + 15 * 60000).toISOString(),
      ...over,
    };
  }

  it("gives one row per harness, Claude first, with percent used and the reset clock", () => {
    const rows = limitRows(
      [
        quota({ Harness: "codex", LeftPercent: 90 }),
        quota({ LeftPercent: 58, ResetsAt: new Date(2026, 9, 3, 15, 4).getTime() / 1000 }),
        quota({ Window: "seven_day", label: "7d", LeftPercent: 15, low: true, ResetsAt: new Date(2026, 9, 6, 9, 0).getTime() / 1000 }),
      ],
      local,
    );
    expect(rows).toEqual([
      {
        harness: "claude",
        tag: "CC",
        stale: "",
        windows: [
          { label: "5h", used: 42, low: false, stale: false, resets: "15:04" },
          { label: "7d", used: 85, low: true, stale: false, resets: "Tue" },
        ],
      },
      { harness: "codex", tag: "CX", stale: "", windows: [{ label: "5h", used: 10, low: false, stale: false, resets: "" }] },
    ]);
  });

  it("drops windows that have already reset and harnesses with none left", () => {
    expect(limitRows([quota({ ResetsAt: (local - 1000) / 1000 })], local)).toEqual([]);
  });

  it("marks a report older than its stale time and says how old it is", () => {
    const rows = limitRows([quota({ ReportedAt: new Date(local - 40 * 60000).toISOString(), stale_at: new Date(local - 25 * 60000).toISOString() })], local);
    expect(rows[0].stale).toBe("40m ago");
    expect(rows[0].windows[0].stale).toBe(true);
  });
});
