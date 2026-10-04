import type { MessagesPage } from "./api";
import type { Message, TranscriptFrame } from "./stream";

export type Transcript = {
  messages: Message[];
  before: number | null;
  loaded: boolean;
  closed: boolean;
};

export function emptyTranscript(): Transcript {
  return { messages: [], before: null, loaded: false, closed: false };
}

function merge(first: Message[], second: Message[]): Message[] {
  const out: Message[] = [];
  const at = new Map<string, number>();
  for (const m of [...first, ...second]) {
    const i = at.get(m.id);
    if (i === undefined) {
      at.set(m.id, out.length);
      out.push(m);
    } else {
      out[i] = m;
    }
  }
  return out.sort((a, b) => a.cursor - b.cursor);
}

export function withPage(t: Transcript, page: MessagesPage): Transcript {
  return { ...t, messages: merge(page.messages, t.messages), before: page.before, loaded: true };
}

export function withFrame(t: Transcript, frame: TranscriptFrame): Transcript {
  if (frame.reset) {
    return { ...t, messages: merge([], frame.messages), before: null, closed: Boolean(frame.closed) };
  }
  return { ...t, messages: merge(t.messages, frame.messages), closed: t.closed || Boolean(frame.closed) };
}

export function newestCursor(t: Transcript): number {
  return t.messages.reduce((max, m) => Math.max(max, m.cursor), 0);
}

export type ToolRow = {
  name: string;
  detail: string;
  status: string;
  result: string;
};

export function toolRow(m: Message): ToolRow {
  const name = m.tool?.name ?? "";
  const summary = m.tool?.summary ?? "";
  let detail = summary;
  if (summary === name) {
    detail = "";
  } else if (name && summary.startsWith(name + " ")) {
    detail = summary.slice(name.length + 1);
  }
  return { name, detail, status: m.tool?.status ?? "", result: testResult(m.text ?? "") };
}

function plural(n: number, word: string): string {
  return n + " " + word + (n === 1 ? "" : "s");
}

function goResult(text: string): string {
  const ok = text.match(/^ok\s+\S+/gm)?.length ?? 0;
  const failed = text.match(/^FAIL\s+\S+/gm)?.length ?? 0;
  if (failed > 0) {
    return plural(failed, "package") + " failed" + (ok > 0 ? ", " + ok + " ok" : "");
  }
  return ok > 0 ? plural(ok, "package") + " ok" : "";
}

function countsLine(text: string): string {
  const lines = text.split("\n");
  const tests = lines.find((l) => /^\s*Tests:?\s/.test(l) && /\d+ (passed|failed)/.test(l));
  if (tests) {
    return tests;
  }
  return lines.filter((l) => /\d+ (passed|failed)/.test(l)).pop() ?? "";
}

export function testResult(text: string): string {
  const go = goResult(text);
  if (go) {
    return go;
  }
  const counts = [...countsLine(text).matchAll(/(\d+) (passed|failed)/g)].filter((m) => m[1] !== "0").map((m) => m[1] + " " + m[2]);
  return counts.join(", ");
}
