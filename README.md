# zorko

Свой трекер ошибок, совместимый с SDK Sentry: фронтендеры подключают официальные библиотеки
Sentry (`@sentry/vue`, `@sentry/react`, `sentry_flutter`, `@sentry/react-native`) и меняют только
адрес (DSN). Хранилища: Postgres (проекты, ключи, проблемы), ClickHouse (события, 30 дней),
Redpanda (очередь между приёмом и обработкой), S3 (карты исходников, записи сессий).

Проект и решения — `.10x/` (спека шага 1 — `.10x/specs/2026-11-04-error-ingest-design.md`, ADR — `.10x/adrs/`).

## Порты

| Порт | Что |
|---|---|
| `INGEST_PORT` (8000) | приём SDK: `POST /api/{project_id}/envelope/`, `/store/`; заливка карт и релизы для `sentry-cli` / `@sentry/vite-plugin`: `/api/0/…` — публичный, свой домен |
| `HTTP_PORT` | API интерфейса (HTTP-шлюз): вход (`/usr/login`), проблемы, события, проекты, люди — с JWT (свой или провайдера, см. «Вход в интерфейс») |
| `ADMIN_PORT` (8001) | интерфейс (SPA из `admin-ui`, `make build-admin`) |
| `GRPC_PORT` (5050) | админский API (gRPC) |
| 3003 | `/healthcheck`, `/metrics`, `/docs/*` |

## Вход в интерфейс

Способ входа выбирается при установке переменной `AUTH_MODE` (ADR-019), один на установку:

- **`local`** (по умолчанию) — свои учётки zorko: логин и пароль, первый админ — на `/setup`.
  Нужен `JWT_SECRET`.
- **`oidc`** — внешний OpenID-провайдер (Keycloak, id.mdev.kz и т.п.). Интерфейс сам ведёт вход,
  zorko проверяет подпись токена по ключам провайдера. В провайдере — public-клиент с адресом
  возврата `https://<интерфейс>/login/callback` и адресом после выхода `https://<интерфейс>/login`.
  ```
  AUTH_MODE=oidc
  OIDC_ISSUER=https://keycloak.example.kz/realms/corp
  OIDC_CLIENT_ID=zorko-ui
  AUTH_ADMIN_ROLE=zorko-admin
  ```
- **`proxy`** — перед API zorko стоит доверенный шлюз со своим входом (ruto): он проверяет токен,
  держит его в куках и передаёт бэкенду `Authorization: Bearer`. zorko подпись **не проверяет**,
  поэтому API zorko должен быть доступен только через шлюз (без своего Ingress API, все ручки на
  шлюзе — с проверкой входа). Интерфейс знает только адреса шлюза, `{url}` — адрес возврата:
  ```
  AUTH_MODE=proxy
  ADMIN_API_BASE_URL=https://api.mdev.kz/zorko
  AUTH_PROXY_LOGIN_URL=https://api.mdev.kz/_auth/id/login?redirect={url}
  AUTH_PROXY_REFRESH_URL=https://api.mdev.kz/_auth/id/refresh
  AUTH_PROXY_LOGOUT_URL=https://api.mdev.kz/_auth/id/logout?redirect={url}
  AUTH_ADMIN_ROLE=zorko:admin
  ```

В `oidc` и `proxy`:

- **Человек заводится сам** при первом входе и **не видит ни одного проекта**, пока админ не выдаст
  их в «Людях» (или галочку «Все проекты»). Узнаётся по claim `sub`; заведённый админом заранее
  (по логину как у провайдера) привязывается при первом входе и сохраняет выданные проекты — так
  же переезжают люди, заведённые до включения внешнего входа.
- **Админ — по роли из токена** `AUTH_ADMIN_ROLE`: `клиент:роль` ищется в
  `resource_access.<клиент>.roles` (id.mdev.kz, роли клиентов Keycloak), просто `роль` — в `roles`,
  `role`, `realm_access.roles` и у любого клиента. Для необычного провайдера — `AUTH_ROLES_CLAIM`,
  путь к ролям через точку (`groups`). Логин — `AUTH_USERNAME_CLAIM`, по умолчанию
  `preferred_username`, затем `email`. Роль сняли у провайдера — человек перестаёт быть админом
  со следующим токеном.
