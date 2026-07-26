# Project Summary — grpc-todo (Go server)

This document exists so a human or an AI assistant picking up this project
cold can understand **why** it's built the way it is, not just what the code
does (the code already says that). It's the Go/server half of a 3-repo
system; see the sibling summaries in `grpc-todo-client` (Python) and
`grpc-todo-android` (Android).

## What this actually is

A Todo REST-CRUD app would take an afternoon. This one took much longer on
purpose: it exists to learn gRPC in depth, feature by feature, with a real
deployed system at the end — not a toy. Every gRPC concept below was added
because it was the next thing to learn, then kept because it turned out
useful.

## gRPC concepts covered, and where

| Concept | Where | Why it's there |
|---|---|---|
| Unary RPC | `CreateTodo`, `GetTodo`, `ListTodos`, `UpdateTodo`, `DeleteTodo`, `CompleteTodo` | The baseline — request in, response out. |
| Server streaming | `WatchTodos` (`server/main.go`) | Server pushes every change to any number of connected clients via an internal pub/sub broadcaster (`server/broadcaster.go`). |
| Bidirectional streaming | `SyncTodos` (`server/main.go`) | The client can keep sending changes *and* receiving events on the same connection, at the same time. Built specifically to complete the "4 RPC types" learning arc after unary + server-streaming. |
| Unary interceptors | `loggingInterceptor`, `authInterceptor` | Chained via `grpc.ChainUnaryInterceptor` — logging wraps auth. |
| Stream interceptors | `streamAuthInterceptor` | **Separate chain from unary** — this is a real gRPC-Go gotcha: unary and streaming interceptors don't share a pipeline. Reflection and health-check are explicitly excluded so `grpcurl`/Postman can still introspect the API without a token. |
| Deadlines & cancellation | Tested in `server/main_test.go` | RPCs (including the long-lived `WatchTodos`/`SyncTodos` streams) respect client deadlines and clean up (unsubscribe from the broadcaster) when the client disconnects. |
| Health checking | `grpc.health.v1.Health`, wired in `main()` | Standard, unauthenticated — used by Docker's `HEALTHCHECK` and could be used by any orchestrator. |
| Reflection | `reflection.Register(s)` | Lets `grpcurl`/Postman discover the API without needing this repo's `.proto`. |
| Structured errors | `codes.InvalidArgument`, `codes.NotFound`, `codes.Unauthenticated`, `codes.DeadlineExceeded` | Proper status codes instead of generic failures. |

## SyncTodos: the interesting part

`SyncTodos` (bidirectional) is the most conceptually involved piece in the
whole system. Two things make it non-obvious:

**1. Only one goroutine may call `stream.Send`.** gRPC-Go forbids concurrent
sends on the same stream. The handler has exactly one goroutine responsible
for all outgoing traffic — it's subscribed to the broadcaster (same
mechanism `WatchTodos` uses) and also drains a small `errs` channel for
per-client validation errors (see below). The `Recv` loop (reading incoming
`TodoChange` messages) never calls `Send` directly.

**2. It reuses the unary handlers, not a copy of their logic.** When a
`SyncTodos` client sends a `TodoChange{create: ...}`, the handler literally
calls `s.CreateTodo(ctx, op.Create)` — the exact same method the unary RPC
uses. Same validation, same repository call, same broadcaster publish. This
was a deliberate choice to avoid duplicating business logic across RPC
shapes.

**3. Per-client errors don't use the broadcaster.** If a client sends an
invalid change (e.g. empty title), the resulting error would be meaningless
to every *other* connected client if broadcast. Instead, `TodoEvent` has an
`error` field, and a rejected change is sent as `type: "error"` directly to
the originating client only, via the same single-writer goroutine (through
the `errs` channel, not the broadcaster channel).

