# internal/serve

`agentws serve`: the HTTP and WebSocket API the phone app (PWA) talks to. It is a separate process that reaches the daemon only through `rpc`, like the TUI, and exposes an allowlist of it, never a passthrough. See [ADR 0046](../../docs/adr/0046-remote-app.md); pairing and revocation on the daemon side are in [internal/daemon/](../daemon/AGENTS.md).

Tests: `go test ./internal/serve/ ./cmd/agentws/ -run Serve` (an in-memory daemon fake in `fakes_test.go`, `httptest` and a real WebSocket client) and `go test -tags integration ./test/integration/ -run ServeAgainst` (a real daemon with SQLite). Golden responses are in `testdata/`; regenerate with `go test ./internal/serve -update` and review the diff, since a changed golden is a changed API.

## Layers

- depguard: `internal/serve` imports only `rpc` and `domain` from `internal/`, plus the standard library and `github.com/coder/websocket`. No `os/exec`: `cmd/agentws/serve.go` parses the flags, reads `[serve] url`, and passes a `Dial` that starts the daemon the way the TUI does (`rpc.Connect` with `spawn`).
- `serve.Daemon` is the slice of `*rpc.Client` serve uses (`Call`, `Subscribe`, `Close`). Every HTTP request dials its own connection and closes it; every stream holds its own connection, so a slow phone never stalls another client's reader.

## Listening

- `serve.Options{Addr, Cert, Key, SelfSigned, CertDir}`; `Check` is the rule: plain HTTP only on a loopback address (`localhost`, `127.0.0.0/8`, `::1`), whatever the other flags. `--cert` and `--key` go together; `--self-signed` replaces them.
- `--self-signed` keeps `cert.pem` and `key.pem` (600) in `$AGENTWS_HOME/serve`, valid 5 years, for `localhost`, `127.0.0.1`, `::1`, `host.docker.internal`, the hostname and the `--addr` host. It is reused while it has 30 days left and covers the address, so a proxy that trusts it keeps working across restarts. When serve replaces it (a new `--addr` host, or near expiry), the proxy must be set to trust the new one.
- `Server.Start(ctx)` opens the revocation `subscribe` connection; call it before serving. It fails when the daemon cannot be reached, and resubscribes every second after the daemon restarts. Cancelling `ctx` closes every open stream with 1001.

## HTTP API (`/api/v1`)

Every response is JSON with `Cache-Control: no-store`. An error is `{"error":{"code","message"}}` with the daemon's `rpc` code and an HTTP status: `bad_request` 400, `unauthorized` 401, `forbidden` 403, `not_found` 404, `stale` 409, `rate_limited` 429, `failed`/`launch_failed` 500, `unknown_method` 501, `version_mismatch` 502, `unavailable` 503 (also when the daemon cannot be reached). Unknown paths under `/api/` answer `not_found`.

| Endpoint | Auth | Daemon method | Body → response |
|---|---|---|---|
| `GET /api/v1/hello` | no | none | → `{"api":"v1","build"}` (serve's build, which is the daemon's) |
| `POST /api/v1/pair` | no | `pair.redeem` | `{"code","name"}` → `{"device":{"id","name","created_at","last_seen"},"token"}` |
| `GET /api/v1/workspaces` | yes | `workspace.list` | → `{"workspaces":[Workspace],"last_used"}` |
| `POST /api/v1/sessions/{id}/end` | yes | `session.end` | → the ended `Session` |
| `POST /api/v1/sessions/{id}/resume` | yes | `session.resume` | → the resumed `Session` |
| `POST /api/v1/sessions/{id}/mute` | yes | `session.mute` | `{"muted":bool}` (required) → `{}` |
| `POST /api/v1/sessions/{id}/rename` | yes | `session.rename` | `{"name"}` → `{}` |

- Auth is `Authorization: Bearer <token>`, checked with `device.check` on the request's own connection before the method runs, and then against the set of revoked device IDs.
- `pair` passes the caller's address as `addr`: `RemoteAddr`, or the last `X-Forwarded-For` entry (else `X-Real-IP`) when the peer is a loopback or private address, which is where a proxy in front of serve sits. A forged header from a direct client is ignored.
- Domain structs (`Workspace`, `Session`, ...) encode with their Go field names, as in the socket protocol; the goldens pin them.
- Bodies are capped at 64 KiB.

### Adding an endpoint

1. Write the failing test in `server_test.go`: the request, the daemon method and params the fake saw, and a golden for the response.
2. Add one line to `Server.routes()`: a Go 1.22 pattern (`"POST /api/v1/sessions/{id}/send"`) and a `func(r *http.Request, d Daemon) (any, error)`. Routes there are authenticated; return a value to encode or an error (an `*rpc.Error` keeps its code). `sessionAction(method, params)` covers "decode a body, add the session id, call one method, pass the result through".
3. Add the method to `allowed` in `server_test.go` and the endpoint to `authedEndpoints`, so the token and allowlist tests cover it, and add a row to the table above.

The `/` fallback: `Handler(fallback)` mounts `fallback` at `/` (the embedded PWA); `Handler(nil)` serves the API only.

## Stream (`GET /api/v1/stream`, WebSocket)

- The `Origin` header must equal the public URL's origin (`--url`, else `[serve] url`; a URL without a scheme means `https`; default ports are ignored). Another or a missing `Origin`, or no public URL, is refused with 403 before the upgrade.
- The first client frame must be `{"token":"<device token>"}` within 5 s. A wrong, missing or late token closes the socket with code 4401.
- Then the server sends `{"state":{"seq","workspaces","tasks","worktrees","sessions","queue","sends"}}` and one `{"diff":{...}}` per change that touches those: a diff sets `seq` and one of `workspace`, `task`, `worktree`, `session`, `queue` (whole list), `sends` (whole list), `removed_workspace`, `removed_worktree`, `removed_session`. Events, subagents, review drafts and comments are dropped, so `seq` has gaps. Worktrees come without `Ports`.
- An unknown client frame gets `{"error":{"code":"bad_request","message"}}` and the stream stays open.
- Close codes: 4401 unauthorized or revoked, 1013 the daemon went away (reconnect), 1001 serve is stopping, 1011 a write failed.
- Revocation: a stream subscribes before it runs `device.check` on the same connection, so the daemon's loop orders them: a revoke before the check fails it, a revoke after reaches the stream's own subscription as a `revoked_device` diff. Either that diff or the one on the `Start` connection cancels every open stream of that device (4401 at once) and records the ID, so REST checks that answered just before it still fail. The stream does not depend on the `Start` connection, which may be reconnecting after a daemon restart.
