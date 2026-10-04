import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { ApiError, apiVersion, hello, interrupt, messagesPage, pair, sendMessage, unsend } from "./api";
import { FakeServer } from "./test/fake-server";

const device = { id: "k3m9p2qx", name: "iPhone", created_at: "2026-10-03T10:00:00Z", last_seen: "2026-10-03T10:00:00Z" };

async function failure(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise;
  } catch (err) {
    if (err instanceof ApiError) {
      return err;
    }
    throw err;
  }
  throw new Error("expected the call to fail");
}

describe("hello", () => {
  it("reads the API version and build without a token", async () => {
    const server = new FakeServer().on("GET", "/api/v1/hello", { status: 200, body: { api: "v1", build: "v0.12.0+abc" } });
    await expect(hello(server.fetch)).resolves.toEqual({ api: "v1", build: "v0.12.0+abc" });
  });

  it("names the API version with a v whatever the server sends", () => {
    expect(apiVersion({ api: "v1", build: "x" })).toBe("v1");
    expect(apiVersion({ api: 1 as unknown as string, build: "x" })).toBe("v1");
  });
});

describe("the server's golden responses", () => {
  const golden = (name: string) => JSON.parse(readFileSync(new URL("../../internal/serve/testdata/" + name, import.meta.url), "utf8"));

  it("reads hello", async () => {
    const server = new FakeServer().on("GET", "/api/v1/hello", { status: 200, body: golden("hello.json") });
    const h = await hello(server.fetch);
    expect(apiVersion(h)).toBe("v1");
    expect(h.build).toBe("v0.12.0+test");
  });

  it("reads a pairing", async () => {
    const server = new FakeServer().on("POST", "/api/v1/pair", { status: 200, body: golden("pair.json") });
    const paired = await pair(server.fetch, "ABCD2345", "phone");
    expect(paired.token).toBe("dGhlLXRva2Vu");
    expect(paired.device).toEqual({ id: "k3m9p2qx", name: "phone", created_at: "2026-10-03T12:00:00Z", last_seen: "2026-10-03T12:00:00Z" });
  });

  it("reads an error", async () => {
    const server = new FakeServer().on("POST", "/api/v1/pair", { status: 401, body: golden("error.json") });
    const err = await failure(pair(server.fetch, "ABCD2345", "phone"));
    expect(err.code).toBe("unauthorized");
    expect(err.message).toBe("the pairing code is wrong or has expired");
  });
});

describe("pair", () => {
  it("swaps the code and device name for a device and token", async () => {
    const server = new FakeServer().on("POST", "/api/v1/pair", { status: 200, body: { device, token: "t0k" } });
    await expect(pair(server.fetch, "ABCD2345", "iPhone")).resolves.toEqual({ device, token: "t0k" });
    expect(server.calls).toMatchObject([{ path: "/api/v1/pair", method: "POST", body: { code: "ABCD2345", name: "iPhone" } }]);
    expect(server.calls[0].headers.authorization).toBeUndefined();
  });

  it.each([
    [401, { error: { code: "unauthorized", message: "the pairing code is wrong or has expired" } }, "unauthorized"],
    [429, { error: { code: "rate_limited", message: "too many failed pairing tries; wait a minute" } }, "rate_limited"],
    [401, { code: "unauthorized", message: "flat" }, "unauthorized"],
    [401, undefined, "unauthorized"],
    [429, undefined, "rate_limited"],
    [500, undefined, "failed"],
  ])("maps a %i answer to %j to the error code", async (status, body, code) => {
    const server = new FakeServer().on("POST", "/api/v1/pair", { status, body });
    const err = await failure(pair(server.fetch, "ABCD2345", "iPhone"));
    expect(err.status).toBe(status);
    expect(err.code).toBe(code);
  });

  it("keeps the server's message", async () => {
    const server = new FakeServer().on("POST", "/api/v1/pair", {
      status: 401,
      body: { error: { code: "unauthorized", message: "the pairing code is wrong or has expired" } },
    });
    expect((await failure(pair(server.fetch, "ABCD2345", "x"))).message).toBe("the pairing code is wrong or has expired");
  });

  it("reports an unreachable server as a network error", async () => {
    const server = new FakeServer().on("POST", "/api/v1/pair", new TypeError("Load failed"));
    const err = await failure(pair(server.fetch, "ABCD2345", "iPhone"));
    expect(err.code).toBe("network");
    expect(err.status).toBe(0);
  });

  it("survives an error page that is not JSON", async () => {
    const server = new FakeServer().on("POST", "/api/v1/pair", { status: 502, raw: "<html>Bad Gateway</html>" });
    const err = await failure(pair(server.fetch, "ABCD2345", "iPhone"));
    expect(err.code).toBe("failed");
    expect(err.message).toContain("Bad Gateway");
  });
});

describe("session calls", () => {
  const golden = (name: string) => JSON.parse(readFileSync(new URL("../../internal/serve/testdata/" + name, import.meta.url), "utf8"));
  const authed = (server: FakeServer) => ({ fetch: server.fetch, token: "t0k" });

  it("pages a transcript with the device token, newest page first", async () => {
    const server = new FakeServer()
      .on("GET", "/api/v1/sessions/s%2F1/messages?limit=50", { status: 200, body: golden("messages.json") })
      .on("GET", "/api/v1/sessions/s%2F1/messages?before=40&limit=50", { status: 200, body: { messages: [], before: 0 } });
    const page = await messagesPage(authed(server), "s/1");
    expect(page.before).toBe(40);
    expect(page.messages.map((m) => m.id)).toEqual(["u1", "c1"]);
    expect(page.messages[1].tool).toEqual({ name: "Bash", summary: "go test ./...", status: "done" });
    await expect(messagesPage(authed(server), "s/1", 40)).resolves.toEqual({ messages: [], before: 0 });
    expect(server.calls.map((c) => c.headers.authorization)).toEqual(["Bearer t0k", "Bearer t0k"]);
  });

  it("sends a message and reads whether it queued", async () => {
    const server = new FakeServer().on("POST", "/api/v1/sessions/s1/messages", { status: 200, body: golden("send.json") });
    await expect(sendMessage(authed(server), "s1", "run the tests")).resolves.toEqual({ id: "q7", queued: true });
    expect(server.calls).toMatchObject([{ method: "POST", body: { text: "run the tests" }, headers: { authorization: "Bearer t0k" } }]);
  });

  it("drops a queued send and interrupts the session", async () => {
    const server = new FakeServer()
      .on("DELETE", "/api/v1/sessions/s1/sends/q7", { status: 200, body: {} })
      .on("POST", "/api/v1/sessions/s1/interrupt", { status: 200, body: {} });
    await unsend(authed(server), "s1", "q7");
    await interrupt(authed(server), "s1");
    expect(server.calls.map((c) => c.method + " " + c.path + " " + c.headers.authorization)).toEqual([
      "DELETE /api/v1/sessions/s1/sends/q7 Bearer t0k",
      "POST /api/v1/sessions/s1/interrupt Bearer t0k",
    ]);
  });

  it("reports a send that already went out as not_found", async () => {
    const server = new FakeServer().on("DELETE", "/api/v1/sessions/s1/sends/q7", {
      status: 404,
      body: { error: { code: "not_found", message: "no queued send q7" } },
    });
    expect((await failure(unsend(authed(server), "s1", "q7"))).code).toBe("not_found");
  });
});
