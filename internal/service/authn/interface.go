// Package authn — кто пришёл: сессия по токену из запроса в режиме AUTH_MODE (ADR-019).
//
//   - local — свой токен zorko (подпись JWT_SECRET, ADR-005);
//   - oidc  — токен внешнего OpenID-провайдера: подпись по его ключам, iss, exp, при настройке aud;
//   - proxy — токен, уже проверенный доверенным шлюзом перед zorko: claims читаются без проверки.
//
// Во внешних режимах человек находится в «Людях» по claim sub (или привязывается по логину) и
// заводится при первом входе без проектов; админ — по роли из токена (AUTH_ADMIN_ROLE).
package authn

import (
	"context"

	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
)

type Authenticator interface {
	// Authenticate — сессия по токену. Негодный токен — пустая сессия без ошибки (права решает
	// usecase); ошибка — сбой (БД, провайдер недоступен) или отключённый человек (errs.UsrDisabled).
	Authenticate(ctx context.Context, token string) (*sessionModel.Session, error)
}