- Пароля, первого админа на `/setup` и правки своего логина нет — этим управляет провайдер.
  Отключить человека в zorko можно: при входе он увидит «учётная запись отключена».

## Подключить проект

1. Откройте интерфейс: при первом запуске (`AUTH_MODE=local`) он попросит создать администратора.
2. «Проекты» → новая организация → новый проект → новый ключ.
3. На странице проекта — DSN и готовый код подключения для Vue, Nuxt, React, браузера, Flutter,
   React Native.

```js
Sentry.init({ dsn: '<dsn из zorko>', release: 'shop@1.4.2', environment: 'production' })
```

- **Версия** (`release`) — в примерах её собирает конфиг сборки: версия из `package.json` + короткий
  хеш коммита (`shop@1.4.2-a1b2c3d`). Без версии ошибки принимаются, и карты тоже находятся — по
  метке сборки (debug id), которую плагин ставит в каждый файл.
- **Окружение** (`environment`) задавайте явно (`staging`, `production`): `import.meta.env.MODE`
  и `NODE_ENV` на тестовом стенде и на бою одинаковые — `production`.
- **Nuxt** — `@sentry/vue` в client-плагине и обязательно `attachErrorHandler: false`, а ошибки
  компонентов ловят хуки Nuxt `vue:error` и `app:error` (готовый код — на вкладке Nuxt). Без этого
  Sentry оборачивает обработчик ошибок Vue, Nuxt не может снять свой после загрузки, и любая ошибка
  в компоненте открывает страницу ошибки (`error.vue`).
- **Мобильные приложения на Capacitor / Cordova** шлют ошибки с адреса `capacitor://localhost` (iOS)
  и `https://localhost` (Android) — для zorko это сайт `localhost`. Если у ключа заполнен список
  разрешённых сайтов, добавьте в него `localhost` (кнопка в окне ключа), иначе ошибки с телефонов
  получают 403.

## Карты исходников

Сжатый стек переводится в исходный (файл, строка, функция, код) при приёме ошибки — спека
`.10x/specs/2026-11-04-sourcemaps-design.md`, ADR 008–011.

1. «Проекты» → организация → «Токены CI» → новый токен. Он показывается один раз; в CI — переменная
   `SENTRY_AUTH_TOKEN`. Без неё примеры карты не строят вовсе — чтобы `.map` без заливки не попали
   на сайт.
2. На странице проекта, вкладка «Сборка: карты» — готовый `vite.config` или `nuxt.config` с
   `@sentry/vite-plugin` (или те же шаги через `sentry-cli`). Плагин заливает карты, а `.map` из
   сборки убираются.
3. Без CI — «Карты с сайта» в проекте: zorko скачает карту сам, только с перечисленных хостов и не
   чаще интервала. Выключено по умолчанию.

Особенности Nuxt (всё уже учтено в примере): плагин подключается в `vite.plugins` (в `vite.$client`
плагины задать нельзя), каждому его плагину добавлен `applyToEnvironment: env => env.name === 'client'`
— заливается только браузерная часть; `sourcemap: { server: false, client: 'hidden' }`. Опция плагина
`filesToDeleteAfterUpload: ['./dist/**/*.map']` в Nuxt промахивается — промежуточная папка сборки
там `.nuxt` или `node_modules/.cache/nuxt/.nuxt`, — поэтому `.map` удаляет хук Nuxt
`nitro:build:public-assets` из `nitro.options.output.publicDir`.

### Если карты не заливаются

Проверено с `@sentry/vite-plugin` 11.10.

- **`org` в конфиге обязателен.** Плагин 11.x внутри работает через новый CLI Sentry (пакет
  `sentry`), а он, в отличие от `sentry-cli`, не берёт организацию и адрес из токена. Нужны `url`,
  `org` и `project` — они есть в готовом примере.
