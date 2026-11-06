# Senior Engineer — replay

- **R1.** CH: `replay_segment` (ReplacingMergeTree по `(project_id, replay_id, segment_id)`,
  partition по дню `replay_start`, TTL 30 дней), `replay_issue`. PG: три колонки `project`.
  `s3.ListPrefixes(prefix)` — `ListObjects` с `Recursive:false`; `DeletePrefix` — `RemoveObjects`
  по списку `ListObjects(Recursive:true)`.
- **R2.** `Replay*` в `project.Main/Edit`, select/upsert, proto `ProjectMain`/`ProjectUpdateReq`,
  dto. Usecase: режим ∈ {off,error,session}, доля 0..1, предел 1..1e6; `mode != off` без S3 →
  `errs.ReplayDisabled`. `projectlabel` хранит `map[int64]ReplaySettings` рядом с метками.
- **R3.** Разбор: `envelopeItem` уже даёт тип и тело. Пара: первая `replay_event` + первая
  `replay_recording`. Тело записи: до `\n` — JSON `{"segment_id":N}`; дальше — `0x78…` (zlib) или
  `[` (JSON). `usecase.Req.Replay *RawReplay`; результат `Ingest` — `Result{ReplayStop time.Duration}`
  → handler ставит `X-Sentry-Rate-Limits: N:replay:project`. Порядок: события → запись.
  `replaylimit.Allow(projectId, limit, now)` — `map[int64]{hour, n}` под мьютексом.
- **R4.** Описание в очередь: `queueModel.Event{EventId: replayId+"/"+segment, Payload: replay_event,
  …}` + заголовки `s3_key`, `size` — та же модель очереди, без новой. `replayproc`: Poll → разбор
  JSON в `replay.Segment` (UA → браузер/ОС через normalize-функции, вынести экспортируемый
  разбор) → `InsertSegments` → Commit; повтор как в processor.
- **R5.** После `Record`: `lo.FilterMap(all, …)` по тегу `replayId` (hex 32 → UUID) → `Link`.
  Вставка в `eventService.Store`? Нет — отдельный `replayService.InsertLinks` внутри той же
  повторяемой стадии ClickHouse (сначала события, затем связи; повтор целиком безопасен —
  ReplacingMergeTree).
- **R6.** Тикер сутки (первый через 5 мин), `GcLock` через `pg_try_advisory_lock(<своя константа>)`;
  `ListPrefixes("replay/")` → дни `< now - retention - 24h` → `DeletePrefix`.
- **R7.** Список: `select replay_id, min(replay_start), max(timestamp), any(replay_type), …
  from replay_segment where project_id in (…) group by replay_id order by start desc limit/offset`
  — фильтры через `having`/`where` по полям сегмента 0. Сводка — то же для одного id +
  `count()` сегментов + связи. Сегмент: проверка доступа к проекту → S3 Get (предел 12 МБ) → тело.
- **R8.** `rrweb-player` монтируется в `onMounted`; события: распаковка
  `new Response(blob.stream().pipeThrough(new DecompressionStream('deflate'))).json()`; порции по
  20, `player.getReplayer().addEvent(e)` для следующих. Вкладки — по custom-событиям
  (`type 5`, `data.tag` = `breadcrumb` / `performanceSpan`).
- **R9.** `sdk.ts`: при `replay_mode != off` — `replayIntegration`, `replaysOnErrorSampleRate`,
  `replaysSessionSampleRate`. Подсказка CSP.
- **R10.** Образец в scratchpad: Vue + Vite + `@sentry/vue` + replay; Chrome через Claude-in-Chrome.