**4. `client_id` for offline reconciliation.** `TodoChange` and `TodoEvent`
both carry an optional `client_id` string. When a client creates something
before it has a real server-assigned ID (typically: created while offline),
it attaches a locally-generated `client_id`. The server doesn't store or
interpret this — it just echoes it back on the resulting event via
`context.WithValue` (see `withClientID`/`clientIDFromContext`), letting the
*same* client recognize "this is the real version of the thing I created
optimistically" and swap its temporary local copy, without needing a
correlation table server-side. Other clients receiving the same broadcast
event simply see a `client_id` that isn't theirs and ignore it.

## Architecture: interfaces over concrete types

`todoServer` depends on `TodoRepository` and `EventPublisher`/
`EventSubscriber` — never on a concrete map or a concrete broadcaster. This
is what let SQLite (and later Turso) get swapped in for the original
in-memory map without touching a single RPC handler or existing test. It's
not exhaustive SOLID for its own sake — it's applied exactly where a real
swap happened (storage backend), not speculatively everywhere.

The broadcaster (`server/broadcaster.go`) publishes to subscriber channels
with a non-blocking `select`/`default`: a slow watcher drops events instead
of stalling every other RPC. There's no replay/history — a client that
wasn't subscribed when something happened will never see that event. This
is why the Android app does a full `ListTodos` refresh on reconnect (see the
Android summary) instead of relying on the stream alone to catch up.

## Auth: deliberately the simplest thing that works

A single bearer token (`AUTH_TOKEN` env var), checked in `authInterceptor`/
`streamAuthInterceptor`. No OAuth, no JWT, no per-user accounts — this is a
personal, single-user project, and that complexity was explicitly rejected
as not worth it. The token used to be hardcoded in source (`secret-token-123`)
until it was found publicly visible in the GitHub repo; it was rotated and
moved to an env var everywhere (Go, Python, Android's `BuildConfig` sourced
from gitignored `local.properties`), with a `local-dev-token-not-for-production`
fallback so local development never needs a real secret.

## Persistence

Same `TodoRepository` interface, three implementations: in-memory (tests),
SQLite via `modernc.org/sqlite` (pure Go, no CGO — local dev), and Turso
(libSQL, wire-compatible with SQLite) in production. Selected at startup in
`main()` based on whether `TURSO_DATABASE_URL`/`TURSO_AUTH_TOKEN` are set.

## Testing

`server/main_test.go` uses `google.golang.org/grpc/test/bufconn` — the real
server, real interceptors, real broadcaster, but no actual TCP socket. This
means tests never flake on a busy port and run fast. Notable tests:
deadline/cancellation behavior, listener leak detection after a client
disconnects (`b.listenerCount()`), and the `SyncTodos` client_id round-trip.

## Docker & CI/CD

Multi-stage Dockerfile → ~45MB static binary, non-root user, custom Go
`healthcheck` binary (calls `grpc.health.v1.Health` — `curl` can't speak
gRPC). GitHub Actions has three jobs: `test`, `docker` (build + smoke test),
`deploy` (only on push to `main`, needs `GCP_SA_KEY`/`TURSO_*`/`AUTH_TOKEN`
secrets).

## Cloud Run deployment: the gotcha worth remembering

`gcloud run deploy --use-http2 ...` is required for gRPC to work at all on
Cloud Run. The default Cloud Run request timeout is **300 seconds** — for a
long-lived stream like `WatchTodos`/`SyncTodos`, this means the connection
gets forcibly killed by the platform after 5 minutes, surfacing to clients
as `StatusCode.UNAVAILABLE` / `INTERNAL` with `RST_STREAM`. This is expected
platform behavior, not a bug in this server. It was never "fixed" (would
need `--timeout` raised, up to Cloud Run's max of 3600s, and even that isn't
truly unlimited) — instead, every client is expected to retry on
disconnect, which both `watch.py` and the Android app do (see their
summaries).

## Known tradeoffs, left as-is on purpose

- No replay buffer on the broadcaster — a disconnected client misses
  everything until it does a full `ListTodos` refresh.
- No mTLS, no per-user auth — one shared token, by design (see Auth above).
- `client_id` reconciliation only covers `create` (the only operation
  without a pre-existing real ID); `update`/`delete` always reference a
  known ID so there's no ambiguity to resolve.
