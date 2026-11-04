# CLAUDE.md

Руководство для Claude Code по работе с этим репозиторием. Go-сервис на Clean Architecture + DDD,
с gRPC/grpc-gateway, Postgres (pgx), proto-генерацией через Makefile.

Конвенции этого стека вынесены в глобальные Claude Code скиллы (`crud`, `mobone`,
`golang-service`, `golang-samber-lo`) — они подхватываются автоматически по описанию.
Ставятся из `github.com/rendau/go-service-skill` (`install.sh`); если их нет в списке скиллов —
предложи разработчику установить.

---

## Структура проекта

### Верхний уровень
- `cmd/main.go` — entrypoint, поднимает `internal/app.App`.
- `internal/` — бизнес-логика и инфраструктура (закрытые пакеты).
- `api/proto/` — исходные `.proto` данного сервиса.
- `pkg/proto/` — сгенерированный код protobuf/grpc/gateway (**не редактировать вручную**).
- `migrations/` — SQL миграции Postgres.
- `docs/` — swagger JSON и статические доки, выдаются через `/docs/*`.
- `vendor-proto/` — внешние `.proto` зависимости (`make refresh-vendor-proto` из `PROTOS_REPO`
  в `Makefile`; пустой — обновляются только вместе с шаблоном).
- `Dockerfile`, `Makefile` — сборка, запуск, генерация proto.
- `.env.example`, `.migrate_scripts.example` — примеры окружения и миграций.

