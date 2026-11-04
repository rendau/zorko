package loginlimit

// LoginLimiter — защита от перебора паролей: неудачные входы по ключу (логин, IP) за минуту
type LoginLimiter interface {
	Allowed(key string) bool
	Fail(key string)
}
