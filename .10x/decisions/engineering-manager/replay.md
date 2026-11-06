# Engineering Manager — replay

| # | Задача | Зависит от |
|---|---|---|
| R1 | миграции PG `000003` и CH `000002`; конфиг (`KAFKA_TOPIC_REPLAYS`, `REPLAY_MAX_SEGMENT_BYTES`, `REPLAY_RETENTION`); `infra/s3` `ListPrefixes`/`DeletePrefix` | — |
| R2 | поля проекта (model/repo/proto/dto/usecase, проверки: без S3 режим не включить); `projectlabel.Replay` | R1 |
| R3 | `handler/ingest`: пары частей, проверки; `usecase/ingest`: режим, тип, `replaylimit`, стоп-заголовок; тесты | R2 |
| R4 | `domain/replay` (model, repo/blob, repo/ch, service); S3 + очередь в приёме; вторая копия очереди с меткой `topic`; `service/replayproc` → `replay_segment`; тесты | R1, R3 |
| R5 | стадия связей в `processor` → `replay_issue`; тесты (связь и для «только посчитанных») | R4 |
| R6 | `service/replaygc`; тест | R4 |
| R7 | `usecase/replay`, proto `replay.proto`, `handler/grpc/replay.go`, `handler/replayseg`; `GET /issue/{id}/replay`; тесты доступа | R4, R5 |
| R8 | фронт: `/replays`, `/replays/:id` (rrweb-player, шкала, вкладки), меню | R7 |
| R9 | фронт: блок в карточке, кнопка у события, настройки проекта, примеры подключения, CSP; README | R7, R8 |
| R10 | вживую (Vue + `@sentry/vue` + replay, Chrome), нагрузка (`loadgen` сегменты), отчёты QA и безопасности | всё |

Риски: совместимость проигрывателя с форком rrweb (R8 — начать с прототипа на настоящей записи);
размер бандла (R8); реакция SDK на стоп-заголовок (R10).
