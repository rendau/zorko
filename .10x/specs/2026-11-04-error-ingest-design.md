# error-ingest — приём ошибок, очередь, группировка, хранение

**Дата:** 2026-11-04
**Статус:** согласовано с пользователем по частям, ждёт финального просмотра
**Шаг дорожной карты:** 1 из 6 (см. `decisions/product-manager/_index.md`)

## Зачем

Фронтендеры не видят ошибок с устройств клиентов. Шаг 1 — фундамент: zorko принимает ошибки
от официальных библиотек Sentry, склеивает одинаковые в проблемы и хранит их. Интерфейса на
этом шаге нет (шаг 2); проверка — по данным в базах.

## Что входит / не входит

**Входит:**
- приём событий в формате Sentry (envelope и старый store) на отдельном порту;
- очередь Redpanda между приёмом и обработкой;
- обработка: нормализация, вычистка чувствительных данных, отпечаток, проблемы в Postgres,
  события в ClickHouse; правило «считать всё, хранить часть»;
- админский API: организации, проекты, ключи проектов (без входа, только внутри кластера);
- миграции Postgres и ClickHouse, метрики, тестовый стенд в docker.

**Не входит (следующие шаги):** интерфейс и API проблем (2), карты исходников (3), релизы
как сущность, статусы, оповещения (4), люди и вход (5), особенности Flutter/RN (6),
замеры скорости, сессии, вложения, логи, записи сессий.

## Решения, принятые с пользователем

| Вопрос | Решение |
|---|---|
| Очередь | Kafka-протокол, брокер **Redpanda** (один бинарь, без Java) |
| Объём | неизвестен; закладываем горизонтальное масштабирование копиями |
| Хранение подробностей | **30 дней** (TTL в ClickHouse); проблемы в Postgres — бессрочно |
| Организации | **несколько** (`org → project`) |
| Наплыв | **считать всё, хранить часть**: полные записи — не больше 100/мин на проблему |
| Схема | приём → Redpanda → обработка → Postgres + ClickHouse; приём — свой порт и домен |

## Общая схема

Один бинарь `svc`, в каждой копии три части (каждую можно выключить env-ом, чтобы позже
разнести по разным деплойментам):

```
SDK Sentry ──HTTP──▶ [приём :INGEST_PORT] ──▶ Redpanda: zorko.events
                                                  │
                     [обработка, consumer group] ◀┘
                        │                │
                        ▼                ▼
                   Postgres          ClickHouse
                (issue, ключи)   (event_stat, event)

[админ API: gRPC :GRPC_PORT + HTTP-шлюз :HTTP_PORT] ──▶ Postgres (org, project, project_key)
```

## 1. Приём

### Ручки (порт `INGEST_PORT`, по умолчанию 8000; отдельный http.Server, не grpc-gateway)
- `POST /api/{project_id}/envelope/` — основной формат всех современных SDK.
- `POST /api/{project_id}/store/` — старый формат (одно событие JSON).
- `OPTIONS` на обе — CORS: разрешён любой Origin (проверка разрешённых сайтов — ниже),
  заголовки `Content-Type`, `X-Sentry-Auth`, `Content-Encoding`; отдаём
  `Access-Control-Expose-Headers: X-Sentry-Rate-Limits, Retry-After`.
- Слэш в конце необязателен.

### Ключ проекта
Ищется по порядку: заголовок `X-Sentry-Auth` (`sentry_key=…`), query `sentry_key`,
`dsn` в заголовке envelope. Ключ должен принадлежать `project_id` из пути и быть активным.

Ключи держатся в памяти: полная перечитка активных ключей из Postgres каждые 30 с. Ключа нет в
кэше — сразу `403`, без похода в базу (иначе мусорные ключи нагружают Postgres). Новый ключ
начинает работать в пределах 30 с.

### Проверки и ответы
| Ситуация | Ответ |
|---|---|
| принято | `200 {"id":"<event_id>"}` |
| нет ключа / чужой / неактивный | `403` |
| Origin/Referer не в `allowed_origin` ключа (если список не пуст) | `403` |
| тело больше `INGEST_MAX_BODY_BYTES` (20 МБ) — до или после распаковки | `413` |
| событие больше `INGEST_MAX_EVENT_BYTES` (1 МБ) | событие отброшено, метрика; ответ `200` |
| битый envelope / JSON заголовка | `400` |
| превышен лимит ключа | `429`, `Retry-After: <сек>`, `X-Sentry-Rate-Limits: <сек>::project` |
| Redpanda недоступна | `503` |

