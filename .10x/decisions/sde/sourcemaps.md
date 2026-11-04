# SDE — sourcemaps (шаг 3)

Спека `specs/2026-11-04-sourcemaps-design.md`, ADR 008–011. Задачи M1–M11 — сделаны.

## Что построено
| Часть | Где |
|---|---|
| S3, HTTP-клиенты | `internal/infra/s3` (minio-go, `EnsureBucket`), `internal/infra/httpx` (фабрика: dial/TLS 2 с, 20 на хост, InsecureSkipVerify, `DialControl`) |
| токены CI | `domain/org_token` (формат `sntrys_` как в sentry-cli, в базе SHA-256), `usecase/org_token`, `/org_token` |
| архивы | `domain/artifact` (repo/db: архивы, файлы, поиск, site_fetch, уборка; repo/blob: `chunk/`, `file/` в S3), `service/assemble` |
| API sentry-cli | `handler/sentryapi` + `usecase/upload` на сервере приёма: `/api/0/`, org, chunk-upload, artifactbundle/assemble, releases, deploys, repos |
| релизы | `domain/release` (версия, проекты, `date_released`) |
| перевод стека | `service/symbolicate` (поиск по метке → по релизу+пути → сайт; LRU по байтам; `jsscope` — имена функций) |
| карты с сайта | `service/sitefetch` (список хостов, проверка адреса после DNS, ≤3 перенаправления, singleflight, память в `site_fetch`) |
| уборка | `service/artifactgc` (сутки, `pg_try_advisory_lock`) |
| ручки интерфейса | `/artifact_bundle`, `/artifact_bundle/{id}/file`, `/site_fetch`, поля `site_fetch_*` у `/project` |
| фронт | `ExceptionBlock` (переключатель, пометки, плашка), `settings/BundleList`, `SiteFetchForm`, `TokenList`, вкладка «Сборка: карты» (`utils/sdk.ts`) |
| обработчик | стадия `symbolicate` между normalize и group; `Event.Dist`, `Event.DebugImages` из `debug_meta` |
| стенд | MinIO `cgr.dev/chainguard/minio` в `scripts/stand.sh` (19100/19101) |
| нагрузка | `tools/loadgen -js <inject-нутый файл>` — кадры по настоящему сжатому файлу с меткой |

## Отступления от плана
- **goja не понимает ES-модули** (`import`/`export`, `import.meta`, `import()`, await верхнего уровня),
  а сборки Vite — модули. Добавлен `jsscope/demodule.go`: лексер `tdewolff/parse/v2/js` находит эти
  конструкции и заменяет кодом той же длины (позиции не сдвигаются), плюс обёртка в async-функцию.
  `t14raptor/go-fast` проверен — модулей тоже не знает.
- **`@sentry/vite-plugin` 11 ходит новым JS-клиентом `sentry` 0.45**, а не Rust `sentry-cli`: он сперва
  запрашивает `GET /api/0/organizations/{org}/` (адрес региона) — ручка добавлена.
- Плагин заливает файлы под именами `~/<debugId>-0.js`: его сборки находятся **только по метке**; поиск
  по релизу и пути работает для `sentry-cli sourcemaps upload` (проверено с `?v=3` в адресе).
- Удалённый архив может ещё **до минуты** находиться обработчиком (память поиска), а не «сразу».
- Отдельная причина `site_address_blocked` для внутренних адресов (было бы «сайта нет в списке» —
  вводило в заблуждение, если хост в списке есть).
- `StoreFile` сперва отмечает файл в `blob` (`created_at = now()`), а уборка трогает файлы старше суток:
  иначе уборка могла удалить файл, который как раз попадает в новый архив.
- `minio/minio` больше нет в Docker Hub — образ Chainguard; данным нужен том (`-v /data`).
- Стендовая база пересоздана: единый `000001_init` изменился (проект не развёрнут).
- В `WaitJobs` symbolicator ждётся до processor (один порядок с `Start`): отметки «пригодился» последней
  пачки при остановке могут потеряться — отметка часовая, на уборку через 30 дней не влияет.

## Технический долг
- Перевод разбирает и собирает JSON события второй раз: обработка с картами ~в 1,6 раза медленнее
  (8,5 с → 13,6 с на 100 тыс. событий), приём не меняется. Можно править кадры на дереве из normalize.
- `go-sourcemap`: составные карты (sections) с подозрительной арифметикой смещений; Vite их не делает.
- Singleflight карт с сайта — внутри копии; несколько копий могут скачать один адрес по разу.
- Значение секретного заголовка сайта лежит в PG открытым текстом (нужно для отправки).
- Повторного перевода принятых ошибок нет (решение ADR-008).
