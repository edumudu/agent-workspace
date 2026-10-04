import { act } from "@testing-library/react";
import type { SocketLike } from "../stream";

export class FakeSocket implements SocketLike {
  readonly sent: unknown[] = [];
  closed: { code?: number; reason?: string } | null = null;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;

  send(data: string): void {
    this.sent.push(JSON.parse(data));
  }

  close(code?: number, reason?: string): void {
    this.closed = { code, reason };
  }

  open(): void {
    act(() => this.onopen?.(new Event("open")));
  }

  push(frame: unknown): void {
    act(() => this.onmessage?.(new MessageEvent("message", { data: JSON.stringify(frame) })));
  }

  drop(code: number, reason = ""): void {
    act(() => this.onclose?.(new CloseEvent("close", { code, reason })));
  }
}

export class FakeSockets {
  readonly opened: FakeSocket[] = [];

  readonly open = (): SocketLike => {
    const socket = new FakeSocket();
    this.opened.push(socket);
    return socket;
  };

  get last(): FakeSocket {
    const socket = this.opened[this.opened.length - 1];
    if (!socket) {
      throw new Error("no socket was opened");
    }
    return socket;
  }
}
