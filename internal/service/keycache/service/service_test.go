package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectKeyModel "github.com/rendau/zorko/internal/domain/project_key/model"
)

type projectKeyServiceMock struct {
	items []*projectKeyModel.Main
	err   error
	pars  *projectKeyModel.ListReq
}

func (m *projectKeyServiceMock) List(_ context.Context, pars *projectKeyModel.ListReq) ([]*projectKeyModel.Main, int64, error) {
	m.pars = pars
	return m.items, int64(len(m.items)), m.err
}

func TestFind(t *testing.T) {
	mock := &projectKeyServiceMock{items: []*projectKeyModel.Main{
		{Id: 1, ProjectId: 10, PublicKey: "aaa", Active: true},
		{Id: 2, ProjectId: 20, PublicKey: "bbb", Active: true},
	}}
	s := New(mock, time.Minute)

	_, ok := s.Find(10, "aaa")
	assert.False(t, ok, "до загрузки ключей нет")

	require.NoError(t, s.Refresh(context.Background()))
	require.NotNil(t, mock.pars.Active)
	assert.True(t, *mock.pars.Active, "загружаются только активные")
	assert.Zero(t, mock.pars.PageSize, "все ключи, без страниц")

	key, ok := s.Find(10, "aaa")
	require.True(t, ok)
	assert.Equal(t, int64(1), key.Id)

	_, ok = s.Find(20, "aaa")
	assert.False(t, ok, "ключ чужого проекта")

	_, ok = s.Find(10, "zzz")
	assert.False(t, ok)
}

func TestRefreshErrorKeepsOldKeys(t *testing.T) {
	mock := &projectKeyServiceMock{items: []*projectKeyModel.Main{{Id: 1, ProjectId: 10, PublicKey: "aaa"}}}
	s := New(mock, time.Minute)
	require.NoError(t, s.Refresh(context.Background()))

	mock.err = errors.New("pg down")
	require.Error(t, s.Refresh(context.Background()))

	_, ok := s.Find(10, "aaa")
	assert.True(t, ok, "при сбое Postgres остаются прежние ключи")
}