### Внутренние пакеты (`internal/`)
- `internal/app/` — сборка приложения: серверы, миграции, метрики, трассировка, HTTP-gateway, DI.
  - `app.go` — граф зависимостей и запуск компонентов.
  - `grpc.go` — gRPC сервер + интерсепторы ошибок/метрик/трейсинга.
  - `grpc_gateway.go` — HTTP-gateway, CORS, error handler.
  - `system_http_server.go` — системный HTTP-сервер (порт 3003): /healthcheck, /docs/*, /metrics.
  - `migration.go` — запуск миграций из `migrations/`.
- `internal/config/` — конфигурация через env (см. `config.go`).
- `internal/handler/` — транспортный слой.
  - `grpc/` — gRPC handlers.
  - `grpc/dto/` — преобразование protobuf ↔ domain models; общие хелперы — `dto/common.go`
    (`DecodeListParams`, `JsonObjToGrpcStruct`, `GrpcStructToJsonObj`).
  - могут быть и другие транспортные каналы.
- `internal/usecase/` — usecase-слой (валидация, оркестрация сервисов и доменных сервисов).
- `internal/domain/` — доменная модель, сервисы и репозитории.
  - `*/model/` — доменные структуры (entity).
  - `*/service/` — доменные сервисы (инварианты/логика).
  - `*/repo/` — репозитории.
  - `common/` — общие модели/утилиты/PG базовый репозиторий.
- `internal/service/` — сервисы (фоновые/инфраструктурные), для переиспользования или выделения логики.
- `internal/errs/` и `internal/constant/` — общие коды ошибок и константы (`Version`, `Commit`,
  `BuiltAt` — из ldflags при сборке).

---

## Архитектура: слои и зависимости

- **Transport** (`internal/handler/grpc/*`):
  - Работает только с protobuf DTO и usecase-слоем: handler подключает `*usecase.Usecase` напрямую,
    без интерфейса (handler тонкий, отдельно не тестируется).
  - Не обращается напрямую к репозиториям и сервисам.
- **Usecase** (`internal/usecase/*`):
  - Входной слой от транспортного слоя (запросы от внешних систем).
  - Валидация входных параметров.
  - Оркестрация доменных сервисов и сервисов `internal/service/*`.
  - Вход/выход — доменные модели (не protobuf).
  - Зависимости (доменные сервисы, сервисы) принимает через интерфейсы, объявленные в своём
    пакете (`interfaces.go`).
  - Желательно не обращается в соседние usecases.
- **Domain** (`internal/domain/*`):
  - `model/` — структуры данных, сущности (entity).
    - **Запрещены теги сериализации** (`json:"..."`, `yaml:"..."` и т.п.) на полях доменной
      модели — она не знает про транспорт и про формат хранения.
  - `service/` — доменные операции и инварианты. Может использовать только репозиторий.
  - `repo/` — доступ к хранилищам. Может содержать подпапки для разных типов хранилищ или моков.
- **Service** (Infrastructure/background/external integrations, `internal/service/*`):
  - Фоновые процессы, интеграции с внешними системами.
  - Выделенные или переиспользуемые логики.
  - Может использовать другие сервисы `internal/service/*` и доменные сервисы `internal/domain/*/service`.
  - Не обращается в usecase слой.
- **Composition** (`internal/app/`):
  - Сборка зависимостей, запуск серверов, миграций, фоновых сервисов.

### Правило зависимостей
```
handler        → usecase
usecase        → domain service
usecase        → service
service        → service
service        → domain service
domain service → repo
```
- Обратные зависимости **запрещены**.
- К `repo` слою доступ только из `domain service`.

### Хранилища
- Postgres: все доменные entities (см. `migrations/*`).
- **Имена таблиц всегда в единственном числе**, без plural: `usr`, `app`, `secret`, `item`
  (не `usrs`, `apps`, `secrets`, `items`). То же значение указывается в `TableName` репозитория.

### Миграции
- Файлы в `migrations/` в формате `NNNNNN_<name>.up.sql` / `.down.sql` (golang-migrate).
- В `down`-миграциях во всех командах `DROP` обязательно указывать `CASCADE`
  (напр. `drop table if exists <table> cascade;`).
- В `down` объекты удаляются в порядке, обратном `up` (с учётом внешних ключей).

### API
- gRPC сервисы: `api/proto/{{ cookiecutter.__proto_pkg }}/*`.
- HTTP-gateway: через grpc-gateway + swagger (`docs/api.swagger.json`).
- DTO маппинг: `internal/handler/grpc/dto`.
- **REST-пути (route paths) всегда в единственном числе**, без plural:
  `/secret`, `/secret/{id}`, `/app`, `/item` (не `/secrets`, `/apps`, `/items`).
- **Пагинация: во всех `list` запросах `page` начинается с `0`** (нумерация страниц с нуля).
- **Для update использовать HTTP-метод `PUT`** (не `PATCH`): `put: "/<entity>/{id}"`.

### Ошибки и валидация
- Семантические ошибки — через `internal/errs` (см. gRPC interceptor в `internal/app/grpc.go`).
  Отказ по делу (не найдено, неверный запрос, неверный код) — всегда `errs.Err` (свой код сервиса
  или общий): такой ответ пишется в лог уровнем info и в метрике `request_total` получает свой
  `code` — это отказ, а не сбой. Любая другая ошибка — `service_not_available` (`errs.IsFailure`):
  лог уровнем error, сбой в метриках.
- Валидация параметров — в usecase.
- Нельзя пробрасывать ошибки наружу без wrapping (оборачивать в `fmt.Errorf("...: %w")`).
- Для работы с ошибками всегда используй `errors.Is` и `errors.AsType`. Избегай прямого
  сравнения ошибок (`==`) и type assertion (`err.(*MyError)`), чтобы корректно обрабатывать
  обёрнутые ошибки.

### Правила изменения кода
- gRPC DTO не должны протекать в доменные сервисы.
- `pkg/proto` и `docs/api.swagger.json` — генерируемые файлы (обновляются через Makefile).
- Генерация идёт только локальным тулчейном из `bin/` (в `.gitignore`): `make proto-tools` ставит
  protoc и плагины закреплённых версий (protoc — `PROTOC_VERSION` в Makefile, protoc-gen-go и
  grpc-gateway — версий из `go.mod`), `generate-proto` вызывает его сам. Глобальные protoc/плагины
  не используются; версии поднимаем правкой Makefile/`go.mod`, всей командой сразу.
- В тестах всегда предпочитай `testify`: `require` для проверок, прерывающих тест,
  и `assert` для остальных утверждений.
- При реализации worker pool / параллельной обработки используй `errgroup`
  (golang.org/x/sync/errgroup), а не ручное управление горутинами через `sync.WaitGroup` + каналы.

---

## Композиционный корень (`internal/app/app.go`)

`app.go` — единственная точка композиции приложения: здесь собирается граф зависимостей
(`repo → service → usecase → handler`) и описывается жизненный цикл. Бизнес-логики тут нет —
только связывание компонентов и управление их запуском/остановкой.

### Тип `App`
- В поля выносится **только то, чем нужно управлять после `Init`**: то, что надо явно
  останавливать (серверы), ждать (фоновые сервисы/обработчики), закрывать (трассировщик,
  pgx pool), а также корневой `ctx` с его `ctxCancel` и `exitCode`.
- Локальные звенья графа (repo, service, usecase, handler), которые нужны только для сборки и
  сразу передаются дальше, **не** выносятся в поля — это локальные переменные внутри `Init`.

### Импорты Композиционного корня
- Группируются блоками с пустой строкой между группами: стандартная библиотека → внешние
  зависимости → внутренние пакеты проекта.
- Внутренние пакеты-конструкторы импортируются с суффиксом-алиасом `P`, и в алиасе прописывается весь путь в camel-case
  (например, `internal/service/mdm` -> `serviceMdmP`, `internal/handler/grpc` -> `handlerGrpcP`, `internal/service/2gis/service` -> `service2gisServiceP`).

### Методы-фазы жизненного цикла
Фиксированный набор методов, каждый делает ровно одно:
- `Init` — создание и связывание всех зависимостей.
- `PreStartHook` — действия перед стартом.
- `Start` — запуск серверов и фоновых сервисов.
- `Listen` — блокировка до сигнала ОС (`SIGINT`/`SIGTERM`).
- `Stop` — отмена контекста и graceful-остановка серверов.
- `WaitJobs` — ожидание завершения фоновых задач.
- `Exit` — закрытие ресурсов и выход с `exitCode`.

### Стиль `Init`
- Сборка идёт **сверху вниз в порядке зависимостей**: инфраструктура (логгер, трассировка,
  pgx pool, кэш, миграции) → доменные блоки → сервисы → серверы.
- Каждый логический блок предваряется коротким комментарием-меткой в нижнем регистре
  (`// ord`, `// checkout`, `// grpc server`).
- Внутри блока цепочка строится единообразно:
  `repo := ...New(...)` → `service := ...New(repo)` → `usecase := ...New(service)` →
  `handler := ...New(usecase)`.
- Блоки, которым не нужны внешние переменные, оборачиваются в анонимный блок `{ ... }` —
  для ограничения области видимости и визуального разделения.

### Обработка ошибок при инициализации
- Используется хелпер `errCheck(err, msg)`: на этапе сборки любая ошибка фатальна
  (лог + `os.Exit(1)`). Ошибки из `Init` наверх не пробрасываются.
- Один заранее объявленный `var err error` переиспользуется по ходу `Init`.

### Парность Start / WaitJobs / Stop
- Для каждого фонового компонента, запускаемого в `Start()`, есть симметричный вызов в
  `WaitJobs()` (`.Wait()`) и/или в `Stop()`. **Порядок перечисления компонентов одинаков во
  всех трёх методах** — это упрощает чтение и сверку.

### Конфигурация и опциональные компоненты
- Все параметры берутся из единого глобального конфига (`config.Conf.*`) прямо в месте
  использования.
- Опциональные компоненты включаются по условию на конфиг (`if config.Conf.X != "" { ... }`),
  с фолбэком на in-memory/no-op реализацию через общий интерфейс.

---

## Runtime и конфигурация

### Запуск
- Entry: `cmd/main.go` → `internal/app.App`.
- На старте выполняются:
  - загрузка env (autoload `.env`),
  - настройка логгера/метрик/трейсинга,
  - pgx pool,
  - миграции (`internal/app/migration.go`),
  - запуск gRPC + HTTP-gateway.

### Переменные окружения
- Описаны в `internal/config/config.go`.
- Примеры: `.env.example`, `.migrate_scripts.example`.

### Системный HTTP-сервер
- Отдельный сервер на порту `3003` (`const systemHttpPort` в `internal/app/system_http_server.go`).
- Обслуживает служебные ручки: `/healthcheck`, `/docs/*`, `/metrics`.

### Интеграция с pulse
- **Необязательна и делается отдельно — только когда об этом просит разработчик.** Без такой
  просьбы pulse не подключать и под него не адаптировать: ни пакета `pulsekit`, ни манифеста, ни
  `/.well-known/pulse`, ни вопросов о команде и критичности.
- Когда разработчик попросил: стандарт, готовый пакет `internal/infra/pulsekit` и шаги подключения
  к сервису на этом шаблоне — в `github.com/rendau/pulse`, `docs/service-manifest.md` (раздел
  «Go: пакет pulsekit»); **сначала прочитай там раздел «Для агента»**.

### Метрики и трассировка
- Prometheus метрики на `/metrics` (системный сервер, порт 3003) при `WITH_METRICS=true`.
  Используется `metrics.Registry`, а не дефолтный `promhttp.Handler()`.
- `request_total{protocol, method, status, code}`: `status` — ok|error, `code` — ok или код
  ошибки ответа (`errs.Code`, набор конечен). Лейблы не убирать: по `code` сбои отличаются
  от отказов по делу.
- Трейсинг Jaeger включается при `WITH_TRACING=true` и `JAEGER_ADDRESS`.

### Документация и healthcheck
- `/healthcheck` — HTTP healthcheck (200 OK), порт 3003.
- `/docs/*` — статические docs + swagger (`docs/api.swagger.json`), порт 3003.

### Сборка
- `make build` создаёт бинарник `cmd/build/svc` (версия, полный коммит и время сборки — в ldflags,
  `internal/constant`; пишутся в лог при старте).
- Dockerfile копирует бинарник, `docs/` и `migrations/` в `/app`.

### Flow проверки изменений
```
make generate-proto  →  gofmt  →  go test ./...  →  go run ./cmd/.
```
