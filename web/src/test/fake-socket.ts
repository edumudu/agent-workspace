import { act } from "@testing-library/react";
import type { SocketLike } from "../stream";

export class FakeSocket implements SocketLike {
  readonly sent: unknown[] = [];
  closed: { code?: number; reason?: string } | null = null;
  onopen: ((ev: unknown) => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: ((ev: { code: number; reason: string }) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;

  send(data: string): void {
    this.sent.push(JSON.parse(data));
  }

  close(code?: number, reason?: string): void {
    this.closed = { code, reason };
  }

  open(): void {
    act(() => this.onopen?.({}));
  }

  push(frame: unknown): void {
    act(() => this.onmessage?.({ data: JSON.stringify(frame) }));
  }

  drop(code: number, reason = ""): void {
    act(() => this.onclose?.({ code, reason }));
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
