package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAllow(t *testing.T) {
	s := New()

	ok, _ := s.Allow(1, 0, 1_000_000)
	assert.True(t, ok, "лимит 0 — без предела")

	for range 10 {
		ok, _ = s.Allow(1, 10, 1)
		assert.True(t, ok)
	}

	ok, retryAfter := s.Allow(1, 10, 1)
	assert.False(t, ok, "запас в секунду исчерпан")
	assert.GreaterOrEqual(t, retryAfter, time.Second)

	ok, _ = s.Allow(2, 10, 1)
	assert.True(t, ok, "у другого ключа свой запас")

	ok, retryAfter = s.Allow(3, 10, 100)
	assert.False(t, ok, "пачка больше запаса")
	assert.Equal(t, time.Second, retryAfter)
}

func TestLimitChanged(t *testing.T) {
	s := New()

	ok, _ := s.Allow(1, 1, 1)
	assert.True(t, ok)
	ok, _ = s.Allow(1, 1, 1)
	assert.False(t, ok)

	ok, _ = s.Allow(1, 100, 50)
	assert.True(t, ok, "лимит подняли — новый запас")
}
