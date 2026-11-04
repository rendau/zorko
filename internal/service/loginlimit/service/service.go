package service

import (
	"sync"
	"time"
)

type Service struct {
	maxFailures int
	window      time.Duration

	mu       sync.Mutex
	failures map[string][]time.Time
}

// New — не больше maxFailures неудачных входов за window по одному ключу
func New(maxFailures int, window time.Duration) *Service {
	return &Service{
		maxFailures: maxFailures,
		window:      window,
		failures:    map[string][]time.Time{},
	}
}

func (s *Service) Allowed(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.recent(key, time.Now())) < s.maxFailures
}

func (s *Service) Fail(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.failures[key] = append(s.recent(key, now), now)

	// карта не растёт бесконечно от перебора разных логинов
	if len(s.failures) > 10_000 {
		for k := range s.failures {
			if len(s.recent(k, now)) == 0 {
				delete(s.failures, k)
			}
		}
	}
}

// recent — неудачи по ключу в окне; вызывается под mu
func (s *Service) recent(key string, now time.Time) []time.Time {
	list := s.failures[key]
	i := 0
	for i < len(list) && now.Sub(list[i]) > s.window {
		i++
	}
	if i == len(list) {
		delete(s.failures, key)
		return nil
	}
	list = list[i:]
	s.failures[key] = list
	return list
}
