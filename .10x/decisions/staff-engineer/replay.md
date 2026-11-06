# Staff Engineer — replay

## Библиотеки
- Go: только стандартные `compress/zlib`, `encoding/json`, `encoding/hex`; S3 — имеющийся
  `infra/s3` (+ `ListPrefixes`, `DeletePrefix`). Новых зависимостей нет.
- Фронт: `@sentry-internal/rrweb-player` (MIT), версия той же ветки, что `@sentry-internal/rrweb`
  в `@sentry/replay` 11.11 (2.43.x).

## Соглашения
- Миграции — новыми файлами: PG `000003_project_replay`, CH `000002_replay` (база применена).
- Таблицы и пути в единственном числе: `replay_segment`, `replay_issue`, `/replay`, `/replay/{id}/segment/{n}`.
- Пагинация с 0; доступ — `access.CheckProject` / `ProjectAccess` как у проблем.
- Доменная модель без тегов; описание сегмента в очереди — repo-локальная DTO с json-тегами в
  `service/queue/model` (как событие — сырое тело + заголовки).
- Отказы приёма записи — не ошибки HTTP, а исходы в метрике `replay_segment_total{result}`.
- Ошибки интерфейса: `errs.ReplayNotFound`, `errs.ReplayDisabled` (нет S3).
- Конфиг — `config.Conf.*`; компоненты записей включаются по `S3_ENDPOINT`, иначе no-op.
- Проверка: `gofmt -l`, `go vet ./...`, `go test ./...`; фронт — `typecheck`, `lint`, `check:i18n`, `test`.
