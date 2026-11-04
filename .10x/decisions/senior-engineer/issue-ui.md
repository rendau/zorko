# Senior Engineer — issue-ui

- **U1:** сессия — `domain/session/service` как в kusec (`CreateToken`, `CreateRefreshToken`,
  `FromToken`, `ParseRefreshToken`, контекст). Перехватчик ставится в цепочку до error-интерсептора;
  без токена — пустая сессия (решает usecase). Перебор: `ratelimit` по `login:<username>` и
  `login-ip:<ip>` (IP из metadata `x-forwarded-for`).
- **U2:** токенайзер с кавычками и `!`; результат — `[]Cond{Key, Op(eq|ne|like|nlike), Value}` и `Words`.
- **U3:** условия тегов → `tags[?] = ?` / `like ?`; `level`/`environment`/`release` — колонки.
  Соседи: `(timestamp, event_id) < (?, ?) order by timestamp desc, event_id desc limit 1`.
  Мини-график: `toStartOfInterval(timestamp, interval N second)` и заполнение пустых интервалов в Go.
- **U5:** по образцу `kusec/internal/app/spa.go`; `/_nuxt/*` и прочие файлы — сутки, `index.html` — `no-cache`.
- **U6:** `nuxt.config.ts` → `vite.build.rollupOptions.output.inlineDynamicImports`,
  `vite.build.cssCodeSplit=false`, `experimental.payloadExtraction=false`; проверка числа файлов
  в `scripts/check-bundle.mjs`.
- **U8:** разбор события — `utils/event.ts`, чистые функции с `node --test`.
