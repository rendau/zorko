# SDE — error-ingest: что построено

## Построено (T1–T13)
- Стенд `scripts/stand.sh` (Postgres 17, ClickHouse 25.8, Redpanda v25.2.1), раздел в CLAUDE.md.
- CRUD `org`, `project`, `project_key` (gRPC + HTTP), генерация ключа, готовый `dsn`,
  нормализация `allowed_origin`, отказы по делу: `slug_exists`, `org_not_found`, `project_not_found`.
- Миграции PG и CH `000001_init`.
- `service/queue` (franz-go): продюсер, потребитель группы, создание темы.
- `service/keycache`, `service/ratelimit`.
- `handler/ingest` + `usecase/ingest`: envelope/store, ключ из 3 мест, gzip/deflate/br,
  пределы до и после распаковки, CORS, разрешённые сайты, 403/413/429/503.
- `service/normalize`: разбор полиморфных полей Sentry, вычистка, `{{auto}}` IP, браузер/ОС из
  User-Agent, зажим времени.
- `service/grouping`: отпечаток v1, заголовок, место падения.
- `domain/issue` (поиск, заведение с гонкой, `Bump` с разрешением на минуту), `domain/event` (CH).
- `service/processor`: пачки, повтор только CH, предел времени попытки, остановка.
- `tools/loadgen` — нагрузка и замер скорости.

## Отступления от спеки/плана
1. **Миграции ClickHouse — свой раннер** (`internal/infra/chmigrate`), а не golang-migrate: его
   драйвер на clickhouse-go v1 регистрирует database/sql-драйвер под тем же именем, что v2 →
   паника при старте. Блокировки между копиями нет, миграции пишутся `if not exists`.
2. **Метрики:** вместо `ingest_event_total{result}` — `ingest_request_total{result}` (исход
   запроса) и `ingest_event_accepted_total`; вместо `processor_lag` — `processor_lag_seconds`
   (возраст самого старого события пачки); добавлен `processor_retry_total{stage}`;
   `ingest_item_dropped_total` также считает `event_too_large`.
3. **Новые env:** `KAFKA_TOPIC_PARTITIONS` (12), `KAFKA_TOPIC_RETENTION` (24h) — тема создаётся
   при старте.
4. **Предел времени попытки записи 30 с** (`StageTimeout`): найдено на проверке — замёрзший
   ClickHouse держит соединение, и вставка висела бы вечно без ошибки.
5. **Synthetic-исключения:** SDK 11 шлёт `captureMessage` как исключение без типа со стеком
   (`mechanism.synthetic`). Тип не участвует в отпечатке, заголовок — сообщение, место падения —
   не из кадров (сверху кадры SDK), а `transaction`/путь страницы.
6. **Браузер/ОС из User-Agent** (`github.com/mileusna/useragent`): браузерный SDK их не шлёт,
   Sentry вычисляет на сервере. Версия браузера — до «major.minor».
7. **Удалённый проект:** его отпечатки получают `IssueId = 0` и пропускаются
   (`processor_event_total{result="project_deleted"}`), а не роняют пачку.
8. **Образцы SDK:** в тестах — настоящие пачки `@sentry/node` 11.10.0; `@sentry/browser` 11.10.0
   проверен вживую в Chrome. Образцы `sentry_flutter` и `@sentry/react-native` не сняты — шаг 6.

## Технический долг
- Нет общего предела одновременных запросов приёма: 100 параллельных тел по 20 МБ — до 2 ГБ
  памяти. Нужен семафор по байтам или меньший `INGEST_MAX_BODY_BYTES`.
- Падение копии между записью в PG и подтверждением (и ребаланс во время долгой аварии CH) →
  повтор пачки → `times_seen` может немного «переплюсовать» (принято в ADR-003).
- До шага 3 кадры SDK, вшитого в бандл приложения, считаются «своими».
