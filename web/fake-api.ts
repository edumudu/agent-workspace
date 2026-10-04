import { createHash } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";
import type { Duplex } from "node:stream";
import type { Plugin } from "vite";

export const fakePairCode = "ABCD2345";
export const fakeRateLimitedCode = "ZZZZZZZZ";

function send(res: ServerResponse, status: number, body: unknown) {
  res.statusCode = status;
  res.setHeader("Content-Type", "application/json");
  res.end(JSON.stringify(body));
}

function readJSON(req: IncomingMessage): Promise<Record<string, unknown>> {
  return new Promise((resolve) => {
    let raw = "";
    req.on("data", (chunk) => {
      raw += chunk;
    });
    req.on("end", () => {
      try {
        resolve(JSON.parse(raw || "{}"));
      } catch {
        resolve({});
      }
    });
  });
}

const minute = 60000;

function fakeSession(id: string, over: Record<string, unknown>) {
  return {
    ID: id,
    TaskID: "t-" + id,
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

export function fakeStreamState(now = Date.now()) {
  const ago = (minutes: number) => new Date(now - minutes * minute).toISOString();
  const quota = (harness: string, window: string, label: string, left: number, resetsIn: number) => ({
    Harness: harness,
    Window: window,
    LeftPercent: left,
    ResetsAt: resetsIn ? Math.floor((now + resetsIn * minute) / 1000) : 0,
    ReportedAt: ago(1),
    label,
    low: left < 20,
    stale_at: new Date(now + 14 * minute).toISOString(),
  });
  return {
    seq: 1,
    workspaces: [],
    tasks: [],
    worktrees: [],
    sessions: [
      fakeSession("s1", { State: "permission", name: "fix the login redirect", where: "api@login", banner: "needs permission: Bash: make test", since: ago(3) }),
      fakeSession("s2", { Harness: "codex", State: "waiting", name: "pick a cache key", where: "web@cache", banner: "asks: Which TTL should the cache use?", since: ago(12) }),
      fakeSession("s3", { State: "running", name: "add retries", where: "api@retries", since: ago(8) }),
      fakeSession("s4", { State: "done", Unread: true, name: "update the docs", where: "docs@setup", banner: "Rewrote the setup section. (6m40s)", since: ago(20) }),
      fakeSession("s5", { Harness: "codex", State: "done", name: "bump deps", where: "web@deps", banner: "Bumped vite and vitest; the tests pass. (3m2s)", since: ago(125) }),
      fakeSession("s6", { State: "idle", name: "explore the auth flow", where: "api@main", since: ago(26 * 60) }),
    ],
    limits: [quota("claude", "five_hour", "5h", 38, 130), quota("claude", "seven_day", "7d", 19, 3 * 24 * 60), quota("codex", "five_hour", "5h", 77, 200)],
    queue: [],
    sends: [],
  };
}

function wsFrame(text: string): Buffer {
  const payload = Buffer.from(text);
  const len = payload.length;
  const head = len < 126 ? Buffer.from([0x81, len]) : Buffer.from([0x81, 126, len >> 8, len & 255]);
  return Buffer.concat([head, payload]);
}

function wsClose(code: number): Buffer {
  return Buffer.from([0x88, 2, code >> 8, code & 255]);
}

let offlineServed = false;

function fakeStream(req: IncomingMessage, socket: Duplex) {
  const offline = process.env.AGENTWS_FAKE_STREAM === "offline";
  if (offline && offlineServed) {
    socket.destroy();
    return;
  }
  const key = String(req.headers["sec-websocket-key"] ?? "");
  const accept = createHash("sha1").update(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").digest("base64");
  socket.write(["HTTP/1.1 101 Switching Protocols", "Upgrade: websocket", "Connection: Upgrade", "Sec-WebSocket-Accept: " + accept, "", ""].join("\r\n"));
  let authed = false;
  socket.on("data", (chunk: Buffer) => {
    if (authed || (chunk[0] & 0x0f) !== 1) {
      return;
    }
    authed = true;
    socket.write(wsFrame(JSON.stringify({ state: fakeStreamState() })));
    if (offline) {
      offlineServed = true;
      setTimeout(() => socket.end(wsClose(1013)), 300);
    }
  });
  socket.on("error", () => undefined);
}

export function fakeApi(build = "v0.12.0+demo"): Plugin {
  return {
    name: "agentws-fake-api",
    configureServer(server) {
      server.httpServer?.on("upgrade", (req: IncomingMessage, socket: Duplex) => {
        if ((req.url ?? "").split("?")[0] === "/api/v1/stream") {
          fakeStream(req, socket);
        }
      });
      server.middlewares.use(async (req, res, next) => {
        const path = (req.url ?? "").split("?")[0];
        if (req.method === "GET" && path === "/api/v1/hello") {
          send(res, 200, { api: "v1", build });
          return;
        }
        if (req.method === "POST" && path === "/api/v1/pair") {
          const body = await readJSON(req);
          const code = String(body.code ?? "");
          if (code === fakeRateLimitedCode) {
            send(res, 429, { error: { code: "rate_limited", message: "too many failed pairing tries; wait a minute" } });
            return;
          }
          if (code !== fakePairCode) {
            send(res, 401, { error: { code: "unauthorized", message: "the pairing code is wrong or has expired" } });
            return;
          }
          const now = new Date().toISOString();
          send(res, 200, {
            device: { id: "k3m9p2qx", name: String(body.name ?? "phone"), created_at: now, last_seen: now },
            token: "demo-token",
          });
          return;
        }
        if (req.method === "GET" && path === "/api/v1/workspaces") {
          send(res, 200, {
            workspaces: [
              { Root: "/home/me/api", Kind: "single", Repos: [], LastUsed: "2026-10-03T12:00:00Z" },
              { Root: "/home/me/web", Kind: "single", Repos: [], LastUsed: "2026-10-01T12:00:00Z" },
            ],
            last_used: "/home/me/api",
          });
          return;
        }
        if (req.method === "GET" && path === "/api/v1/work-items/resolve") {
          const query = new URL(req.url ?? "", "http://fake").searchParams;
          const item = query.get("item") ?? "";
          const workspace = query.get("workspace") ?? "";
          if (item.startsWith("http") && !item.includes("linear.app")) {
            send(res, 400, { error: { code: "bad_request", message: "unsupported link: use a Linear issue or GitHub pull request URL" } });
            return;
          }
          if (item.includes("linear.app")) {
            send(res, 200, { source: "linear", ref: "ENG-12", title: "Fix the login redirect", worktree: "eng-12", workspace });
            return;
          }
          send(res, 200, { source: "text", title: item, worktree: item.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, ""), workspace });
          return;
        }
        if (req.method === "POST" && path === "/api/v1/sessions") {
          const body = await readJSON(req);
          if (String(body.work_item ?? "").includes("broken")) {
            send(res, 500, {
              error: { code: "failed", message: "setup /home/me/.agentws/worktrees/api/broken: npm ci: exit status 1\nnpm ERR! code EUSAGE\nnpm ERR! missing lockfile" },
            });
            return;
          }
          send(res, 200, fakeSession("s9", { name: String(body.work_item ?? ""), State: "running" }));
          return;
        }
        if (path.startsWith("/api/")) {
          send(res, 404, { error: { code: "not_found", message: "not in the fake API" } });
          return;
        }
        next();
      });
    },
  };
}
