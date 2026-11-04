# Senior Engineer — sourcemaps

- **M1.** MinIO `minio/minio` в `stand.sh` (контейнер `zorko-minio`, порты 19100 API / 19101 консоль,
  `zorko`/`zorko-secret`, бакет `zorko` создаётся при старте через `mc` или `MakeBucket` в тестах).
  `httpx.New(Config{DialTimeout, TLSHandshakeTimeout, MaxConnsPerHost, DialControl})` — нули → дефолты.
  `infra/s3.Client`: `Put(ctx,key,r,size)`, `Get`, `Exists`, `Delete`, `ListOlder(prefix, before)`.
- **M2.** Токен: `sntrys_` + base64.StdEncoding(JSON) + `_` + base64.RawStdEncoding(32 байта).
  Тест: разбор тем же алгоритмом, что `sentry-cli` (`org_auth_token.rs`). `Auth(raw) → OrgToken`
  ищет по SHA-256; `last_used_at` — не чаще раза в минуту (`where last_used_at < now()-1m`).
- **M3.** `sentryapi` — `net/http` mux на сервере приёма рядом с `ingest` (общий `Router`),
  `MaxBytesReader` 32 МБ. Куски: `multipart.Reader` потоком, `file_gzip` → `gzip.Reader` с
  `LimitReader(8 МБ+1)`. Assemble: `INSERT … ON CONFLICT DO NOTHING RETURNING` = захват; фон —
  семафор на 2. Сборка: куски по порядку → `os.CreateTemp`, `sha1` на лету → `zip.OpenReader`
  → `manifest.json` → файлы (предел на каждый через `LimitReader`) → S3 `file/<sha1>` gzip →
  транзакция PG. Состояния и `detail` — в строке `artifact_bundle`.
- **M4.** `release.Ensure(org, version, projectIds)` — upsert; ответы в JSON Sentry (`dateCreated` и пр.).
  Неизвестный путь `/api/0/…` → 404 `{"detail":"not found"}` + лог, чтобы заметить в M11.
- **M5.** Спайк: собрать Vite-приложение, разобрать `dist/assets/*.js` goja, проверить объемлющие
  функции. `smap`: `go-sourcemap` (`Parse`, `Source(line, col)`); колонки SDK 1-based, UTF-16 →
  байты строки. `scope`: обход AST goja → `[]Scope{Start, End, NamePos, Name}` отсортированы, поиск
  самой внутренней. LRU по байтам (`container/list`), ключ — SHA-1 блоба. Поиск: по метке —
  `artifact_file` с проверкой проекта через `artifact_bundle_project`; по релизу — `~/путь` варианты.
- **M6.** Стадия в `processBatch`: normalize → symbolicate(ctx с бюджетом 10 с) → group. Правка
  `Data`: `map[string]any` → `exception.values[i].stacktrace.frames` → заменить, старое в
  `raw_stacktrace`; `json.Marshal` обратно. `in_app` пересчитывается. Тест: два «релиза» с разными
  сжатыми именами → один `Hash`.
- **M7.** `Dialer.Control` проверяет `netip.Addr` (loopback, link-local, unspecified, multicast);
  `CheckRedirect` ≤3 + хост из списка. `singleflight.Group` по `project:url`. Память — `site_fetch`
  (чтение перед запросом, запись после). Тесты на `httptest` + подмена резолвера.
- **M8.** `artifactgc` в процессоре: тикер сутки (первый — через 5 мин после старта),
  `pg_try_advisory_lock(<const>)`. Отметка: `symbolicate` копит `bundle_id` в `sync.Map`, раз в
  минуту `update … where id = any($1) and coalesce(last_used_at,'epoch') < now()-1h`.
- **M9.** proto `artifact_bundle.proto`, `org_token.proto`, `site_fetch.proto`; поля проекта в
  `project.proto`. Права — как у ключей (смотреть всем, менять админу).
- **M10.** `event.ts`: кадры + `raw_stacktrace` + `zorko_error`; компонент стека — переключатель и
  пометки; страницы проекта и организации; `sdk.ts` — вкладка «Сборка».
- **M11.** Приложение-образец в scratchpad (не в репозитории): Vue + Vite + `@sentry/vue` +
  `@sentry/vite-plugin`, раздача `dist` через `npx serve`/python; Chrome для клика; журнал запросов
  статики для «один запрос».

## Спайк M5 (сделан до M1, 2026-11-04)
Vite 8.3.5 (rolldown/oxc) + Vue 3.5, бандл 178 КБ, карта 1,7 МБ: goja разбирает за 7,5 мс,
`go-sourcemap` — 9 мс. Позиции goja `Idx` — байтовые, с 1. Колонки карты — UTF-16: на строке с
кириллицей байтовая колонка даёт неверный кадр → перевод «UTF-16 ↔ байты» обязателен.
Результат: `promo.js:10` в `validateRule` (было `yv`), `Cart.vue:17` в `missingRule` (было `a`);
стрелочная в переменной (`bv=e=>…`) — имя берётся из привязки; методы (`applyCode`) — ключ как есть.
В goja нет `ast.Walk` — свой обход по reflect. ADR-011 подтверждён.
