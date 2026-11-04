package service

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Service struct {
	mu       sync.Mutex
	limiters map[int64]*rate.Limiter
}

func New() *Service {
	return &Service{limiters: map[int64]*rate.Limiter{}}
}

// Allow — можно ли принять n событий по ключу; при отказе — через сколько повторить (не меньше секунды).
// limitPerSec <= 0 — без предела. Запас («ведро») — одна секунда лимита.
func (s *Service) Allow(keyId int64, limitPerSec int32, n int) (bool, time.Duration) {
	if limitPerSec <= 0 {
		return true, 0
	}

	limiter := s.limiter(keyId, limitPerSec)

	now := time.Now()
	if limiter.AllowN(now, n) {
		return true, 0
	}

	// пачка больше всего запаса не пройдёт никогда: просим подождать секунду
	retryAfter := time.Second
	if n <= limiter.Burst() {
		r := limiter.ReserveN(now, n)
		retryAfter = max(r.DelayFrom(now), time.Second)
		r.CancelAt(now)
	}

	return false, retryAfter.Round(time.Second)
}

func (s *Service) limiter(keyId int64, limitPerSec int32) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()

	// новый ключ или лимит ключа поменяли в админке — новое ведро с полным запасом
	limiter, ok := s.limiters[keyId]
	if !ok || limiter.Burst() != int(limitPerSec) {
		limiter = rate.NewLimiter(rate.Limit(limitPerSec), int(limitPerSec))
		s.limiters[keyId] = limiter
	}

	return limiter
}
