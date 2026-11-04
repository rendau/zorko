# Engineering Manager — issue-ui

| # | Задача | Зависит от |
|---|---|---|
| U1 | `usr` (миграция, CRUD), `session` (JWT), перехватчик, `usecase/usr` (Login, Refresh, Bootstrap, Profile), права в org/project/project_key; тесты | — |
| U2 | `service/searchquery` + тесты | — |
| U3 | CH-запросы: агрегат списка, мини-графики, ряд, теги, ключи/значения тегов, события и соседи; интеграционные тесты | U2 |
| U4 | proto + usecase + handler: `issue`, `event`, `tag`; интеграционные тесты списка | U1, U3 |
| U5 | Раздача статики (`admin_http_server.go`, `config.js`, `.br/.gz`, кэш) + тесты | — |
| U6 | `admin/`: каркас (Nuxt, i18n, тема, один бандл, compress), `apiFetch`, вход/setup/refresh, меню | U1, U5 |
| U7 | Экран «Проблемы» (фильтры, поиск с подсказками, таблица, мини-графики) | U4, U6 |
| U8 | Карточка проблемы (шапка, график, теги, событие, вкладка событий) | U4, U6 |
| U9 | «Проекты» и «Люди» | U1, U6 |
| U10 | Проверка в Chrome + Level C + отчёты | всё |

Риск: сборка Nuxt в один файл (U6) — проверяется первой в U6.
