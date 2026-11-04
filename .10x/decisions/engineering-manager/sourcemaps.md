# Engineering Manager — sourcemaps

| # | Задача | Зависит от |
|---|---|---|
| M1 | MinIO в `stand.sh` + CLAUDE.md; `infra/httpx`, `infra/s3`; конфиг; схема PG в `000001_init` | — |
| M2 | `domain/org_token` + `usecase/org_token` + proto/handler; `Auth` для API CI; тесты | M1 |
| M3 | `domain/artifact` (repo/db, repo/s3), `service/assemble`, `usecase/upload` (chunk-upload, assemble), `handler/sentryapi`; тесты пределов | M1, M2 |
| M4 | `domain/release` + ручки релизов, `/api/0/`, `repos` в `sentryapi` | M2 |
| M5 | **спайк goja на сжатом JS Vite**; `symbolicate`: smap, scope, пути, LRU, поиск по метке и релизу; тесты на фикстурах настоящей сборки | M3 |
| M6 | стадия `symbolicate` в processor, правка `Data` (`raw_stacktrace`, `zorko_error`), `Dist`, перегруппировка; тесты «два релиза — одна проблема» | M5 |
| M7 | `service/sitefetch` (httpx + проверка адреса, singleflight, `site_fetch`), поля проекта; тесты SSRF и памяти | M5 |
| M8 | `service/artifactgc` + отметка `last_used_at`; тесты | M3, M6 |
| M9 | ручки интерфейса: `artifact_bundle` (список, файлы, удаление), `site_fetch`, поля проекта | M3, M7 |
| M10 | фронт: стек в карточке, плашка, переключатель; проект — карты, сайт, «Сборка»; организация — токены; ru/en | M6, M9 |
| M11 | вживую: Vite + `@sentry/vite-plugin` + `@sentry/vue`, `sentry-cli`, старый способ, сайт; нагрузка; Chrome; отчёты QA и безопасности | всё |

Риски: имена функций (M5, начинается со спайка); неучтённые вызовы плагина (M11 → правки M4);
скорость приёма (M11, нагрузка).
