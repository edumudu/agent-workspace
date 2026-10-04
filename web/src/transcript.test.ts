import { describe, expect, it } from "vitest";
import type { Message } from "./stream";
import { emptyTranscript, newestCursor, testResult, toolRow, withFrame, withPage } from "./transcript";

function msg(id: string, cursor: number, over: Partial<Message> = {}): Message {
  return { id, cursor, turn: "p1", role: "assistant", text: id, at: "2026-10-03T12:00:00Z", ...over };
}

const ids = (messages: Message[]) => messages.map((m) => m.id);

describe("a transcript", () => {
  it("starts empty with nothing loaded and no cursor", () => {
    const t = emptyTranscript();
    expect(t.loaded).toBe(false);
    expect(t.messages).toEqual([]);
    expect(newestCursor(t)).toBe(0);
  });

  it("takes the newest page and remembers where the older one starts", () => {
    const t = withPage(emptyTranscript(), { messages: [msg("u1", 10), msg("a1", 20)], before: 4 });
    expect(t.loaded).toBe(true);
    expect(t.before).toBe(4);
    expect(ids(t.messages)).toEqual(["u1", "a1"]);
    expect(newestCursor(t)).toBe(20);
  });

  it("puts an older page before the messages it holds", () => {
    let t = withPage(emptyTranscript(), { messages: [msg("u2", 30), msg("a2", 40)], before: 25 });
    t = withPage(t, { messages: [msg("u1", 10), msg("a1", 20)], before: 0 });
    expect(ids(t.messages)).toEqual(["u1", "a1", "u2", "a2"]);
    expect(t.before).toBe(0);
  });

  it("appends live messages and keeps messages of one line in the order they came", () => {
    let t = withPage(emptyTranscript(), { messages: [msg("u1", 10)], before: 0 });
    t = withFrame(t, { session: "s1", messages: [msg("a1", 20), msg("c1", 20, { role: "tool" })] });
    expect(ids(t.messages)).toEqual(["u1", "a1", "c1"]);
    expect(newestCursor(t)).toBe(20);
  });

  it("replaces a message that comes again with the same id, in place", () => {
    const running = msg("c1", 20, { role: "tool", text: "", tool: { name: "Bash", summary: "Bash make test", status: "running" } });
    let t = withPage(emptyTranscript(), { messages: [msg("u1", 10), running], before: 0 });
    t = withFrame(t, { session: "s1", messages: [msg("a2", 30)] });
    t = withFrame(t, { session: "s1", messages: [{ ...running, text: "ok", tool: { name: "Bash", summary: "Bash make test", status: "done" } }] });
    expect(ids(t.messages)).toEqual(["u1", "c1", "a2"]);
    expect(t.messages[1].tool?.status).toBe("done");
    expect(t.messages[1].text).toBe("ok");
    expect(newestCursor(t)).toBe(30);
  });

  it("starts over on a reset, and the older pages are those of the new file", () => {
    let t = withPage(emptyTranscript(), { messages: [msg("u1", 900)], before: 300 });
    t = withFrame(t, { session: "s1", messages: [msg("n1", 50)], reset: true });
    expect(ids(t.messages)).toEqual(["n1"]);
    expect(t.before).toBeNull();
    expect(newestCursor(t)).toBe(50);
  });

  it("marks a closed watch", () => {
    const t = withFrame(withPage(emptyTranscript(), { messages: [], before: 0 }), { session: "s1", messages: [], closed: true });
    expect(t.closed).toBe(true);
  });
});

describe("a tool row", () => {
  it("splits the tool name off its summary", () => {
    expect(toolRow(msg("c1", 1, { role: "tool", text: "", tool: { name: "Bash", summary: "Bash go test ./...", status: "running" } }))).toEqual({
      name: "Bash",
      detail: "go test ./...",
      status: "running",
      result: "",
    });
    expect(toolRow(msg("c2", 1, { role: "tool", text: "", tool: { name: "Shell", summary: "Shell ls", status: "done" } })).detail).toBe("ls");
    expect(toolRow(msg("c3", 1, { role: "tool", text: "", tool: { name: "TodoWrite", summary: "TodoWrite", status: "done" } })).detail).toBe("");
    expect(toolRow(msg("c4", 1, { role: "tool", text: "", tool: { name: "Bash", summary: "go test", status: "done" } })).detail).toBe("go test");
  });

  it("carries the test result its output shows", () => {
    const row = toolRow(msg("c1", 1, { role: "tool", text: "ok  \tgithub.com/x/api\t0.2s\nok  \tgithub.com/x/web\t0.1s", tool: { name: "Bash", summary: "Bash go test ./...", status: "done" } }));
    expect(row.result).toBe("2 packages ok");
  });
});

describe("a test result", () => {
  it.each([
    ["ok  \tgithub.com/x/api\t0.2s\nok  \tgithub.com/x/web\t(cached)", "2 packages ok"],
    ["ok  \tgithub.com/x/api\t0.2s\n--- FAIL: TestLogin (0.00s)\nFAIL\tgithub.com/x/web\t0.1s\nFAIL", "1 package failed, 1 ok"],
    [" Test Files  3 passed (3)\n      Tests  41 passed (41)", "41 passed"],
    ["Tests:       2 failed, 9 passed, 11 total", "2 failed, 9 passed"],
    ["===== 1 failed, 12 passed in 0.31s =====", "1 failed, 12 passed"],
    ["test result: ok. 8 passed; 0 failed; 0 ignored", "8 passed"],
    ["test result: FAILED. 7 passed; 1 failed; 0 ignored", "7 passed, 1 failed"],
    ["Compiling...\nBuild succeeded", ""],
    ["", ""],
  ])("reads %j as %j", (text, want) => {
    expect(testResult(text)).toBe(want);
  });
});
