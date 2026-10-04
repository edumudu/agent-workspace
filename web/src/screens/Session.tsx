import { sessionMeta, timeInState } from "../sessions";
import type { StreamSnapshot } from "../stream";
import { Connection, sessionTitle } from "./SessionList";

export function SessionScreen({
  id,
  snapshot,
  now,
  onRetry,
}: {
  id: string;
  snapshot: StreamSnapshot;
  now: number;
  onRetry: () => void;
}) {
  const session = snapshot.state?.sessions.find((s) => s.ID === id);
  const age = session ? timeInState(session.since, now) : "";
  return (
    <main className="screen">
      <a className="back" href="#/">
        <span aria-hidden="true">{"‹ "}</span>
        Sessions
      </a>
      <Connection snapshot={snapshot} now={now} onRetry={onRetry} />
      {session ? (
        <>
          <header className="top">
            <h1>{sessionTitle(session)}</h1>
            <p className="meta">{sessionMeta(session) + (age ? " · " + age : "")}</p>
            {session.banner ? <p className="banner">{session.banner}</p> : null}
          </header>
          <section className="card empty">
            <p>The chat view arrives in the next update of the app.</p>
          </section>
        </>
      ) : snapshot.state ? (
        <section className="card empty">
          <p>This session is gone.</p>
        </section>
      ) : null}
    </main>
  );
}
