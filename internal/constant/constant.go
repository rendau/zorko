package constant

const (
	ServiceName = "zorko"

	MaxPageSize = 1000
)

// Способ входа в интерфейс — AUTH_MODE (ADR-019)
const (
	// AuthModeLocal — свои учётки zorko: логин и пароль, токены подписывает JWT_SECRET
	AuthModeLocal = "local"
	// AuthModeOidc — внешний OpenID-провайдер: токен получает интерфейс, zorko проверяет подпись
	AuthModeOidc = "oidc"
	// AuthModeProxy — перед zorko стоит доверенный шлюз со своим входом: он уже проверил токен,
	// zorko только читает claims, подпись не проверяет
	AuthModeProxy = "proxy"
)

// Version, Commit, BuiltAt подставляются при сборке (Makefile, ldflags): что именно запущено —
// версия, полный SHA коммита, время сборки (пишутся в лог при старте).
var (
	Version = "dev"
	Commit  = ""
	BuiltAt = ""
)
