# Senior Engineer — error-ingest

## Подход по задачам
- **T2:** `chconn.go` — `clickhouse.Open` по `CH_DSN` (нативный протокол); миграции CH —
  golang-migrate `clickhouse://…?x-multi-statement=true`, отдельная таблица версий.
- **T4:** строго по skill `crud`. `project_key.Create` в domain service генерирует
  `public_key` (32 hex из `crypto/rand`); `Dsn` вычисляется в domain service из
  `config.Conf.IngestPublicUrl`. Фильтры: `project.org_id`, `project_key.project_id`.
- **T5:** продюсер `kgo.NewClient(SeedBrokers, ProducerBatchCompression(zstd), ProducerLinger(5ms),
  RequiredAcks(AllISRAcks))`, `ProduceSync` по пачке записей одного запроса. Потребитель —
  `ConsumerGroup`, `DisableAutoCommit`, `PollRecords(ctx, n)`, `CommitRecords` после обработки;
  `BlockRebalanceOnPoll` + `AllowRebalance`, чтобы ребаланс не рвал пачку.
- **T6:** keycache — `map[projectId]map[publicKey]Key` под `atomic.Pointer`, полная перечитка
  раз в 30 с; ratelimit — `golang.org/x/time/rate` на ключ (map + mutex), лимит 0 — пропуск.
- **T7:** envelope — построчный разбор: заголовок (JSON), затем пары «заголовок элемента +
  payload»; длина payload из `length`, иначе до `\n`. Распаковка через `io.LimitReader`
  (лимит + 1 байт → 413). Ответ 200 с `event_id` первого события.
- **T8:** локальные структуры Sentry с json-тегами в `normalize/service/model`; поля, которые
  могут быть и строкой, и объектом (`message`, `tags` как map или список пар, `exception` как
  объект с `values` или массив) — свои `UnmarshalJSON`. Вычистка — обход
  `map[string]any` до разбора в структуры, затем `json.Marshal` очищенного дерева в `data`.
- **T9:** отпечаток — чистые функции над доменной моделью события; регулярки — пакетные `var`.
- **T10:** `issue` — кастомные запросы squirrel/pgx в `repo/db/custom.go`: `ResolveHashes`
  (`select … where project_id = $1 and hash = any($2)` по группам), `CreateWithHash`
  (tx + `on conflict do nothing returning`), `Bump` (`with old as (select … for update)
  update … from old, (values …) returning`). `event` — `conn.PrepareBatch` для каждой таблицы.
- **T11:** цикл: Poll → normalize/grouping (ошибки — метрика, пропуск) → `issue.Bump` (повтор
  при ошибке, без commit) → вставки CH (повтор с паузой 1→30 с) → Commit. Остановка по ctx.

## Сложные места
- Гонка создания проблемы двумя копиями (T10) — обязателен тест с двумя параллельными вызовами.
- Сжатая бомба и `length` больше тела (T7) — тесты.
- Время событий и TTL (T8) — зажим в окно `[received − 30 дн, received + 1 мин]`.
