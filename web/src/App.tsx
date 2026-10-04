import { useEffect, useMemo, useState } from "react";
import type { Fetch } from "./api";
import { clearAuth, loadAuth, saveAuth, type Auth, type KeyValue } from "./auth";
import { pairCodeFromHash } from "./display";
import { parseRoute } from "./route";
import { Install } from "./screens/Install";
import { Pair } from "./screens/Pair";
import { SessionScreen } from "./screens/Session";
import { SessionList } from "./screens/SessionList";
import { useStream, useStreamClient, type OpenSocket } from "./stream";

export type AppEnv = {
  fetch: Fetch;
  storage: KeyValue;
  installed: boolean;
  hash: string;
  host: string;
  userAgent: string;
  openStream: OpenSocket;
  retryDelay?: (attempt: number) => number;
  now: () => number;
  onHashChange: (listener: (hash: string) => void) => () => void;
};

function useNow(now: () => number, every: number): number {
  const [value, setValue] = useState(now);
  useEffect(() => {
    setValue(now());
    const timer = setInterval(() => setValue(now()), every);
    return () => clearInterval(timer);
  }, [now, every]);
  return value;
}

function Connected({ env, auth, onUnauthorized }: { env: AppEnv; auth: Auth; onUnauthorized: () => void }) {
  const client = useStreamClient({ open: env.openStream, token: auth.token, retryDelay: env.retryDelay, now: env.now });
  const snapshot = useStream(client);
  const [hash, setHash] = useState(env.hash);
  const now = useNow(env.now, snapshot.status === "offline" ? 1000 : 30000);
  const { onHashChange, fetch } = env;
  const api = useMemo(() => ({ fetch, token: auth.token }), [fetch, auth.token]);

  useEffect(() => onHashChange(setHash), [onHashChange]);

  useEffect(() => {
    if (snapshot.status === "unauthorized") {
      onUnauthorized();
    }
  }, [snapshot.status, onUnauthorized]);

  const route = parseRoute(hash);
  const retry = () => client.retryNow();
  if (route.screen === "session") {
    return <SessionScreen key={route.id} id={route.id} snapshot={snapshot} now={now} onRetry={retry} client={client} api={api} />;
  }
  return <SessionList host={env.host} snapshot={snapshot} now={now} onRetry={retry} />;
}

export function App({ env }: { env: AppEnv }) {
  const [auth, setAuth] = useState<Auth | null>(() => loadAuth(env.storage));
  const code = useMemo(() => pairCodeFromHash(env.hash), [env.hash]);
  const { storage } = env;
  const unpair = useMemo(
    () => () => {
      clearAuth(storage);
      setAuth(null);
    },
    [storage],
  );
  if (auth) {
    return <Connected env={env} auth={auth} onUnauthorized={unpair} />;
  }
  if (!env.installed) {
    return <Install code={code} host={env.host} />;
  }
  return <Pair env={env} code={code} onPaired={(paired) => setAuth(saveAuth(env.storage, paired))} />;
}
