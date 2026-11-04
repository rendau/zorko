package ratelimit

import "time"

// RateLimiter — защитный предел событий в секунду на ключ, в памяти копии приёма
type RateLimiter interface {
	Allow(keyId int64, limitPerSec int32, n int) (bool, time.Duration)
}
