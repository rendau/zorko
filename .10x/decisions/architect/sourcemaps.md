# Architect — sourcemaps

ADR: 008 (перевод при приёме), 009 (приём, S3, токены), 010 (карты с сайта), 011 (имена функций).
Спека: `specs/2026-11-04-sourcemaps-design.md`.

## Компоненты
| Часть | Где |
|---|---|
| S3-клиент | `internal/infra/s3` (обёртка `minio-go`: Put/Get/Stat/Delete/List, no-op без `S3_ENDPOINT`) |
| HTTP-клиенты | `internal/infra/httpx.New(Config)` — фабрика по общим правилам (site fetch) |
| токены CI | `internal/domain/org_token` (model, repo/db, service: генерация, хеш, `Auth`) |
| архивы и файлы | `internal/domain/artifact` (model `Bundle/File/Blob/SiteFetch`; repo/db; repo/s3 — блобы и куски; service) |
| релизы | `internal/domain/release` (model, repo/db, service: `Ensure`, `Update`) |
| проект | `internal/domain/project` — поля `SiteFetch*` |
| сборка архива | `internal/service/assemble` (фон, ≤2 одновременно, zip → файлы → PG) |
| карты и имена | `internal/service/symbolicate` (поиск, кэш, `sourcemap`, `scope` на goja, чистка путей, перевод `Event`) |
| сайт | `internal/service/sitefetch` (httpx с проверкой адреса, singleflight, память в PG) |
| уборка | `internal/service/artifactgc` (сутки, `pg_try_advisory_lock`) |
| API CI | `internal/usecase/upload` + `internal/handler/sentryapi` (на сервере приёма) |
| API интерфейса | `usecase/org_token`, `usecase/artifact`, `usecase/project` (+поля); `handler/grpc/{org_token,artifact_bundle,site_fetch}.go` + proto |
| обработчик | `service/processor` — стадия `symbolicate` между `normalize` и `group` |
| фронт | `admin/` — карточка (стек), проект (карты, сайт, «Сборка»), организация (токены) |

## Потоки
- CI: Bearer → `org_token.Auth` → chunk-upload (S3) → assemble (PG замок → фон `assemble` → `ok`).
- Ошибка: normalize → symbolicate (debug_meta → artifact_file → S3 → LRU → кадры; иначе релиз+путь;
  иначе sitefetch) → group → record/store.
- Уборка: архивы по `last_used_at` → блобы без ссылок → куски > суток.

## Отказы
- S3 недоступен: заливка → 503 `{"detail"}`; перевод → кадры с `storage_unavailable`, приём идёт.
- Копия упала во время сборки → через 10 минут сборку берёт следующий запрос `assemble`.
- Сайт недоступен → `site_fetch` `error`, повтор через интервал.