- **С ноутбука заливка уходит на `localhost:8000`.** Новый CLI запоминает «организация → адрес»
  в `~/.sentry/cli.db`. Если zorko уже запускали локально с той же организацией, он шлёт карты
  туда. Лечится сборкой с пустой папкой настроек: `SENTRY_CONFIG_DIR=$(mktemp -d) npm run build`
  (или удалите эту организацию из `~/.sentry/cli.db`). На CI папка каждый раз чистая — проблемы нет.
- **`setCommits: false`.** По умолчанию плагин пытается отправить список коммитов релиза; zorko
  коммиты не хранит, такого адреса у него нет — без этой опции лишний запрос и шум в логе сборки.

S3: `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_REGION`, `S3_SECURE`. Без
`S3_ENDPOINT` карты выключены, остальное работает. Карта удаляется, если 30 дней по ней не было ошибок
(`SOURCEMAP_RETENTION`).

## Запись сессий

Запись экрана сайта перед ошибкой, как Session Replay у Sentry: обычный `replayIntegration()` из
`@sentry/vue` / `@sentry/react` / `@sentry/browser`. Это не видео, а снимок страницы и её изменения
(rrweb): клики, переходы, консоль, запросы к серверу. Спека — `.10x/specs/2026-11-06-replay-design.md`,
ADR 012–016.

1. «Проекты» → проект → «Запись сессий»: режим «Только при ошибке» (минута до ошибки и дальше до
   конца сессии) или «При ошибке и часть всех сессий». По умолчанию выключено.
2. В примерах подключения появится `replayIntegration`, `replaysOnErrorSampleRate` и
   `replaysSessionSampleRate`. Если на сайте запись уже включена (как у seller), новый билд не нужен.
3. Записи — в карточке проблемы (вкладка «Записи», кнопка «Смотреть запись» у события) и в разделе
   «Записи».

- **Выключить или урезать можно сразу, без релиза сайта.** Режим и предел новых записей в час
  доходят до приёма за ≤ 30 с; лишняя запись получает ответ 200 и заголовок
  `X-Sentry-Rate-Limits: …:replay:project`, браузер перестаёт её писать, ошибки идут как прежде.
  Долю сессий решает код сайта — новая доля доходит только с релизом.
- **Личные данные.** В примере весь текст и картинки скрыты (`maskAllText`, `blockAllMedia`). Сайт
  может выключить скрытие — тогда в записи видно всё, кроме паролей. Сообщения консоли SDK пишет
  как есть.
- **Заголовки запросов.** Во вкладке «Запросы» запрос раскрывается стрелкой: заголовки запроса и
  ответа и размеры тел. Сайт присылает их только для адресов из `networkDetailAllowUrls`; сверх
  `Content-Type`, `Content-Length` и `Accept` — те, что перечислены в `networkRequestHeaders` /
  `networkResponseHeaders` (всё это есть в примере подключения, нужен релиз сайта). Видно только то,
  что код сайта выставил сам: `Cookie`, `User-Agent` и прочее браузер скрипту не показывает. Ответ
  с другого домена отдаёт свои заголовки, только если API шлёт `Access-Control-Expose-Headers`.
  Заголовки с токенами (`Authorization`) в список не добавлять — они лежат в записи 30 дней. Тела
  запросов и ответов zorko не показывает, в примере они выключены (`networkCaptureBodies: false`).
- **CSP.** Запись сжимается в фоновом скрипте из `blob:`; если у сайта есть `Content-Security-Policy`,
  нужен `worker-src 'self' blob:` — иначе запись уходит несжатой (в разы больше трафика).
- Хранение: тела сегментов — S3 `replay/<день приёма>/<проект>/<запись>/<сегмент>`, описания — ClickHouse
  (`replay_segment`, `replay_issue`), 30 дней (`REPLAY_RETENTION` и TTL миграции — менять вместе).
  Описания идут через отдельную тему `KAFKA_TOPIC_REPLAYS` (`zorko.replays`, группа
  `<KAFKA_CONSUMER_GROUP>-replay`). Без `S3_ENDPOINT` записи выключены. Тело сегмента —
  до `REPLAY_MAX_SEGMENT_BYTES` (10 МБ).
