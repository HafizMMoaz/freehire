# internal/observability — Sentry Error Tracking

Opt-in Sentry across all three surfaces, env-gated.

## Backend Server (`cmd/server`)

- `observability.Init(dsn, environment)` wraps `sentry.Init` with defaults (`SendDefaultPII:false`, tracing off — **errors-only**), returns a `flush`.
- Empty DSN = **no-op** (app runs unchanged). Malformed DSN = **fatal** (fail-fast).
- `sentryfiber` middleware registered **after** `recover.New` so deferred capture reports panic *with a stack* before `recover.New` renders standard 500 (`Repanic:true`).
- `handler.RenderError` reports **only** fall-through unexpected 500 to request hub — routine 4xx / `pgx.ErrNoRows`→404 / FK-violation→404 are never reported. Recovered panic is **not** double-reported (recover middleware marks it via `handler.LocalPanicReported`).
- **Streamed responses need their own reporting.** `RenderError` only ever sees errors a handler *returns*, and an SSE handler returns `nil` before its body writer runs — so a failure inside the stream reports nothing, while the access log records the `200` the stream opened with. `handler.reportStreamFault` closes that gap for the fit stream: it takes a **clone** of the request hub captured before the ctx is released, and defers to the same `classify` policy so a reader who walked away is not filed as a fault. Any new streaming endpoint has this blind spot until it does the same.

## Workers

- `observability.Init` lives in `worker.Bootstrap` (flush folded into `cleanup`).
- Every cron worker's `main` uses `worker.Main(run)` — deferred `capturePanic` captures + flushes + re-panics so short-lived run-once process still delivers fatal panic before crashing non-zero.
- `harvest-*`/`gen-contracts` **dev tools are out of scope** (no Bootstrap).

## Frontend (`web/`)

- `@sentry/sveltekit` in `hooks.client.ts`/`hooks.server.ts`, gated on `PUBLIC_SENTRY_DSN` (+ `PUBLIC_SENTRY_ENVIRONMENT`).
- `sentrySvelteKit()` Vite plugin uploads source maps only when `SENTRY_AUTH_TOKEN`/`SENTRY_ORG`/`SENTRY_PROJECT` are set (build succeeds without them).
- No CSP change needed — no `default-src`/`connect-src`, browser delivery to ingest host is unrestricted.

## Config

`SENTRY_DSN`/`SENTRY_ENVIRONMENT` (backend + workers) and `PUBLIC_SENTRY_DSN`/`PUBLIC_SENTRY_ENVIRONMENT` (frontend), all optional, injected by `freehire-ops` (never committed). Two Sentry projects (frontend + backend); `SENTRY_ENVIRONMENT` tags events for shared project filtering.
