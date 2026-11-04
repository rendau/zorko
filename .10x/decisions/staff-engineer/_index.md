# Staff Engineer — общие решения

## Соглашения из каркаса и CLAUDE.md
- Таблицы и REST-пути — в единственном числе; пагинация с `page=0`; update — `PUT`.
- Ошибки: отказ по делу — `errs.Err`, прочее — сбой (`service_not_available`);
  `errors.Is`/`errors.AsType`, обёртка `fmt.Errorf("...: %w")`.
- Доменные модели без json-тегов; DTO ↔ domain через `lo.Map`.
- Тесты — testify; параллельная обработка — errgroup; `wg.Go`.
- Указатели — `new(expr)`; JSON — только `encoding/json`.
- HTTP-клиенты — только через фабрику с таймаутами (dial/TLS 2s, 20 соединений на хост).
- Проверка: `gofmt -l`, `go vet ./...`, `go test ./...`. golangci-lint не запускаем.
- Тестовые БД — только в docker; стенд фиксируется в CLAUDE.md проекта.
- Миграции: пока не задеплоено — один `000001_init`.

## Активные
- [error-ingest](error-ingest.md) — библиотеки и раскладка шага 1
- [issue-ui](issue-ui.md) — шаг 2
