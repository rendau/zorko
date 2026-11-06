# Architect — replay

ADR: 012 (S3 + отдельная тема), 013 (режимы, остановка с сервера), 014 (таблица связей),
015 (проигрыватель). Спека: `specs/2026-11-06-replay-design.md`.

## Компоненты
| Часть | Где |
|---|---|
| домен записей | `internal/domain/replay` — model (`Segment`, `Link`, `Summary`, `ListReq`), repo/ch (`replay_segment`, `replay_issue`), repo/blob (S3: сегменты, уборка папок дня), service |
| настройки | `domain/project` — поля `Replay*`; `service/projectlabel` — кэш `Replay(projectId)` |
| предел в час | `internal/service/replaylimit` — счётчик новых записей на проект по часам (память копии) |
| приём | `handler/ingest` — пара `replay_event` + `replay_recording`; `usecase/ingest` — проверки, режим, предел, S3, очередь |
| очередь | `service/queue` — вторая копия на `zorko.replays`; метки `topic` в метриках |
| обработка описаний | `internal/service/replayproc` — цикл: тема → разбор описания (браузер/ОС через normalize) → `replay_segment` пачкой |
| связи | `service/processor` — стадия `replay_link` после склейки, в той же записи ClickHouse |
| уборка | `internal/service/replaygc` — сутки, `pg_try_advisory_lock`, папки дня в S3 |
| API интерфейса | `usecase/replay` + `handler/grpc/replay.go` + `api/proto/.../replay.proto`; сегмент — `handler/replayseg` (HTTP на шлюзе) |
| фронт | `admin/` — `/replays`, `/replays/:id` (проигрыватель), блок в карточке проблемы, настройки проекта, примеры |

## Потоки
- Приём: ключ/сайт → события как прежде → для каждой пары: проверки → режим/тип/предел (иначе
  стоп-заголовок) → S3 put → очередь `zorko.replays` → 200.
- Описания: `replayproc` poll → разбор → `replay_segment` → commit.
- Ошибка: normalize → symbolicate → group → PG → ClickHouse (события + `replay_issue`).
- Просмотр: список/сводка из ClickHouse; сегменты — S3 через `handler/replayseg` → браузер
  распаковывает → rrweb-player.

## Отказы
- S3 / очередь записей недоступны → 503 на конверт с записью; ошибки в том же конверте уже
  опубликованы до записи? Нет: события публикуются первыми; при сбое записи ответ 503 и SDK
  повторит весь конверт — события дедуплицируются по `event_id` (ReplacingMergeTree), счётчик
  проблемы может прибавиться дважды. Чтобы не так: конверт записи у SDK всегда отдельный
  (только `replay_event` + `replay_recording`), поэтому смешанного случая на практике нет.
- ClickHouse недоступен для описаний → `replayproc` повторяет пачку, не подтверждая.
- Без S3 → записи отбрасываются со стоп-заголовком, режим в проекте не включить.
