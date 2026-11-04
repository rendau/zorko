# zorko

Свой трекер ошибок, совместимый с SDK Sentry: фронтендеры подключают официальные библиотеки
Sentry (`@sentry/vue`, `@sentry/react`, `sentry_flutter`, `@sentry/react-native`) и меняют только
адрес (DSN). Хранилища: Postgres (проекты, ключи, проблемы), ClickHouse (события, 30 дней),
Redpanda (очередь между приёмом и обработкой), S3 (карты исходников).

Проект и решения — `.10x/` (спека шага 1 — `.10x/specs/2026-11-04-error-ingest-design.md`, ADR — `.10x/adrs/`).

## Порты

| Порт | Что |
|---|---|
| `INGEST_PORT` (8000) | приём SDK: `POST /api/{project_id}/envelope/`, `/store/`; заливка карт и релизы для `sentry-cli` / `@sentry/vite-plugin`: `/api/0/…` — публичный, свой домен |
| `HTTP_PORT` | API интерфейса (HTTP-шлюз): вход (`/usr/login`), проблемы, события, проекты, люди — с JWT |
| `ADMIN_PORT` (8001) | интерфейс (SPA из `admin-ui`, `make build-admin`) |
| `GRPC_PORT` (5050) | админский API (gRPC) |
| 3003 | `/healthcheck`, `/metrics`, `/docs/*` |

## Подключить проект

1. Откройте интерфейс: при первом запуске он попросит создать администратора.
2. «Проекты» → новая организация → новый проект → новый ключ.
3. На странице проекта — DSN и готовый код подключения для Vue, React, браузера, Flutter, React Native.

```js
Sentry.init({ dsn: '<dsn из zorko>', release: 'shop@1.4.2', environment: 'production' })
```

## Карты исходников

Сжатый стек переводится в исходный (файл, строка, функция, код) при приёме ошибки — спека
`.10x/specs/2026-11-04-sourcemaps-design.md`, ADR 008–011.

1. «Проекты» → организация → «Токены CI» → новый токен. Он показывается один раз; в CI — переменная
   `SENTRY_AUTH_TOKEN` (адрес zorko и организация уже внутри токена).
2. На странице проекта, вкладка «Сборка: карты» — готовый `vite.config` с `@sentry/vite-plugin`
   (или те же шаги через `sentry-cli`). Плагин заливает карты и удаляет `.map` из сборки.
3. Без CI — «Карты с сайта» в проекте: zorko скачает карту сам, только с перечисленных хостов и не
   чаще интервала. Выключено по умолчанию.

S3: `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_REGION`, `S3_SECURE`. Без
`S3_ENDPOINT` карты выключены, остальное работает. Карта удаляется, если 30 дней по ней не было ошибок
(`SOURCEMAP_RETENTION`).

## Локально

```bash
scripts/stand.sh up          # Postgres, ClickHouse, Redpanda, MinIO в docker (CLAUDE.md, «Тестовый стенд»)
cp .env.example .env && make build && ./cmd/build/svc
go test ./...                            # модульные
go test -tags integration ./...          # на стенде
go run ./tools/loadgen -dsn <dsn> -n 100000 -c 64 -distinct 1   # нагрузка на приём
go run ./tools/loadgen -dsn <dsn> -n 100000 -js dist/assets/index-….js  # то же с переводом стека (файл после sentry-cli sourcemaps inject, карта залита)
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
