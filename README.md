# gRPC Todo Service

[![CI](https://github.com/Edgargmc/golang/actions/workflows/ci.yml/badge.svg)](https://github.com/Edgargmc/golang/actions/workflows/ci.yml)

A Todo service built to learn gRPC in depth — not a toy example. It covers unary and streaming RPCs, interceptors, authentication, structured errors, deadlines/cancellation, persistence, and automated testing, all designed with SOLID principles (interfaces over concrete types, single-responsibility components) so each piece can be swapped independently.

## Features

- **Full CRUD** — Create, Get, List, Update, Delete, Complete
- **Input validation** — empty titles rejected with `InvalidArgument`
- **Auth** — a bearer token required on every RPC (unary and streaming), enforced via interceptors
- **Structured errors** — proper gRPC status codes (`NotFound`, `InvalidArgument`, `Unauthenticated`, `DeadlineExceeded`) instead of generic failures
- **Real-time events** — `WatchTodos` streams `created`/`updated`/`completed`/`deleted` events to any number of connected clients, via an internal pub/sub broadcaster
- **Deadlines & cancellation** — RPCs (including the long-lived `WatchTodos` stream) respect client-set deadlines and clean up their own resources when a client disconnects
- **Persistence** — SQLite-backed storage in production, in-memory storage in tests, behind the same `TodoRepository` interface
- **Automated tests** — a `bufconn`-based suite exercises the real server (interceptors included) with no network sockets involved; run with the race detector in CI
- **CI** — every push/PR runs `go build`, `go vet`, and `go test -race` via GitHub Actions

## Project Structure

```
grpc-todo/
├── proto/todo.proto           # Service contract (source of truth)
├── gen/                       # Generated Go code from the .proto (do not edit)
├── server/
│   ├── main.go                 # todoServer: RPC handlers, interceptors, wiring
│   ├── broadcaster.go           # EventPublisher/EventSubscriber pub/sub for WatchTodos
│   ├── repository.go            # TodoRepository interface + in-memory implementation
│   ├── repository_sqlite.go     # SQLite implementation of TodoRepository
│   ├── main_test.go              # bufconn integration tests (CRUD, auth, deadlines, streaming)
│   └── repository_sqlite_test.go # Isolated repository test (:memory: sqlite)
├── client/main.go              # Example Go client exercising every RPC
├── .github/workflows/ci.yml    # Build + vet + test on every push
├── build.sh                    # Regenerates gen/ from proto/todo.proto
└── Makefile                    # run-server / run-client / build shortcuts
```

## Architecture notes

- **`todoServer` depends on interfaces, not implementations.** It talks to `TodoRepository` (storage) and `EventPublisher`/`EventSubscriber` (pub/sub) — never to a concrete map or a concrete broadcaster. This is what let SQLite get swapped in for the original in-memory map without touching a single RPC handler or existing test.
- **Interceptors are layered, not scattered.** `loggingInterceptor` wraps `authInterceptor` for every unary RPC (`grpc.ChainUnaryInterceptor`); `streamAuthInterceptor` covers streaming RPCs separately, since unary and stream interceptors are independent chains in gRPC-Go. Reflection is explicitly excluded from auth so tools like `grpcurl`/Postman can still discover the API.
- **`WatchTodos` never blocks a mutation.** The broadcaster publishes to subscriber channels with a non-blocking `select`/`default` — a slow watcher drops events instead of stalling `CreateTodo`/`UpdateTodo`/etc. for everyone.
- **Tests run in-process.** `server/main_test.go` spins up the real `todoServer` (real interceptors, real broadcaster) over `google.golang.org/grpc/test/bufconn` — no TCP port, no flakiness from a busy `:50051`.

## Getting Started

### Prerequisites

- Go 1.25+ (see `go.mod`)
- `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` — only needed if you change `proto/todo.proto` and want to regenerate `gen/`
- [`grpcurl`](https://github.com/fullstorydev/grpcurl) — handy for manual testing (`brew install grpcurl`)

### Run it

```bash
# Terminal 1
cd server
go run .          # creates/opens todos.db in the current directory

# Terminal 2
grpcurl -plaintext -H 'authorization: secret-token-123' \
  -d '{"title":"Buy milk"}' localhost:50051 todo.TodoService/CreateTodo
```

All RPCs except server reflection require the `authorization` metadata header shown above (a hardcoded token for learning purposes — see `authToken` in `server/main.go`).

### Regenerate code from the `.proto`

```bash
./build.sh
```

### Run the test suite

```bash
go test ./server/... -race -v
```

## API

Defined in `proto/todo.proto`:

| RPC | Type | Description |
|---|---|---|
| `CreateTodo` | unary | Create a todo |
| `GetTodo` | unary | Fetch a todo by id |
| `ListTodos` | unary | Paginated list |
| `UpdateTodo` | unary | Replace title/description/completed |
| `DeleteTodo` | unary | Delete by id |
| `CompleteTodo` | unary | Mark as completed |
| `WatchTodos` | **server streaming** | Live feed of every change, across all clients |

## Related

A Python client (linear demo, interactive CLI, and a `WatchTodos` watcher) lives in a sibling project and talks to this same server, generated straight from this repo's `proto/todo.proto` — proof that the contract is language-agnostic.
