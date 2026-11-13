// Package model — вход MCP-клиентов по OAuth (ADR-020): клиент, подключение, токены
package model

import "time"

const (
	TokenKindCode    = "code"
	TokenKindAccess  = "access"
	TokenKindRefresh = "refresh"
)

const (
	ClientAuthNone        = "none"
	ClientAuthSecretPost  = "client_secret_post"
	ClientAuthSecretBasic = "client_secret_basic"
)

// Client — OAuth-клиент из динамической регистрации. В БД не хранится: его данные несёт сам
// client_id, подписанный zorko (service.ClientCodec).
type Client struct {
	Name         string
	RedirectUris []string
	AuthMethod   string
}

// Grant — подключение OAuth-клиента, разрешённое человеком на странице согласия. Его токены
// дают клиенту права этого человека (только чтение — других инструментов у MCP нет).
type Grant struct {
	Id            string
	UsrId         int64
	ClientId      string
	ClientName    string
	RedirectUri   string
	CodeChallenge string
	CreatedAt     time.Time
	ConnectedAt   *time.Time
	LastUsedAt    *time.Time
}

// Token — код авторизации, access- или refresh-токен подключения. Hash — sha256 токена; сам
// токен не хранится.
type Token struct {
	Hash      string
	GrantId   string
	Kind      string
	ExpiresAt time.Time
}