- Распаковка: `gzip`, `deflate`, `br` (по `Content-Encoding`); лимит считается по распакованным байтам.
- Разрешённые сайты: хост из `Origin`, иначе из `Referer`; маски вида `*.mdev.kz`. Запрос без
  обоих заголовков (мобильные приложения, сервер) пропускается.
- Лимит ключа `rate_limit_per_sec` (0 — без лимита) — «ведро жетонов» в памяти **каждой копии**:
  общий предел = лимит × число копий. Это защитный предел от мусора, а не механизм «хранить часть».
- IP клиента: первый адрес из `X-Forwarded-For`, иначе `X-Real-IP`, иначе адрес соединения.
  Записывается в `user.ip_address` **только** если SDK прислал `{{auto}}` (семантика Sentry
  `sendDefaultPii`).

### Типы элементов envelope
- `event` → в Redpanda.
- Все прочие (`transaction`, `session`, `sessions`, `attachment`, `client_report`, `replay_*`,
  `profile`, `check_in`, `log`, `span`, `feedback`, `user_report`, …) — принимаются и
  выбрасываются; счётчик в метрике по типу. Ответ всё равно `200`.

### Сообщение в Redpanda
Тема `KAFKA_TOPIC_EVENTS` (по умолчанию `zorko.events`), ключ — `event_id` (равномерно по
разделам), значение — JSON: `project_id`, `received_at`, `client_ip`, `payload` (сырые байты
события). Продюсер franz-go: `acks=all`, идемпотентный, сжатие zstd, `linger` 5 мс.
Ответ клиенту — после подтверждения записи брокером. Хранение в теме — 24 ч.

## 2. Обработка

Consumer group `KAFKA_CONSUMER_GROUP` (по умолчанию `zorko-processor`); копии делят разделы.
Пачка — до `PROCESSOR_BATCH_SIZE` (500) записей или `PROCESSOR_BATCH_WAIT` (1 с).

Для пачки:
1. **Разбор и нормализация** каждого события: `event_id` (нет — генерируем), время, уровень
   (по умолчанию `error`), платформа, релиз, окружение (по умолчанию `production`), пользователь,
   контексты (браузер, ОС, устройство), исключения со стеками, `message`/`logentry`, теги.
   Время события вне окна `[received_at − 30 дн, received_at + 1 мин]` заменяется на
   `received_at` (кривые часы на устройствах).
2. **Вычистка**: обход всего JSON события; значения ключей, содержащих (без учёта регистра)
   `password`, `passwd`, `secret`, `token`, `api_key`, `apikey`, `auth`, `cookie`, `session`,
   `csrf`, `card`, `cvv` → `[Filtered]`; строки с номером карты (13–19 цифр, проверка Луна) →
   `[Filtered]`.
3. **Отпечаток** (раздел 3) и заголовок.
4. **Проблемы в Postgres**: сгруппировать пачку по `(project_id, hash)`; найти `issue_id` через
   `issue_hash` одним запросом; для новых — завести `issue` + `issue_hash` в транзакции (гонка
   двух копий: вставка `issue_hash … on conflict do nothing`; проиграли — откат и повторный поиск).
   Затем одним запросом (с блокировкой строк) обновить у проблем `times_seen`, `last_seen`,
   `last_release` и счётчик минуты и получить, сколько полных записей можно сохранить.
5. **ClickHouse**: одна вставка в `event_stat` (все события пачки) и одна в `event` (только
   разрешённые). При ошибке ClickHouse повторяется **только шаг 5** с нарастающей паузой, без
   повторного шага 4.
6. **Подтверждение** смещений в Redpanda — после шагов 4 и 5.

Битое сообщение (не разбирается JSON, нет `project_id`) — лог, метрика, пропуск.
Недоступность Postgres/ClickHouse — ждём и повторяем, смещения не подтверждаем.

