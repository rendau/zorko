# ADR-015: Проигрыватель — `@sentry-internal/rrweb-player` в общем бандле, сегменты сырыми zlib

**Status:** Accepted
**Date:** 2026-11-06
**Feature:** replay
**Author:** 10x-Team (Architect + Staff Engineer)

## Context
Записи SDK пишет форк rrweb от Sentry (`@sentry-internal/rrweb` 2.x). ADR-007 требует один бандл без
динамических подгрузок. Сегменты хранятся zlib; бинарное тело через grpc-gateway не пропускается.

## Decision
- Проигрыватель — `@sentry-internal/rrweb-player` (MIT) той же ветки, что SDK; входит в общий бандл.
  Прирост бандла фиксируется в `check-bundle` (новый порог — по факту сборки, R8).
- Ручка `GET /replay/{id}/segment/{n}` — обычный HTTP-обработчик на мультиплексоре шлюза
  (`HandlePath`), JWT из `Authorization` проверяется так же, как в gRPC; отдаёт zlib как есть.
  Браузер распаковывает `DecompressionStream('deflate')`.
- Запись проигрывается в рамке rrweb без скриптов.

## Alternatives Considered
| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Upstream `rrweb-player` | шире известен | расхождения с форком Sentry (события, сжатие) | риск несовместимости |
| Распаковка на сервере, JSON через gRPC | один транспорт | процессор сервера, большие JSON, base64 | браузер справится |
| Отдельный бандл проигрывателя | основной бандл не растёт | нарушает ADR-007 | пользователь просил один бандл |

## Consequences
### Positive
- Нагрузка распаковки — на браузере; сегменты кэшируются браузером.
### Negative
- Бандл растёт для всех страниц.
### Risks
- `DecompressionStream` нет в старых браузерах — интерфейс для современных браузеров, принимаем.

## Dependencies
ADR-007, ADR-012.
