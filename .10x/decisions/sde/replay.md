# SDE — replay (запись сессий)

Спека `specs/2026-11-06-replay-design.md`, ADR 012–016. Задачи R1–R10 — сделаны.

## Что построено
| Часть | Где |
|---|---|
| настройки | PG `000003_project_replay` (`replay_mode`, `replay_session_rate`, `replay_hour_limit`); `domain/project`, proto/dto; проверка в `usecase/project` (без S3 режим не включить — `errs.ReplayDisabled`); кэш `projectlabel.Replay` |
| приём | `handler/ingest/replay.go` (пара частей, строка-заголовок, zlib/JSON), `usecase/ingest` (`Ingest` → `Rep{ReplayResult, ReplayStop}`, режим/тип/предел, S3, очередь), заголовок `X-Sentry-Rate-Limits: N:replay:project` |
| предел в час | `service/replaylimit` |
| домен | `domain/replay` (model, repo/ch, repo/blob, repo/db — замок уборки, service) |
| очередь | вторая копия `service/queue` на `zorko.replays`; `queueModel.Event.BodySize`; метка `topic` у метрик отставания |
| описания | `service/replayproc` — описание через `normalize` (пользователь, браузер, релиз) + поля записи → `replay_segment` |
| связи | стадия в `service/processor` (`replayLinks`, тег `replayId`, для всех событий пачки) → `replay_issue` |
| уборка | `service/replaygc` — дни старше срока + сутки |
| API | `usecase/replay`, proto `replay.proto` (`/replay`, `/replay/{id}`, `/issue/{issue_id}/replay`), `handler/replayseg` (`/replay/{id}/segment/{n}` на `HandlePath` шлюза) |
| фронт | `pages/replays/{index,[id]}.vue`, `components/replay/{ReplayPlayer,ReplayItems,ReplayTable}.vue`, `settings/ReplayForm.vue`, вкладка «Записи» и «Смотреть запись» в карточке, `utils/replay.ts`, `api/replay.api.ts`, `sdk.ts` (replayIntegration в примерах) |
| нагрузка | `tools/loadgen -replay` |

## Отступления от спеки и плана
- Папка дня в S3 и TTL — по времени приёма (ADR-016), а не по началу записи.
- Проигрыватель стартует после загрузки всех сегментов (полоса загрузки), а не после первой порции:
  `addEvent` rrweb-player не обновляет длину шкалы. Для записей «при ошибке» (обычно 1–5 минут) это
  секунды; часовые записи — до ~15 МБ.
- SDK пишет часть своих событий (запросы, замеры, фокус) со временем в секундах — фронт приводит к мс.
- Проигрыватель начинает с первого Meta: служебные события SDK перед снимком давали пустую рамку.
- Фильтр «окружение» в разделе «Записи» не сделан (есть в ручке); поиск — по адресу и пользователю.
- `check-bundle` получил порог размера JS (1,6 МБ; сейчас 1,34 МБ, в brotli 311 КБ — +49 КБ за проигрыватель).

## Техдолг
- Предел в час — в памяти копии приёма (при N копиях — до N × предела).
- Ручка тела сегмента ходит в ClickHouse за днём на каждый сегмент (точечный запрос по первичному ключу).
