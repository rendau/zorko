# Staff Engineer — sourcemaps

## Библиотеки
- `github.com/minio/minio-go/v7` — S3; транспорт из `httpx` (dial/TLS 2 с, InsecureSkipVerify, 20 на хост).
- `github.com/go-sourcemap/sourcemap` — разбор карт (вкл. секции); если на картах Vite неверно —
  свой декодер VLQ (~200 строк) в `symbolicate/service/smap`.
- `github.com/dop251/goja/parser` + `ast` — разбор сжатого JS (ADR-011).
- `github.com/hashicorp/golang-lru/v2` — не берём: нужен LRU по байтам → свой маленький (`container/list`).
- `golang.org/x/sync/singleflight`, `errgroup` — уже в зависимостях (`x/sync`).
- `archive/zip`, `compress/gzip`, `crypto/sha1`, `crypto/sha256` — стандартные.

## Соглашения
- Новые таблицы — в `000001_init` (проект не развёрнут); имена в единственном числе.
- `usecase/upload` отдаёт доменные ошибки `errs.*`; `handler/sentryapi` переводит их в
  `{"detail"}` + HTTP-код (401/403/404/400/413/503).
- Ошибки Sentry-API — свои коды в `internal/errs`: `TokenInvalid`, `ChunkInvalid`,
  `BundleTooLarge`, `SourcemapsDisabled`.
- Конфиг — `config.Conf.*` в месте использования; компоненты S3/перевода/сайта/уборки включаются
  по `S3_ENDPOINT`, иначе no-op через интерфейсы.
- Метрики — в `metrics.Registry`, как в шаге 1.
- JSON события правится через `map[string]any` (`encoding/json`), как в normalize; модель без тегов.
- jsonb не вводим: настройки сайта — обычные колонки, списки — `text[]`.
