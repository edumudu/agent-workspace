import type { Fetch } from "../api";

export type FakeCall = { path: string; method: string; body: unknown };

export type FakeRoute = { status: number; body?: unknown; raw?: string };

export type FakeHandler = (call: FakeCall) => FakeRoute | Promise<FakeRoute>;

function headersOf(init?: RequestInit): Record<string, string> {
  const out: Record<string, string> = {};
  new Headers(init?.headers).forEach((value, key) => {
    out[key] = value;
  });
  return out;
}

export class FakeServer {
  readonly calls: FakeCall[] = [];
  readonly headers: Record<string, string>[] = [];
  private routes = new Map<string, FakeRoute | Error | FakeHandler>();

  on(method: string, path: string, route: FakeRoute | Error | FakeHandler): this {
    this.routes.set(method + " " + path, route);
    return this;
  }

  readonly fetch: Fetch = async (input, init) => {
    const method = init?.method ?? "GET";
    const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
    const call = { path: input, method, body };
    this.calls.push(call);
    this.headers.push(headersOf(init));
    let route = this.routes.get(method + " " + input);
    if (route instanceof Error) {
      throw route;
    }
    if (typeof route === "function") {
      route = await route(call);
    }
    if (!route) {
      return new Response(JSON.stringify({ error: { code: "not_found", message: "no route" } }), { status: 404 });
    }
    const text = route.raw ?? (route.body === undefined ? "" : JSON.stringify(route.body));
    return new Response(text, { status: route.status, headers: { "Content-Type": "application/json" } });
  };
}

export class Deferred<T> {
  readonly promise: Promise<T>;
  resolve!: (value: T) => void;

  constructor() {
    this.promise = new Promise<T>((resolve) => {
      this.resolve = resolve;
    });
  }
}

export class MemoryStorage {
  private values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }
}
