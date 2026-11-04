package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoginLimit(t *testing.T) {
	s := New(3, 50*time.Millisecond)

	for range 3 {
		assert.True(t, s.Allowed("ivan"))
		s.Fail("ivan")
	}
	assert.False(t, s.Allowed("ivan"), "3 неудачи — дальше нельзя")
	assert.True(t, s.Allowed("olga"), "другой ключ не затронут")

	time.Sleep(60 * time.Millisecond)
	assert.True(t, s.Allowed("ivan"), "окно прошло")
}
