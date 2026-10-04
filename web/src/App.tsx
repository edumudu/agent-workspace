import { useEffect, useMemo, useState } from "react";
import type { Fetch } from "./api";
import { clearAuth, loadAuth, saveAuth, type Auth, type KeyValue } from "./auth";
import { pairCodeFromHash } from "./display";
import { disablePush, noPush, type PushEnv } from "./push";
import { Install } from "./screens/Install";
import { Pair } from "./screens/Pair";
import { Sessions } from "./screens/Sessions";
import { Settings } from "./screens/Settings";

export type AppEnv = {
  fetch: Fetch;
  storage: KeyValue;
  installed: boolean;
  hash: string;
  host: string;
  userAgent: string;
  push?: PushEnv;
};

function useHash(initial: string): string {
  const [hash, setHash] = useState(initial);
  useEffect(() => {
    const changed = () => setHash(window.location.hash);
    window.addEventListener("hashchange", changed);
    return () => window.removeEventListener("hashchange", changed);
  }, []);
  return hash;
}

export function App({ env }: { env: AppEnv }) {
  const [auth, setAuth] = useState<Auth | null>(() => loadAuth(env.storage));
  const hash = useHash(env.hash);
  const code = useMemo(() => pairCodeFromHash(hash), [hash]);
  if (auth) {
    const settings = hash === "#/settings";
    return (
      <>
        {settings ? (
          <Settings
            auth={auth}
            host={env.host}
            installed={env.installed}
            fetch={env.fetch}
            push={env.push ?? noPush}
            onSignOut={async () => {
              await disablePush(env.push ?? noPush, env.fetch, auth.token);
              clearAuth(env.storage);
              setAuth(null);
            }}
          />
        ) : (
          <Sessions auth={auth} host={env.host} />
        )}
        <nav className="tabs" aria-label="Screens">
          <a href="#/" aria-current={settings ? undefined : "page"}>
            Sessions
          </a>
          <a href="#/settings" aria-current={settings ? "page" : undefined}>
            Settings
          </a>
        </nav>
      </>
    );
  }
  if (!env.installed) {
    return <Install code={code} host={env.host} />;
  }
  return <Pair env={env} code={code} onPaired={(paired) => setAuth(saveAuth(env.storage, paired))} />;
}
