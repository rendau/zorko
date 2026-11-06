package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAllow(t *testing.T) {
	s := New()
	now := time.Date(2026, 11, 6, 10, 59, 30, 0, time.UTC)

	assert.True(t, first(s.Allow(1, 2, now)))
	assert.True(t, first(s.Allow(1, 2, now)))

	ok, retry := s.Allow(1, 2, now)
	assert.False(t, ok, "предел часа исчерпан")
	assert.Equal(t, 30*time.Second, retry, "до начала следующего часа")

	assert.True(t, first(s.Allow(2, 2, now)), "у другого проекта свой счётчик")
	assert.True(t, first(s.Allow(1, 0, now)), "0 — без предела")

	next := now.Add(time.Minute)
	assert.True(t, first(s.Allow(1, 2, next)), "новый час — счёт заново")
	assert.Len(t, s.counters, 1, "счётчики прошлого часа убраны")
}

func first(ok bool, _ time.Duration) bool { return ok }
