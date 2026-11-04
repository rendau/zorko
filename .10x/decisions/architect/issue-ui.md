# Architect — issue-ui

ADR: 005 (вход), 006 (список проблем), 007 (фронт и статика). Спека: `specs/2026-11-04-issue-ui-design.md`.

## Компоненты
| Часть | Где |
|---|---|
| пользователи | `internal/domain/usr` (CRUD + `AuthByUsernamePassword`, `HasAny`), `usecase/usr`, `handler/grpc/usr.go` |
| сессии (JWT) | `internal/domain/session` (model + service, без repo) |
| перехватчик | `internal/app/grpc.go` → `GrpcInterceptorSession` |
| права | usecase-и `org`, `project`, `project_key`, `usr` (+ новые) |
| поиск | `internal/service/searchquery` (разбор строки → условия) |
| проблемы: чтение | `domain/issue` (repo/db: `List` по id, `IdsByText`; ) + `domain/event/repo/ch` (агрегаты) |
| usecase/handler | `usecase/issue`, `usecase/event`, `usecase/tag`; `handler/grpc/{issue,event,tag}.go` |
| статика | `internal/app/admin_http_server.go` (+ `/config.js`) |
| фронт | `admin/` |

## Потоки
- Вход: `Login` → bcrypt → пара токенов; запрос → Bearer → перехватчик → сессия; 401 → refresh.
- Список: usecase → (PG ids по словам) → CH агрегат → PG строки → сортировка/страница → CH графики.
- Событие: CH `event` по `issue_id` + `event_id`; соседи — запросы `<`/`>` по `(timestamp, event_id)`.

## Отказы
- CH недоступен — список/карточка отдают `service_not_available`, проекты и люди работают.
- Неверный поиск — `invalid_query` (отказ по делу).
