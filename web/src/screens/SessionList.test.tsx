import { readFileSync } from "node:fs";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { App, type AppEnv } from "../App";
import type { Frame, StreamState } from "../stream";
import { FakeServer, MemoryStorage } from "../test/fake-server";
import { FakeSockets } from "../test/fake-socket";

const golden = (name: string) => JSON.parse(readFileSync(new URL("../../../internal/serve/testdata/" + name, import.meta.url), "utf8"));
const goldenState = (): StreamState => (golden("stream-state.json") as Frame).state as StreamState;
const goldenDiffs = (): Frame[] => golden("stream-diffs.json") as Frame[];
const device = { id: "k3m9p2qx", name: "iPhone", created_at: "2026-10-03T10:00:00Z", last_seen: "2026-10-03T10:00:00Z" };
const halfPastNoon = new Date("2026-10-03T12:30:00Z").getTime();

function setup(hash = "") {
  window.location.hash = hash;
  const sockets = new FakeSockets();
  const storage = new MemoryStorage();
  storage.setItem("agentws.auth", JSON.stringify({ token: "t0k", device }));
  const server = new FakeServer().on("GET", "/api/v1/hello", { status: 200, body: { api: "v1", build: "v0.12.0+abc" } });
  const env: AppEnv = {
    fetch: server.fetch,
    storage,
    installed: true,
    hash,
    host: "agentws.example.ts.net",
    userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)",
    openStream: sockets.open,
    retryDelay: () => 5,
    now: () => halfPastNoon,
    onHashChange: (cb) => {
      const listener = () => cb(window.location.hash);
      window.addEventListener("hashchange", listener);
      return () => window.removeEventListener("hashchange", listener);
    },
  };
  render(<App env={env} />);
  return { sockets, storage };
}

function live(sockets: FakeSockets, state: StreamState = goldenState()) {
  sockets.last.open();
  sockets.last.push({ state });
}

beforeEach(() => {
  window.location.hash = "";
});

afterEach(() => {
  window.location.hash = "";
});

describe("the session list", () => {
  it("groups sessions and shows each one's name, repo@branch · harness, banner line and time in state", () => {
    const { sockets } = setup();
    live(sockets);
    expect(sockets.last.sent[0]).toEqual({ token: "t0k" });
    const working = within(screen.getByRole("region", { name: "Working" }));
    const run = working.getByRole("link", { name: /fix the login redirect/ });
    expect(run).toHaveTextContent("api@login · claude");
    expect(run).toHaveTextContent("30m");
    const done = within(screen.getByRole("region", { name: "Done" }));
    const finished = done.getByRole("link", { name: /add retries/ });
    expect(finished).toHaveTextContent("codex");
    expect(finished).toHaveTextContent("Added the retry to the client. (4m12s)");
    expect(finished).toHaveTextContent("35m");
    expect(within(finished).getByText("unread")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Needs you" })).not.toBeInTheDocument();
  });

  it("moves a session that asks for permission to Needs you without a reload", () => {
    const { sockets } = setup();
    live(sockets);
    sockets.last.push(goldenDiffs()[0]);
    const needs = within(screen.getByRole("region", { name: "Needs you" }));
    expect(needs.getByRole("link", { name: /fix the login redirect/ })).toHaveTextContent("needs permission: Bash: make test");
    expect(screen.queryByRole("region", { name: "Working" })).not.toBeInTheDocument();
  });

  it("shows the Claude and Codex limits in the header", () => {
    const { sockets } = setup();
    live(sockets);
    expect(screen.getByLabelText("Claude limits")).toHaveTextContent(/5h\s*42%/);
    expect(screen.getByLabelText("Claude limits")).toHaveTextContent(/7d\s*85%/);
    expect(screen.getByLabelText("Codex limits")).toHaveTextContent(/5h\s*10%/);
  });

  it("says when there are no sessions", () => {
    const { sockets } = setup();
    live(sockets, { ...goldenState(), sessions: [], limits: [] });
    expect(screen.getByText("No sessions yet")).toBeInTheDocument();
  });

  it("shows an offline state when the connection drops and recovers on its own", async () => {
    const { sockets } = setup();
    live(sockets);
    sockets.last.drop(1013, "the agentws daemon went away");
    expect(screen.getByRole("status")).toHaveTextContent(/Offline/);
    expect(screen.getByRole("link", { name: /fix the login redirect/ })).toBeInTheDocument();
    await waitFor(() => expect(sockets.opened).toHaveLength(2));
    live(sockets);
    expect(screen.queryByText(/Offline/)).not.toBeInTheDocument();
  });

  it("reconnects at once from the offline state when asked", async () => {
    const { sockets } = setup();
    live(sockets);
    sockets.last.drop(1006);
    await userEvent.click(screen.getByRole("button", { name: "Retry now" }));
    expect(sockets.opened.length).toBeGreaterThanOrEqual(2);
  });

  it("goes back to pairing when the stream refuses the token", async () => {
    const { sockets, storage } = setup();
    sockets.last.open();
    sockets.last.drop(4401, "device revoked");
    expect(await screen.findByRole("heading", { name: "Pair this device" })).toBeInTheDocument();
    expect(storage.getItem("agentws.auth")).toBeNull();
  });
});

describe("opening a session", () => {
  it("opens the session when its card is tapped, at a URL a notification can open", async () => {
    const { sockets } = setup();
    live(sockets);
    await userEvent.click(screen.getByRole("link", { name: /fix the login redirect/ }));
    expect(await screen.findByRole("heading", { name: "fix the login redirect" })).toBeInTheDocument();
    expect(window.location.hash).toBe("#/sessions/s1");
  });

  it("starts on the session from its URL and goes back to the list", async () => {
    const { sockets } = setup("#/sessions/s2");
    live(sockets);
    expect(screen.getByRole("heading", { name: "add retries" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("link", { name: "Sessions" }));
    expect(await screen.findByRole("region", { name: "Working" })).toBeInTheDocument();
  });
});
