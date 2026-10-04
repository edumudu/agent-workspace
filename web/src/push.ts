import { pushKey, pushSubscribe, pushUnsubscribe, type Fetch, type PushSubscriptionBody } from "./api";

export type PushEnv = {
  supported: boolean;
  permission: () => NotificationPermission;
  requestPermission: () => Promise<NotificationPermission>;
  subscribe: (applicationServerKey: Uint8Array) => Promise<PushSubscriptionBody>;
  unsubscribe: () => Promise<void>;
};

export const noPush: PushEnv = {
  supported: false,
  permission: () => "denied",
  requestPermission: async () => "denied",
  subscribe: () => Promise.reject(new Error("this browser has no push")),
  unsubscribe: async () => undefined,
};

export function decodeKey(base64url: string): Uint8Array {
  const base64 = base64url.replace(/-/g, "+").replace(/_/g, "/");
  const padded = base64 + "=".repeat((4 - (base64.length % 4)) % 4);
  const raw = atob(padded);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) {
    out[i] = raw.charCodeAt(i);
  }
  return out;
}

export type PushOutcome = "on" | "denied";

export async function enablePush(push: PushEnv, fetchFn: Fetch, token: string): Promise<PushOutcome> {
  const permission = await push.requestPermission();
  if (permission !== "granted") {
    return "denied";
  }
  const key = await pushKey(fetchFn, token);
  const subscription = await push.subscribe(decodeKey(key.public_key));
  await pushSubscribe(fetchFn, token, subscription);
  return "on";
}

export async function disablePush(push: PushEnv, fetchFn: Fetch, token: string): Promise<void> {
  await Promise.allSettled([pushUnsubscribe(fetchFn, token), push.unsubscribe()]);
}

function sameKey(a: ArrayBuffer | null | undefined, b: Uint8Array): boolean {
  if (!a) {
    return false;
  }
  const bytes = new Uint8Array(a);
  return bytes.length === b.length && bytes.every((v, i) => v === b[i]);
}

export function browserPush(win: Window & typeof globalThis): PushEnv {
  const supported = "Notification" in win && "serviceWorker" in win.navigator && "PushManager" in win;
  if (!supported) {
    return noPush;
  }
  return {
    supported,
    permission: () => win.Notification.permission,
    requestPermission: () => win.Notification.requestPermission(),
    subscribe: async (key) => {
      const registration = await win.navigator.serviceWorker.ready;
      const existing = await registration.pushManager.getSubscription();
      if (existing && sameKey(existing.options.applicationServerKey, key)) {
        return existing.toJSON();
      }
      await existing?.unsubscribe();
      const created = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: key as Uint8Array<ArrayBuffer>,
      });
      return created.toJSON();
    },
    unsubscribe: async () => {
      const registration = await win.navigator.serviceWorker.getRegistration();
      const existing = await registration?.pushManager.getSubscription();
      await existing?.unsubscribe();
    },
  };
}
