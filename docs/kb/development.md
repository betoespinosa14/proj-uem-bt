---
title: Development
section: Operations
order: 80
summary: Setting up a development environment, running the tests and console checks, adding a database migration, and previewing changes safely on a copy of the live data.
reviewed: 2026-11-20
---

## Never point development at the live database

On the Mac Mini, the production database is the `uem` database on `localhost:5432`. That is also the default `DATABASE_URL` in the code and the README's development example. **If you run `go run ./cmd/server` on the Mac Mini without setting `DATABASE_URL`, you are running against live data.**

Always give development servers a different database name (`uem_dev`, `uem_preview`, `uem_test`) and a different port. Ideally, develop on another computer.

## Requirements

- Go 1.26 or newer (`go.mod` says `go 1.26.0`)
- Node 22 or newer (the console's Docker image uses `node:22-alpine`)
- PostgreSQL 16
- `make`, `git`

## Run it locally

1. Create a development database (on a dev machine):
   ```bash
   createdb -h localhost -U uem uem_dev
   ```
2. Start the API server. Migrations run automatically on start.
   ```bash
   DATABASE_URL='postgres://uem:<password>@localhost:5432/uem_dev?sslmode=disable' \
   UEM_LISTEN=:8080 UEM_AGENT_DIST=dist UEM_ALLOWED_ORIGINS=localhost:3000 \
   go run ./cmd/server
   ```
3. Start the console on port 3000. `next dev` proxies `/api` to the server (`web/next.config.ts`), but it cannot proxy WebSockets, so the live feed goes straight to the server:
   ```bash
   cd web && npm install
   NEXT_PUBLIC_LIVE_URL=ws://localhost:8080/api/live npm run dev
   ```
4. Open `http://localhost:3000/signup` and create a test organization (sign-up is allowed by default).
5. Optional, an agent in the foreground with its own config file (so it does not touch a real install):
   ```bash
   make build-agents
   go run ./cmd/agent enroll --server http://localhost:8080 --token enr_… --config ./agent.json
   go run ./cmd/agent run --config ./agent.json
   go run ./cmd/agent inventory     # print what the agent collects, as JSON
   ```

## Tests

```bash
createdb -h localhost -U uem uem_test   # once
make test                               # go vet, then all Go tests with -race
make test-unit                          # Go tests without the race detector
```

`make test` sets `UEM_TEST_DATABASE_URL` from the `Makefile`'s `TEST_DB` (the `uem_test` database on localhost). Override it with `make test TEST_DB=postgres://…`. The integration tests in `internal/server/api/api_test.go` refuse to run against a database whose name does not end in `_test`, so they cannot write to the live one. Without `UEM_TEST_DATABASE_URL` they are skipped.

Console checks:

```bash
cd web && npx tsc --noEmit -p . && npm run lint
```

`npm run lint` runs ESLint and then `scripts/check-grids.mjs`. That script fails when a responsive grid (`sm:grid-cols-…` and so on) has no base `grid-cols-*`, which makes pages wider than a phone screen. The fix is to add `grid-cols-1`.

Run all of these before every release ([Releases and deploying](releases.md)).

## Adding a database migration

Migrations are SQL files in `internal/server/migrations/`, run by goose and embedded in the server binary. They run automatically when the server starts.

1. Create the next numbered file. The last one is `00013_app_usage_last_active.sql`, so the next is e.g.:
   ```text
   internal/server/migrations/00014_short_description.sql
   ```
2. Write both directions:
   ```sql
   -- +goose Up
   -- One or two lines on why this change exists.
   ALTER TABLE devices ADD COLUMN example text;

   -- +goose Down
   ALTER TABLE devices DROP COLUMN example;
   ```
3. Add the queries in `internal/server/store/`. Every tenant query takes `orgID`.
4. Run `make test`; the tests apply all migrations to `uem_test`.
5. Mention the change in `CHANGELOG.md` ("The server adds a column … on start-up.").

Prefer additive changes (new columns, new tables). The server never runs migrations backwards, so an additive change keeps a rollback to the previous release possible.

## Previewing a change on a copy of the live data

Use this on the Mac Mini to check a change against real data without touching it. The copy holds real, sensitive data: treat it like production and delete it when done.

1. Copy the live database into a new one:
   ```bash
   PG=/opt/homebrew/opt/postgresql@16/bin
   $PG/createdb -h localhost -U uem uem_preview
   $PG/pg_dump -h localhost -U uem uem | $PG/psql -h localhost -U uem -q uem_preview
   ```
2. Turn off webhooks **in the copy**, so the preview server cannot post to real Teams channels or SIEM tools:
   ```bash
   $PG/psql -h localhost -U uem uem_preview -c "UPDATE webhooks SET enabled = false"
   ```
3. Start your working-tree server on another port, against the copy:
   ```bash
   DATABASE_URL='postgres://uem@localhost:5432/uem_preview?sslmode=disable' \
   UEM_LISTEN=:8081 UEM_AGENT_DIST=dist UEM_ALLOWED_ORIGINS=localhost:3100 \
   go run ./cmd/server
   ```
   (Postgres on the Mac Mini trusts local connections, so no password is needed.)
4. Start the console on port 3100, pointed at that server:
   ```bash
   cd web
   UEM_API_URL=http://localhost:8081 NEXT_PUBLIC_LIVE_URL=ws://localhost:8081/api/live npx next dev -p 3100
   ```
5. Open `http://localhost:3100` and sign in with your normal account (the users were copied).
6. When done, stop both processes and drop the copy:
   ```bash
   $PG/dropdb -h localhost -U uem uem_preview
   ```

What to expect: devices in the copy show offline, because real agents only talk to the live server. Commands you send there are queued and never delivered. Migrations in your branch run on the copy only.

Before each `go run` on the Mac Mini, read the `DATABASE_URL` once more.

## Related

- [Environment variables](environment.md)
- [Releases and deploying](releases.md)
