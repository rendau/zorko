# Staff Engineer — issue-ui

- Вход — копия подхода kusec (`session/service/service.go`, `usecase/usr`), без TOTP и API-ключей.
  Библиотеки: `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`.
- Новые сущности — skill `crud`; `usr` — таблица в единственном числе, как в kusec.
- Миграции: проект не задеплоен → `usr` дописывается в `000001_init` (правило «один init до деплоя»).
- Ошибки: `not_authorized`, `no_permission` (есть), новые `invalid_query`, `usr_username_exists`,
  `password_invalid`, `login_rate_limited` → ключи `apiError.*` в обе локали.
- Запросы ClickHouse — только параметрами (`?`), имена тегов — тоже параметром (`tags[?]`).
- Фронт — соглашения planora (`CLAUDE.md` planora, раздел «Админка»): фильтры в URL через
  `useQueryFilters`, `useApiAction` для действий, тексты только через i18n.
- Проверка фронта — только Claude-in-Chrome; Level C — скрипт `frontend-visual-qa`.