## 3. Отпечаток и заголовок

Версия алгоритма — `v1` (хранится в `issue_hash`, чтобы алгоритм можно было менять).
`hash = hex(sha256(части через "\x00"))[:32]`.

1. Есть `fingerprint` от SDK → его элементы; `{{ default }}` заменяется частями по умолчанию.
2. Есть исключения со стеком → для каждого исключения цепочки: `type` + кадры «своего кода»;
   кадр = `module`, иначе нормализованное имя файла, + `function`. Номера строк и колонок не
   участвуют.
   - «Свой код»: `in_app` от SDK; если не задан — нет, когда путь содержит `node_modules`,
     начинается с `chrome-extension://`, `moz-extension://`, `safari-extension://`, или это
     `<anonymous>`/`[native code]`; иначе да. Своих кадров нет совсем — берутся все.
   - Нормализация имени файла: без схемы и хоста, без query и `#…`; хеш сборки убирается
     (`app.3f9a1c.js` → `app.js`, `index-BhK3xyz1.js` → `index.js`).
   - Пустые и анонимные функции (`?`, `<anonymous>`) не участвуют.
3. Иначе → `type` (если есть) + нормализованный текст: исключения или `logentry.message`
   (шаблон без подстановки `params`, если он есть), иначе `message`. Нормализация текста:
   uuid → `<uuid>`, почта → `<email>`, ссылки → `<url>`, IPv4 → `<ip>`, hex от 8 символов →
   `<hex>`, числа → `<int>`.

**Заголовок:** `Type: первая строка value` (до 200 символов); без исключения — первая строка
сообщения; иначе `<без заголовка>`.
**Место падения (culprit):** `transaction` события; иначе верхний свой кадр
`<файл> в <функция>`; иначе путь из `request.url`.

**Ограничение до шага 3:** без карт исходников имена функций в сжатом коде меняются от сборки
к сборке, и одна ошибка после релиза может завестись новой проблемой.

## 4. Хранение

### Postgres (`migrations/000001_init`)
- `org` — `id`, `created_at`, `slug` (уникальный), `name`.
- `project` — `id`, `created_at`, `org_id` → `org`, `slug` (уникален в организации), `name`,
  `platform`.
- `project_key` — `id`, `created_at`, `project_id` → `project`, `public_key` (32 hex,
  уникальный), `label`, `active`, `rate_limit_per_sec`, `allowed_origin text[]`.
- `issue` — `id`, `created_at`, `project_id` → `project`, `title`, `culprit`, `type`, `value`,
  `level`, `platform`, `status` (`unresolved`), `first_seen`, `last_seen`, `times_seen`,
  `first_release`, `last_release`, `stored_minute`, `stored_in_minute`.
  Индексы: `(project_id, last_seen desc)`, `(project_id, status, last_seen desc)`.
- `issue_hash` — `project_id`, `hash`, `version`, `issue_id` → `issue`; PK `(project_id, hash)`.
- Удаление проекта каскадом удаляет его ключи и проблемы; события в ClickHouse уходят по TTL.

### ClickHouse (`migrations-ch/000001_init`)
Обе таблицы: `ReplacingMergeTree`, `PARTITION BY toYYYYMMDD(received_at)`,
`TTL received_at + INTERVAL 30 DAY`, `ORDER BY (project_id, issue_id, timestamp, event_id)`,
так что повторная обработка не задваивает строки после слияний.
- `event_stat` — каждое событие коротко: `project_id`, `issue_id`, `event_id`, `timestamp`,
  `received_at`, `level`, `environment`, `release`, `user_key`, `tags Map(String, String)`.
  `user_key` — `user.id`, иначе почта, имя, IP, иначе пусто.
- `event` — полные записи: те же ключевые поля + `platform`, `title`, `culprit`, `message`,
  `exception_type`, `exception_value`, `transaction`, `user_id`, `user_email`, `user_ip`,
  `browser`, `os`, `device`, `sdk_name`, `sdk_version`, `data String CODEC(ZSTD(3))` (всё
  нормализованное событие). Индекс пропуска `bloom_filter` по `event_id`.
