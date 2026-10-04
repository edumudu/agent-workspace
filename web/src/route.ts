export type Route = { screen: "list" } | { screen: "new" } | { screen: "session"; id: string };

export function parseRoute(hash: string): Route {
  if (hash === "#/new") {
    return { screen: "new" };
  }
  const m = hash.match(/^#\/sessions\/([^/?#]+)$/);
  if (!m) {
    return { screen: "list" };
  }
  try {
    return { screen: "session", id: decodeURIComponent(m[1]) };
  } catch {
    return { screen: "list" };
  }
}