- Мобильные приложения (Flutter, React Native — там запись видео) пока не принимаются.

## Хранение и диски

Данные хранятся 30 дней, но три предохранителя не дают дискам переполниться раньше (ADR-017):

- **Журналы самого ClickHouse** (`system.*_log`) по умолчанию хранятся вечно и на пустом сервисе
  растут на десятки МБ в час. Настройки сервера — `scripts/clickhouse.xml` (стенд) и
  `charts/argo/zorko/files/clickhouse.xml` в helm-zeon (менять вместе): журналы — 3 дня,
  `trace_log` и `text_log` выключены, журнал сервера — уровень `warning`, ClickHouse не занимает
  последний 1 ГБ диска.
- **Данные zorko в ClickHouse.** Раз в `CH_DISK_CHECK_INTERVAL` (10 мин) одна из копий обработки
  смотрит диск; если занято больше `CH_DISK_MAX_USED_PERCENT` (80 %), удаляет самые старые дни во
  всех таблицах zorko, пока не станет ниже. Сегодняшний день не трогается. Метрики:
  `chdisk_used_percent`, `chdisk_kept_days` (сколько дней сейчас хранится на самом деле),
  `chdisk_dropped_days_total`. Удалять нечего, а предел превышен — ошибка в логе
  `chdiskgc: disk over limit, nothing more to drop`: место заняли не данные zorko.
- **Очередь Redpanda.** Кроме срока `KAFKA_TOPIC_RETENTION` (24 ч) — предел объёма на раздел
  `KAFKA_TOPIC_RETENTION_BYTES` (512 МБ; 12 разделов × 2 темы ≈ 12 ГБ). При наплыве старые
  сообщения уходят раньше срока. Срок и предел выставляются при старте и уже существующим темам.

Postgres хранит проблемы, проекты и метаданные карт — сотни МБ на годы, предел ему не нужен.

**Память ClickHouse** (ADR-018). Лимит пода — 2 ГБ, свой потолок ClickHouse берёт как 90 % — 1,8 ГБ.
Умолчания у него — под большой сервер, поэтому в тех же `clickhouse.xml`: не больше двух слияний
разом, блок сжатия 256 КБ вместо 1 МБ (слияние `event` вдвое легче), кэши — сотни МБ, пулы
потоков под неиспользуемое (копирование между серверами, Kafka внутри ClickHouse и т.п.) — по 1
потоку (потоков ~100 вместо ~700, памяти вдвое меньше). Признак
нехватки — `processor: retry` с `memory limit exceeded` (code 241) в логе zorko; посмотреть, кто
занимал память:

```sql
select event_time, table, formatReadableSize(peak_memory_usage) from system.part_log
where event_date = today() and event_type = 'MergeParts' order by peak_memory_usage desc limit 10;
```

После первого выката настроек ClickHouse старые журналы остаются на диске: ClickHouse
переименовывает таблицы со сменившимся сроком в `<имя>_0`, а выключенные не трогает. Посмотреть и
удалить:

```sql
select table, formatReadableSize(sum(bytes_on_disk)) from system.parts
where database = 'system' group by table order by sum(bytes_on_disk) desc;

drop table if exists system.trace_log sync;
drop table if exists system.text_log sync;
drop table if exists system.query_log_0 sync;   -- и остальные system.*_0 из списка
```

## Локально

```bash
scripts/stand.sh up          # Postgres, ClickHouse, Redpanda, MinIO в docker (CLAUDE.md, «Тестовый стенд»)
cp .env.example .env && make build && ./cmd/build/svc
go test ./...                            # модульные
go test -tags integration ./...          # на стенде
go run ./tools/loadgen -dsn <dsn> -n 100000 -c 64 -distinct 1   # нагрузка на приём
go run ./tools/loadgen -dsn <dsn> -n 100000 -js dist/assets/index-….js  # то же с переводом стека (файл после sentry-cli sourcemaps inject, карта залита)
go run ./tools/loadgen -dsn <dsn> -replay -n 5000 -c 16   # сегменты записей сессий (в проекте нужен режим записи)
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