- Теги: из события + производные `level`, `environment`, `release`, `browser`, `browser.name`,
  `os`, `os.name`, `device`, `url`, `handled`, `mechanism`, `runtime`. Не больше 200 тегов,
  ключ до 32 символов, значение до 200.

Миграции ClickHouse — golang-migrate (драйвер `clickhouse`), применяются при старте, как Postgres.

## 5. Админский API

gRPC + HTTP-шлюз по соглашениям шаблона (единственное число путей, `PUT` для update,
пагинация с 0): `org`, `project`, `project_key` — list/get/create/update/delete.
При создании ключа `public_key` генерируется. В ответе ключа — готовый адрес:
`dsn = <схема>://<public_key>@<хост INGEST_PUBLIC_URL>/<project_id>`.
Без входа; наружу через Ruto не публикуется до шага 5.

## 6. Конфигурация (env)

| Переменная | По умолчанию | Что |
|---|---|---|
| `INGEST_ENABLED` | `true` | запускать приём |
| `INGEST_PORT` | `8000` | порт приёма |
| `INGEST_PUBLIC_URL` | — | внешний адрес приёма для DSN |
| `INGEST_MAX_BODY_BYTES` | `20971520` | предел тела (до и после распаковки) |
| `INGEST_MAX_EVENT_BYTES` | `1048576` | предел одного события |
| `PROCESSOR_ENABLED` | `true` | запускать обработку |
| `PROCESSOR_BATCH_SIZE` | `500` | размер пачки |
| `PROCESSOR_BATCH_WAIT` | `1s` | максимум ожидания пачки |
| `ISSUE_EVENT_STORE_PER_MINUTE` | `100` | полных записей в минуту на проблему |
| `KAFKA_BROKERS` | — | адреса Redpanda через запятую |
| `KAFKA_TOPIC_EVENTS` | `zorko.events` | тема событий |
| `KAFKA_CONSUMER_GROUP` | `zorko-processor` | группа обработчиков |
| `CH_DSN` | — | ClickHouse |
| `PG_DSN` | — | Postgres (как в шаблоне) |

## 7. Метрики (Prometheus, `/metrics`)

- `ingest_event_total{result}` — `accepted`, `rate_limited`, `forbidden`, `too_large`, `bad_request`, `unavailable`.
- `ingest_item_dropped_total{type}` — выброшенные элементы envelope по типу.
- `ingest_produce_seconds` — время записи в Redpanda.
- `processor_event_total{result}` — `stored`, `counted_only`, `bad_message`.
- `processor_batch_seconds{stage}` — `pg`, `ch`; `processor_lag` — отставание группы.

## 8. Проверка

- Автотесты (testify): разбор envelope/store на образцах настоящих SDK (`@sentry/browser`,
  `@sentry/vue`, `@sentry/node`, `sentry_flutter`, `@sentry/react-native`), распаковка и лимиты,
  нормализация, отпечаток (табличные тесты: одна ошибка из двух мест → две проблемы; один
  стек в двух сборках с разными хешами файлов → одна), вычистка.
- Интеграционно на стенде в docker (Redpanda, Postgres 17, ClickHouse):
  - `@sentry/browser` (страница в Chrome) и `@sentry/node` → события в базах, правильные
    заголовок, место падения, теги;
  - 1000 одинаковых ошибок → одна проблема, `times_seen = 1000`;
  - наплыв 100 тыс. одинаковых → `times_seen` ≈ 100 тыс. (допустим редкий перебор при сбое),
    полных записей ≤ 100 в минуту;
  - остановка ClickHouse посреди потока → после запуска всё дописано, ничего не потеряно;
  - мусорный ключ, чужой Origin, сжатая бомба, превышение лимита → 403/403/413/429.
- Нагрузка: одна копия на ноутбуке — не меньше 5000 событий/с на приёме, задержка ответа p99
  до 50 мс.

## Критерии готовности шага

1. Фронтендер вписывает DSN из API zorko в `@sentry/vue` — его ошибки видны в базах как
   проблемы со счётчиками.
2. Все проверки раздела 8 проходят; стенд записан в `CLAUDE.md` проекта.
3. `gofmt -l`, `go vet ./...`, `go test ./...` чистые.
