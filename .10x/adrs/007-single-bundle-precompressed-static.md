# ADR-007: Фронт — один бандл, предсжатая статика, кэш на сутки

**Status:** Accepted
**Date:** 2026-11-04
**Feature:** issue-ui
**Author:** 10x-Team (Architect + DevOps)

## Context
Требования пользователя: один бандл без динамических подгрузок; сжатие как в ruto/kusec; кэш в
браузере на сутки. Новый релиз должен доходить до людей без ожидания суток.

## Decision
- Nuxt 4 (`ssr: false`), `inlineDynamicImports`, `cssCodeSplit: false` → один JS и один CSS с хешем
  в имени. Запасной путь — Nuxt UI на Vite + Vue.
- `compress.mjs` пишет `.br`/`.gz` при сборке; Go отдаёт копию по `Accept-Encoding`, сам не сжимает.
- `Cache-Control: public, max-age=86400` для файлов сборки; `index.html` и адреса SPA — `no-cache`.

## Alternatives Considered
| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Сжатие на лету (gzip middleware) | проще сборка | CPU на каждый запрос, хуже степень | не как в ruto/kusec |
| Всё на сутки, включая `index.html` | меньше запросов | релиз доходит через сутки | пользователь выбрал проверку `index.html` |
| Разбиение на чанки по страницам | быстрее первая загрузка | отклонено пользователем | — |

## Consequences
### Positive
- Предсказуемая загрузка, без «догрузки»; сжатие без нагрузки на CPU.
### Negative
- Первая загрузка — весь интерфейс сразу (ожидаемо ~300–500 КБ в br).
### Risks
- Nuxt внутри может создавать отдельные чанки → проверка «ровно один JS» в сборке.
