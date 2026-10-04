export {};

declare const self: ServiceWorkerGlobalScope;

const shellFiles = "__AGENTWS_SHELL_FILES__" as unknown as string[];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open("agentws-shell").then((cache) => cache.addAll(["/", ...shellFiles])));
});
