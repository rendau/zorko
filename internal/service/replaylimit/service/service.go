package service

import (
	"sync"
	"time"
)

type counter struct {
	hour time.Time
	n    int32
}

type Service struct {
	mu       sync.Mutex
	counters map[int64]*counter
}

func New() *Service {
	return &Service{counters: map[int64]*counter{}}
}

// Allow считает новую запись в текущем часе (UTC); limit <= 0 — без предела.
// При отказе — время до начала следующего часа, не меньше секунды.
func (s *Service) Allow(projectId int64, limit int32, now time.Time) (bool, time.Duration) {
	if limit <= 0 {
		return true, 0
	}
	hour := now.UTC().Truncate(time.Hour)

	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.counters[projectId]
	if !ok || !c.hour.Equal(hour) {
		// первая запись проекта в этом часе: заодно убрать счётчики прошлых часов (проектов немного)
		s.dropStale(hour)
		c = &counter{hour: hour}
		s.counters[projectId] = c
	}

	if c.n >= limit {
		return false, max(hour.Add(time.Hour).Sub(now), time.Second)
	}
	c.n++
	return true, 0
}

func (s *Service) dropStale(hour time.Time) {
	for id, c := range s.counters {
		if !c.hour.Equal(hour) {
			delete(s.counters, id)
		}
	}
}
