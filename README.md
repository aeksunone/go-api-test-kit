# Go API Test Kit

[English](#english) | [Русский](#russian)

<a id="english"></a>

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

<a id="russian"></a>

## Русская инструкция

REST API на Go с примерами HTTP-тестов, аутентификации, проверки владельца,
контракта PostgreSQL, изоляции фикстур, поиска гонок и покрытия в CI.

**Это учебный шаблон, а не готовая production-система.** Токены `demo-alice`
и `demo-bob` публичные. Не открывайте приложение в интернете, не храните
реальные данные и не подключайте тесты к production-БД.

### Установка и первый запуск

Нужны Git, Go 1.26+, Make и C-компилятор для детектора гонок `-race`.
Docker с Compose v2 и поддержкой `--wait` нужен только для локального PostgreSQL;
для памяти и быстрых тестов необязателен. Команды рассчитаны на POSIX-оболочку.

```sh
git clone https://github.com/aeksunone/go-api-test-kit.git
cd go-api-test-kit
go mod download
make test
make run
```

Сервер слушает `127.0.0.1:8080`. Без `DATABASE_URL` задачи хранятся в памяти
до перезапуска. Остановка: Ctrl+C. В другом терминале:

```sh
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/users/me -H 'Authorization: Bearer demo-alice'
curl -i http://localhost:8080/tasks \
  -H 'Authorization: Bearer demo-alice' \
  -H 'Content-Type: application/json' \
  -d '{"title":"Проверить доступ к чужой задаче"}'
curl -s http://localhost:8080/tasks -H 'Authorization: Bearer demo-alice'
# На свежем сервере первая созданная задача имеет ID 1.
# Если задачи уже создавались, подставьте ID из ответа POST.
curl -i http://localhost:8080/tasks/1 -H 'Authorization: Bearer demo-bob'
```

Последний запрос возвращает `404`: Bob не видит задачу Alice. Создание возвращает
`201` и `Location`; список Alice содержит задачу.

### Быстрые и интеграционные тесты

`make test` запускает unit/HTTP-тесты с `-race`, без БД. Обычный
`go test ./...` также не требует PostgreSQL, но сам не включает детектор гонок.
Интеграционные тесты подключаются через build tag `integration`:

```sh
make db-up
make test-integration
make coverage
go tool cover -html=coverage.out -o coverage.html
make db-down
```

Compose запускает PostgreSQL 17 с публичными локальными реквизитами
`testkit:testkit`, доступный только через loopback `127.0.0.1:55432`.
Данные в `tmpfs`: остановка/удаление контейнера уничтожает их.
`make db-down` удаляет временную БД.

Каждая тестовая фикстура создаёт уникальную схему, применяет встроенную миграцию
и задаёт `search_path` для всех соединений своего пула. После теста удаляется
только созданная им схема. Общие таблицы не очищаются, поэтому фикстуры могут
работать параллельно. Роли нужно право `CREATE` на базе. `db-init` тестам не нужен.

Для существующей одноразовой тестовой БД:

```sh
TEST_DATABASE_URL='postgres://user:password@localhost/testdb?sslmode=disable' make test-integration
```

`make test-integration` включает быстрые тесты; по умолчанию используется
Compose-адрес. Недоступная БД вызывает ошибку. При запуске
`go test -tags=integration ./...` обязательно передайте `TEST_DATABASE_URL`:
без него тесты завершаются ошибкой, а не пропускаются.

### Сервер с PostgreSQL и настройки

```sh
make db-up
make db-init  # только один раз для пустой локальной БД
DATABASE_URL='postgres://testkit:testkit@localhost:55432/testkit?sslmode=disable' make run
```

Сервер проверяет соединение, но не применяет миграции автоматически.
`db-init` работает только с БД проекта в Compose; повторное применение
миграции завершится ошибкой. Чистый старт: `make db-down`, `make db-up`,
`make db-init` (данные теряются). Для внешней БД примените
`migrations/001_init.sql` самостоятельно.

Переменные окружения:

- `DATABASE_URL`: подключение сервера; без значения используется память.
- `TEST_DATABASE_URL`: подключение интеграционных тестов и покрытия.
- `ADDR`: адрес HTTP-сервера, например `ADDR=127.0.0.1:8081 make run`.
- `GO`: команда Go для Make, например `make test GO=/path/to/go`.

`.env.example` — образец; `.env` автоматически не загружается. Передавайте
значения перед командой либо через `export`.

### Контракт API

Кроме `/healthz`, нужен `Authorization: Bearer <token>`. `demo-alice` — пользователь 1,
`demo-bob` — пользователь 2; регистрации и входа нет.

| Метод и маршрут | Успех | Назначение |
| --- | --- | --- |
| `GET /healthz` | 200 | Жив ли процесс; не проверка готовности БД |
| `GET /users/me` | 200 | Текущий пользователь |
| `GET /tasks` | 200 | Свои задачи по возрастанию ID; иначе `[]` |
| `POST /tasks` | 201 | Создать `{"title":"…"}`, получить `Location` |
| `GET /tasks/{id}` | 200 | Прочитать свою задачу |
| `PUT /tasks/{id}` | 200 | Заменить `{"title":"…","done":true}` |
| `DELETE /tasks/{id}` | 204 | Удалить свою задачу |

PUT заменяет поля: пропущенное `done` становится `false`. POST запрещает
`done:true`. Заголовок после обрезки крайних пробелов: 1–200 Unicode-символов.
POST/PUT требуют JSON, отклоняют неизвестные поля и дополнительный JSON;
предел тела — 4 КиБ. ID — положительный `int64`.

Ошибки: `401` для отсутствующей/неверной авторизации, `400` для неверного ввода,
`415` для неподходящего Content-Type, `404` для отсутствующей или чужой задачи.
Формат — `{"error":"…"}`, кроме стандартных ответов роутера `404/405`.
Принадлежность задачи проверяется непосредственно SQL-запросом.

### Покрытие, CI и структура

GitHub Actions проверяет форматирование, запускает `go vet`, интеграционные
тесты с `-race` и загружает `coverage.out`. Локальный эквивалент — `make ci`;
PostgreSQL должен работать. Порога покрытия нет; покрытие не гарантирует безопасность.

- `cmd/api/`: запуск и корректное завершение сервера.
- `internal/api/`: обработчики и HTTP-тесты через `httptest`.
- `internal/store/`: интерфейс, память, PostgreSQL и общий контракт тестов.
- `migrations/`: начальная схема и демонстрационные пользователи.
- `.github/workflows/`: CI; `compose.yaml`: временная локальная БД.

### Если что-то не работает

- Занят `8080`: используйте `ADDR=127.0.0.1:8081 make run` и измените URL curl.
- Занят `55432`: освободите порт либо используйте отдельную тестовую БД
  с соответствующим `TEST_DATABASE_URL`.
- БД недоступна: проверьте `docker compose ps`, `docker compose logs postgres`
  и строку подключения; запустите `make db-up`.
- Ошибка отсутствующей таблицы при работе API: для пустой Compose-БД выполните
  `make db-init`; `/healthz` не выявляет отсутствующую схему.
- `-race` требует cgo/компилятор: установите C toolchain, проверьте `go env CGO_ENABLED`
  и при необходимости запускайте `CGO_ENABLED=1 make test`.

### Адаптация под свой проект

Переименуйте модуль и импорты, замените демонстрационные токены проверяемой
идентификацией, добавьте правила предметной области, миграции и тесты общего
контракта хранилищ. Перед production нужны управление секретами, HTTPS,
политики доступа, ограничения запросов, мониторинг, readiness-проверки,
инструменты миграций и эксплуатационная защита. Пагинации в примере нет.
