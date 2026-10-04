# Go API Test Kit

A small, runnable Go REST API with the tests people often leave until later:
HTTP validation, bearer authentication, resource ownership, PostgreSQL contracts,
parallel fixture isolation, race detection, and CI coverage artifacts.

**This is a learning and starter project, not production authentication.**
The bearer tokens are intentionally public. Do not deploy it on the internet or
put real data into it.

## Quick start

Requirements: Go 1.26+, a C compiler for `-race`, and Make. PostgreSQL examples
also require Docker with Compose v2 (`--wait` support).

```sh
go mod download
make test
make run
```

The API listens on `127.0.0.1:8080`, with a fresh in-memory store on every start.
In another terminal:

```sh
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/users/me -H 'Authorization: Bearer demo-alice'
curl -i http://localhost:8080/tasks \
  -H 'Authorization: Bearer demo-alice' \
  -H 'Content-Type: application/json' \
  -d '{"title":"Write an ownership test"}'
curl -s http://localhost:8080/tasks -H 'Authorization: Bearer demo-alice'
# Bob cannot see Alice's task; expect 404.
curl -i http://localhost:8080/tasks/1 -H 'Authorization: Bearer demo-bob'
```

## Real PostgreSQL tests

```sh
make db-up
make test-integration
make coverage
# Optional: open an HTML coverage report.
go tool cover -html=coverage.out -o coverage.html
make db-down
```

Compose starts a disposable PostgreSQL 17 database in memory, bound only to
localhost port 55432. Stopping/removing the container loses its data. Its public
`testkit` credentials are only for this local development database.

Every integration fixture creates a unique schema, applies the embedded
migration, and opens a pool whose search path targets that schema. Cleanup drops
only the schema it created. No `TRUNCATE public`, shared tables, or database-wide
reset. Fixtures are independent and can run in parallel. Use a **dedicated test
database**, never a production connection: tests intentionally create and drop
schemas. The database role needs schema creation privileges.

To use an existing disposable PostgreSQL instance:

```sh
TEST_DATABASE_URL='postgres://user:password@localhost/testdb?sslmode=disable' make test-integration
```

Plain `go test ./...` runs fast tests without PostgreSQL. The `integration` build
tag opts into real database tests; missing `TEST_DATABASE_URL` fails those tests
rather than silently reporting a skip. `make test-integration` supplies the local
Compose URL by default and fails if PostgreSQL is unavailable.

## Run the API with PostgreSQL

```sh
make db-up
make db-init  # apply once to this empty local database
DATABASE_URL='postgres://testkit:testkit@localhost:55432/testkit?sslmode=disable' make run
```

The server checks connectivity but does not auto-migrate databases. `db-init`
only targets the project's Compose database; reapplying this initial migration
fails instead of silently resetting data. Stop and recreate the disposable
container if you need a fresh local database.

## API contract

All routes except `/healthz` require `Authorization: Bearer <token>`.
`demo-alice` is user 1; `demo-bob` is user 2. There is no login or registration.

| Method | Route | Success | Purpose |
| --- | --- | --- | --- |
| GET | `/healthz` | 200 | Process liveness, not database readiness |
| GET | `/users/me` | 200 | Current demo identity |
| GET | `/tasks` | 200 | Own tasks, ascending ID; empty array when none |
| POST | `/tasks` | 201 | Create `{"title":"…"}`; returns Location header |
| GET | `/tasks/{id}` | 200 | Read own task |
| PUT | `/tasks/{id}` | 200 | Replace `{"title":"…","done":true}` |
| DELETE | `/tasks/{id}` | 204 | Delete own task |

PUT is full replacement: omitted `done` becomes false. Titles are trimmed and
must contain 1–200 Unicode characters. Write routes require JSON, reject unknown
fields and trailing JSON, and limit bodies to 4 KiB. IDs must be positive int64s.
Errors are JSON (`{"error":"…"}`), except the standard router's 404/405 responses.
Missing/invalid auth returns 401; invalid input 400; wrong content type 415.
Both missing and someone else's task return 404 to avoid exposing existence.
Database queries enforce ownership as part of the query, not a separate check.

## What's tested

- Fresh per-test fixtures and deterministic Alice/Bob identities
- HTTP authentication, successful CRUD, input boundaries and ownership denial
- Shared store contract for in-memory and real PostgreSQL implementations
- Cross-user list/read/update/delete isolation
- Concurrent-safe in-memory operations (`go test -race`)
- GitHub Actions: formatting, vet, race-enabled integration tests, coverage upload

`make ci` runs the same aggregate checks locally; it requires PostgreSQL.
Coverage is reported and uploaded, with no arbitrary percentage gate. Coverage
is evidence of exercised code, not a security guarantee.

## Project map

```text
cmd/api/             server startup and graceful shutdown
internal/api/        HTTP handlers + httptest examples
internal/store/      store interface, memory + pgx implementations, contracts
migrations/          embedded initial schema and demo identities
.github/workflows/   PostgreSQL-backed CI
compose.yaml         disposable local PostgreSQL
```

To adapt the kit: rename the module and imports, replace demo auth with verified
identity, add domain rules and migrations, and extend the shared store contract.
Before production add proper secret management, HTTPS, authorization policy,
rate limits, monitoring, readiness checks, migration tooling, pagination and
operational hardening. The current demo is intentionally unpaginated.

## Быстрый старт

```sh
make test              # быстрые HTTP/unit-тесты и race detector
make db-up             # временный PostgreSQL
make test-integration  # реальные SQL-запросы, отдельная схема для каждого теста
make run               # API в памяти, localhost:8080
make db-down           # удалить временную БД
```

Токены `demo-alice` и `demo-bob` публичные и только для примера.
Не используйте реальные данные или production-БД. Для интеграционных тестов
нужны Go 1.26+, C-компилятор, Make и Docker Compose v2.
