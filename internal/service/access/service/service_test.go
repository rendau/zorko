package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	sessionService "github.com/rendau/zorko/internal/domain/session/service"
	usrModel "github.com/rendau/zorko/internal/domain/usr/model"
	"github.com/rendau/zorko/internal/errs"
)

type usrFake map[int64]*usrModel.Main

func (f usrFake) Get(_ context.Context, id int64, _ bool) (*usrModel.Main, bool, error) {
	v, ok := f[id]
	return v, ok, nil
}

func TestProjectAccess(t *testing.T) {
	sessions := sessionService.New("test-secret")
	s := New(sessions, usrFake{
		1: {Id: 1, Active: true, IsAdmin: true, ProjectIds: []int64{5}},
		2: {Id: 2, Active: true, AllProjects: true},
		5: {Id: 5, Active: true},
		3: {Id: 3, Active: true, ProjectIds: []int64{5, 7}},
		4: {Id: 4, Active: false},
	})
	as := func(id int64) context.Context {
		return sessions.WithContext(context.Background(), &sessionModel.Session{Id: id})
	}

	_, err := s.ProjectAccess(context.Background())
	assert.ErrorIs(t, err, errs.NotAuthorized, "без входа")

	a, err := s.ProjectAccess(as(1))
	require.NoError(t, err)
	assert.True(t, a.All, "админу список не ограничивает")

	a, err = s.ProjectAccess(as(2))
	require.NoError(t, err)
	assert.True(t, a.All, "все проекты")

	a, err = s.ProjectAccess(as(5))
	require.NoError(t, err)
	assert.False(t, a.All)
	assert.Empty(t, a.Filter([]int64{1, 5}), "пустой список — ни одного проекта")

	a, err = s.ProjectAccess(as(3))
	require.NoError(t, err)
	assert.False(t, a.All)
	assert.Equal(t, []int64{5, 7}, a.Filter([]int64{1, 5, 6, 7}))
	assert.True(t, a.AllowsAny([]int64{1, 7}))
	assert.False(t, a.AllowsAny([]int64{1, 2}))
	assert.NoError(t, s.CheckProject(as(3), 7))
	assert.ErrorIs(t, s.CheckProject(as(3), 6), errs.NoPermission)

	_, err = s.ProjectAccess(as(4))
	assert.ErrorIs(t, err, errs.NotAuthorized, "отключённый — сразу, не дожидаясь нового токена")
	_, err = s.ProjectAccess(as(99))
	assert.ErrorIs(t, err, errs.NotAuthorized, "удалённый")
}
