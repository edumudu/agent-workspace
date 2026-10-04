import { useCallback, useEffect, useLayoutEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import Markdown from "react-markdown";
import { ApiError, interrupt, messagesPage, sendMessage, unsend, type Authed } from "../api";
import { timeInState } from "../sessions";
import type { Message, QueuedSend, Session, StreamClient, StreamSnapshot, StreamStatus } from "../stream";
import { emptyTranscript, newestCursor, toolRow, withFrame, withPage, type Transcript } from "../transcript";
import { Connection, sessionTitle } from "./SessionList";

const stateLabels: Record<string, string> = {
  running: "working",
  permission: "needs permission",
  waiting: "waiting for you",
  done: "done",
  idle: "idle",
};

const nearTop = 120;
const nearBottom = 80;

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function stateOf(s: Session): string {
  return s.Ended ? "ended" : s.State;
}

function busy(s: Session): boolean {
  return s.State === "running" || s.State === "permission";
}

function Back() {
  return (
    <a className="back" href="#/">
      <span aria-hidden="true">{"‹ "}</span>
      Sessions
    </a>
  );
}

function Header({ session, now, onInterrupt }: { session: Session; now: number; onInterrupt: () => void }) {
  const age = timeInState(session.since, now);
  const state = stateOf(session);
  const meta = [session.where, [session.Harness, session.Model].filter(Boolean).join(" ")].filter(Boolean).join(" · ");
  return (
    <header className="chat-top">
      <div className="chat-bar">
        <Back />
        {busy(session) && !session.Ended ? (
          <button type="button" className="interrupt" onClick={onInterrupt}>
            Interrupt
          </button>
        ) : null}
      </div>
      <h1>{sessionTitle(session)}</h1>
      <p className="meta">{meta}</p>
      <p className="chat-facts">
        <span className={"chip chip-" + state}>{stateLabels[state] ?? state}</span>
        {session.Usage.HasContext ? <span>{session.Usage.ContextLeftPercent + "% context left"}</span> : null}
        {age ? <span>{age}</span> : null}
      </p>
      {session.banner ? <p className="banner">{session.banner}</p> : null}
    </header>
  );
}

function ToolLine({ message }: { message: Message }) {
  const row = toolRow(message);
  const label = [row.name, row.detail].filter(Boolean).join(" ");
  return (
    <details data-message className={"tool tool-" + row.status} role="group" aria-label={label}>
      <summary>
        <span className="tool-dot" aria-hidden="true" />
        <span className="tool-name">{row.name}</span>
        <span className="tool-detail">{row.detail}</span>
        {row.status === "running" ? <span className="live">running</span> : null}
        {row.status === "failed" ? <span className="failed">failed</span> : null}
        {row.result ? <span className="tool-result">{row.result}</span> : null}
      </summary>
      {message.text ? <pre className="tool-output">{message.text}</pre> : <p className="muted">No output yet</p>}
    </details>
  );
}

function MessageLine({ message }: { message: Message }) {
  if (message.role === "tool" || message.tool) {
    return <ToolLine message={message} />;
  }
  if (message.role === "user") {
    return (
      <div data-message className="msg msg-user">
        <p>{message.text}</p>
      </div>
    );
  }
  if (message.role === "system") {
    return (
      <div data-message className="msg msg-system">
        <p>{message.text}</p>
      </div>
    );
  }
  return (
    <div data-message className="msg msg-assistant">
      <Markdown skipHtml>{message.text ?? ""}</Markdown>
    </div>
  );
}

function OlderRow({ t, loading, failed, onLoad }: { t: Transcript; loading: boolean; failed: boolean; onLoad: () => void }) {
  if (!t.loaded) {
    return <div className="older" />;
  }
  if (loading) {
    return (
      <div className="older" role="status">
        Loading earlier messages…
      </div>
    );
  }
  if (t.before === 0) {
    return <div className="older muted">Start of the conversation</div>;
  }
  return (
    <div className="older">
      <button type="button" className="load-older" onClick={onLoad}>
        {failed ? "Couldn’t load earlier messages · Retry" : "Load earlier messages"}
      </button>
    </div>
  );
}

function Chat({ id, client, api, status, children }: { id: string; client: StreamClient; api: Authed; status: StreamStatus; children?: ReactNode }) {
  const [t, setT] = useState<Transcript>(emptyTranscript);
  const [loadError, setLoadError] = useState("");
  const [older, setOlder] = useState<"idle" | "loading" | "failed">("idle");
  const [watchError, setWatchError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const logRef = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const anchor = useRef<{ height: number; top: number } | null>(null);
  const loadingOlder = useRef(false);
  const latest = useRef(t);
  latest.current = t;
  const { fetch, token } = api;

  useEffect(() => {
    let alive = true;
    messagesPage({ fetch, token }, id).then(
      (page) => {
        if (alive) {
          setLoadError("");
          setT((cur) => withPage(cur, page));
        }
      },
      (err: unknown) => {
        if (alive) {
          setLoadError(errorText(err));
        }
      },
    );
    return () => {
      alive = false;
    };
  }, [fetch, token, id, attempt]);

  useEffect(
    () =>
      client.onFrame((frame) => {
        if (frame.transcript && frame.transcript.session === id) {
          const tf = frame.transcript;
          setWatchError("");
          setT((cur) => withFrame(cur, tf));
        } else if (frame.error && frame.watch === id) {
          setWatchError(frame.error.message);
        }
      }),
    [client, id],
  );

  useEffect(() => {
    if (status === "live" && t.loaded) {
      client.send({ watch: id, after: newestCursor(latest.current) });
    }
  }, [client, id, status, t.loaded]);

  useEffect(
    () => () => {
      client.send({ unwatch: id });
    },
    [client, id],
  );

  useLayoutEffect(() => {
    const el = logRef.current;
    if (!el) {
      return;
    }
    if (anchor.current) {
      el.scrollTop = anchor.current.top + el.scrollHeight - anchor.current.height;
      anchor.current = null;
    } else if (atBottom.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [t.messages]);

  const loadOlder = useCallback(() => {
    const cur = latest.current;
    if (!cur.loaded || cur.before === 0 || loadingOlder.current) {
      return;
    }
    loadingOlder.current = true;
    setOlder("loading");
    messagesPage({ fetch, token }, id, cur.before ?? undefined).then(
      (page) => {
        loadingOlder.current = false;
        const el = logRef.current;
        if (el) {
          anchor.current = { height: el.scrollHeight, top: el.scrollTop };
        }
        setOlder("idle");
        setT((c) => withPage(c, page));
      },
      () => {
        loadingOlder.current = false;
        setOlder("failed");
      },
    );
  }, [fetch, token, id]);

  const onScroll = () => {
    const el = logRef.current;
    if (!el) {
      return;
    }
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < nearBottom;
    if (el.scrollTop < nearTop) {
      loadOlder();
    }
  };

  return (
    <div className="chat-log" role="log" aria-label="Messages" ref={logRef} onScroll={onScroll}>
      <OlderRow t={t} loading={older === "loading"} failed={older === "failed"} onLoad={loadOlder} />
      {loadError ? (
        <div className="error chat-error">
          <span>{"Couldn’t load messages: " + loadError}</span>
          <button type="button" className="retry" onClick={() => setAttempt((n) => n + 1)}>
            Retry
          </button>
        </div>
      ) : null}
      {t.loaded && t.messages.length === 0 ? <p className="muted chat-empty">No messages yet</p> : null}
      {t.messages.map((m) => (
        <MessageLine key={m.id} message={m} />
      ))}
      {watchError ? <p className="error">{"Live updates stopped: " + watchError}</p> : null}
      <div className="chat-end">{children}</div>
    </div>
  );
}

function Queued({ sends, onEdit, onDrop }: { sends: QueuedSend[]; onEdit: (s: QueuedSend) => void; onDrop: (s: QueuedSend) => void }) {
  if (sends.length === 0) {
    return null;
  }
  return (
    <ul className="queued" aria-label="Queued">
      {sends.map((s) => (
        <li key={s.id}>
          <span className="queued-label">Queued</span>
          <span className="queued-text">{s.text}</span>
          <button type="button" className="queued-action" aria-label={"Edit: " + s.text} onClick={() => onEdit(s)}>
            Edit
          </button>
          <button type="button" className="queued-action" aria-label={"Drop: " + s.text} onClick={() => onDrop(s)}>
            Drop
          </button>
        </li>
      ))}
    </ul>
  );
}

function Composer({ session, api, sends }: { session: Session; api: Authed; sends: QueuedSend[] }) {
  const [draft, setDraft] = useState("");
  const [posting, setPosting] = useState(false);
  const [error, setError] = useState("");
  const blank = draft.trim() === "";

  const submit = async (ev: FormEvent) => {
    ev.preventDefault();
    if (blank || posting) {
      return;
    }
    setPosting(true);
    setError("");
    try {
      await sendMessage(api, session.ID, draft);
      setDraft("");
    } catch (err) {
      setError(errorText(err));
    } finally {
      setPosting(false);
    }
  };

  const takeBack = async (s: QueuedSend, edit: boolean) => {
    setError("");
    try {
      await unsend(api, session.ID, s.id);
      if (edit) {
        setDraft((cur) => (cur.trim() ? cur + "\n\n" + s.text : s.text));
      }
    } catch (err) {
      setError(err instanceof ApiError && err.code === "not_found" ? "That message was already sent." : errorText(err));
    }
  };

  return (
    <footer className="composer">
      <Queued sends={sends} onEdit={(s) => void takeBack(s, true)} onDrop={(s) => void takeBack(s, false)} />
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      <form onSubmit={(ev) => void submit(ev)}>
        <textarea aria-label="Message" rows={1} value={draft} placeholder={busy(session) ? "Queue a message" : "Message"} onChange={(ev) => setDraft(ev.target.value)} />
        <button type="submit" disabled={blank || posting}>
          {busy(session) ? "Queue" : "Send"}
        </button>
      </form>
    </footer>
  );
}

function ConfirmInterrupt({ api, session, onClose }: { api: Authed; session: string; onClose: () => void }) {
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const confirm = async () => {
    setSending(true);
    setError("");
    try {
      await interrupt(api, session);
      onClose();
    } catch (err) {
      setError(errorText(err));
      setSending(false);
    }
  };
  return (
    <div className="dialog-backdrop">
      <div className="dialog" role="alertdialog" aria-modal="true" aria-labelledby="interrupt-title" aria-describedby="interrupt-body">
        <h2 id="interrupt-title">Interrupt this session?</h2>
        <p id="interrupt-body">The agent stops what it is doing, as if you pressed Escape in its terminal.</p>
        {error ? (
          <p className="error" role="alert">
            {error}
          </p>
        ) : null}
        <div className="dialog-actions">
          <button type="button" className="secondary" autoFocus onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="danger" disabled={sending} onClick={() => void confirm()}>
            Interrupt
          </button>
        </div>
      </div>
    </div>
  );
}

export function SessionScreen({
  id,
  snapshot,
  now,
  onRetry,
  client,
  api,
}: {
  id: string;
  snapshot: StreamSnapshot;
  now: number;
  onRetry: () => void;
  client: StreamClient;
  api: Authed;
}) {
  const [confirming, setConfirming] = useState(false);
  const session = snapshot.state?.sessions.find((s) => s.ID === id);
  if (!session) {
    return (
      <main className="screen">
        <Back />
        <Connection snapshot={snapshot} now={now} onRetry={onRetry} />
        {snapshot.state ? (
          <section className="card empty">
            <p>This session is gone.</p>
          </section>
        ) : null}
      </main>
    );
  }
  const sends = (snapshot.state?.sends ?? []).filter((s) => s.session === id);
  return (
    <div className="chat-screen">
      <Header session={session} now={now} onInterrupt={() => setConfirming(true)} />
      <Connection snapshot={snapshot} now={now} onRetry={onRetry} />
      <main className="chat-main">
        <Chat id={id} client={client} api={api} status={snapshot.status} />
      </main>
      <Composer session={session} api={api} sends={sends} />
      {confirming ? <ConfirmInterrupt api={api} session={id} onClose={() => setConfirming(false)} /> : null}
    </div>
  );
}
