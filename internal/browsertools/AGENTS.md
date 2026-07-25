# internal/browsertools — Browser-Tool Relay

The wire between an agent harness and a user's browser extension. The harness
issues tool calls (`read_form`, `fill_simple`, …); the extension executes them
against whatever page the user is on and sends results back.

## Architecture

- `Hub` holds one channel per user, each with up to two ends (`RoleHarness`,
  `RoleExtension`). `Join` attaches an end and returns its `leave`; `Forward`
  hands a frame to the *other* end of the sender's own channel.
- **Owner-scoped, always.** A channel is keyed by the user id the authenticating
  middleware resolved — never by anything the client claims — so a harness can
  only ever reach its own user's browser.
- **Opaque frames.** The relay parses nothing but the `id` (and only to build an
  error answer): `{id,tool,args}` / `{id,result}` pass through verbatim. Adding a
  primitive is a change in the extension and the harness, not here.
- **Never hang a caller.** A call with no extension attached is answered with
  `{id, error}` rather than dropped — the harness is blocked on that id. A result
  with no harness left is dropped (nobody is waiting).
- **Last connection wins.** Re-joining in a role replaces the previous socket; the
  displaced connection's `leave` is a no-op, so it cannot evict its successor.

## Transport

`internal/handler/browsertools.go` upgrades `GET /api/v1/tools/ws?role=…` behind
`auth.RequireAuthWS`. The extension authenticates through the WebSocket
subprotocol (`freehire-jwt, <token>`) because a browser can set no headers on a
`new WebSocket`; a server-side harness uses `Authorization: Bearer`. Only the
marker is echoed back, never the token.

The hub is in-memory and per-instance: both ends of a channel must be connected
to the same process. Fine while the API is single-node; a multi-node deployment
needs a shared backplane (the seam is `Hub`).
