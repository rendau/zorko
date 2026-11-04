package errs

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCode(t *testing.T) {
	assert.Equal(t, "object_not_found", Code(fmt.Errorf("get: %w", ObjectNotFound)))
	assert.Equal(t, "invalid_request", Code(ErrFull{Err: InvalidRequest, Desc: "bad phone"}))
	assert.Equal(t, "service_not_available", Code(errors.New("dial tcp: connection refused")))
	assert.Equal(t, "service_not_available", Code(ErrFull{Err: errors.New("raw"), Desc: "x"}), "не errs.Err — не в лейбл")

	assert.True(t, IsFailure(Code(errors.New("boom"))))
	assert.True(t, IsFailure(InvalidConfig.Error()))
	assert.False(t, IsFailure(ObjectNotFound.Error()))
	assert.False(t, IsFailure("task_not_found"), "свой код сервиса — отказ по делу")
}
