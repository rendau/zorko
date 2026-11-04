# zorko

Свой трекер ошибок, совместимый с SDK Sentry: фронтендеры подключают официальные библиотеки
Sentry (`@sentry/vue`, `@sentry/react`, `sentry_flutter`, `@sentry/react-native`) и меняют только
адрес (DSN). Хранилища: Postgres (проекты, ключи, проблемы), ClickHouse (события, 30 дней),
Redpanda (очередь между приёмом и обработкой).

Проект и решения — `.10x/` (спека шага 1 — `.10x/specs/2026-11-04-error-ingest-design.md`, ADR — `.10x/adrs/`).

## Порты

| Порт | Что |
|---|---|
| `INGEST_PORT` (8000) | приём SDK: `POST /api/{project_id}/envelope/`, `/store/` — публичный, свой домен |
| `HTTP_PORT` | админский API (HTTP-шлюз): `/org`, `/project`, `/project_key` — **без входа, наружу не открывать** |
| `GRPC_PORT` (5050) | админский API (gRPC) |
| 3003 | `/healthcheck`, `/metrics`, `/docs/*` |

## Подключить проект

```bash
curl -XPOST $ADMIN/org -d '{"slug":"mechta","name":"Мечта"}'
curl -XPOST $ADMIN/project -d '{"org_id":"1","slug":"mechta-site","platform":"vue"}'
curl -XPOST $ADMIN/project_key -d '{"project_id":"1","label":"prod","allowed_origin":{"values":["*.mechta.kz"]}}'
curl "$ADMIN/project_key?list_params.page_size=10&project_id=1"   # поле dsn
```

```js
Sentry.init({ dsn: '<dsn из zorko>', release: 'shop@1.4.2', environment: 'production' })
```

## Локально

```bash
scripts/stand.sh up          # Postgres, ClickHouse, Redpanda в docker (CLAUDE.md, «Тестовый стенд»)
cp .env.example .env && make build && ./cmd/build/svc
go test ./...                            # модульные
go test -tags integration ./...          # на стенде
go run ./tools/loadgen -dsn <dsn> -n 100000 -c 64 -distinct 1   # нагрузка на приём
```

### DB dump:

```
pg_dump --no-owner -Fc -U postgres zorko -f ./zorko.custom
```

### DB restore:

```
dropdb -U postgres zorko
createdb -U postgres zorko
pg_restore --no-owner -d zorko -U postgres ./zorko.custom
```

### Install `migrate` command-tool:

https://github.com/golang-migrate/migrate/tree/master/cmd/migrate

### Create new migration:

```
migrate create -ext sql -dir migrations mg_name
```

### Apply migration:

```
migrate -path migrations -database "postgres://localhost:5432/db_name?sslmode=disable" up
```

### Proto generation:

```
make generate-proto
```

The proto toolchain lives in `bin/` (git-ignored) and is installed by `make proto-tools`, which
`generate-proto` runs first: `protoc` of the version pinned in the Makefile (`PROTOC_VERSION`) and
plugins of the versions from `go.mod`. Global `protoc` and plugins are not used. Installed versions:

```
make proto-tools-version
```
