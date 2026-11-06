package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectModel "github.com/rendau/zorko/internal/domain/project/model"
)

type projectServiceMock struct {
	items []*projectModel.Main
	err   error
	pars  *projectModel.ListReq
}

func (m *projectServiceMock) List(_ context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error) {
	m.pars = pars
	return m.items, int64(len(m.items)), m.err
}

func TestLabel(t *testing.T) {
	mock := &projectServiceMock{items: []*projectModel.Main{
		{Id: 1, OrgId: 1, Slug: "mechta"},
		{Id: 2, OrgId: 1, Slug: "admin"},
		{Id: 3, OrgId: 2, Slug: "admin"},
	}}
	s := New(mock, time.Minute)

	assert.Equal(t, LabelUnknown, s.Label(1), "до загрузки проектов нет")

	require.NoError(t, s.Refresh(context.Background()))
	require.NotNil(t, mock.pars)
	assert.Zero(t, mock.pars.PageSize, "все проекты, без страниц")

	assert.Equal(t, "mechta", s.Label(1))
	assert.Equal(t, "admin-2", s.Label(2), "одинаковый slug в разных организациях")
	assert.Equal(t, "admin-3", s.Label(3))
	assert.Equal(t, LabelUnknown, s.Label(999), "номер из чужого запроса")
	assert.Equal(t, LabelUnknown, s.Label(0))
}

func TestRefreshErrorKeepsOldLabels(t *testing.T) {
	mock := &projectServiceMock{items: []*projectModel.Main{{Id: 1, Slug: "mechta"}}}
	s := New(mock, time.Minute)
	require.NoError(t, s.Refresh(context.Background()))

	mock.err = errors.New("pg down")
	require.Error(t, s.Refresh(context.Background()))
	assert.Equal(t, "mechta", s.Label(1))
}

func TestReplaySettings(t *testing.T) {
	mock := &projectServiceMock{items: []*projectModel.Main{
		{Id: 1, Slug: "seller", ReplayMode: projectModel.ReplayModeError, ReplayHourLimit: 500},
		{Id: 2, Slug: "admin", ReplayMode: projectModel.ReplayModeOff, ReplayHourLimit: 1000},
	}}
	s := New(mock, time.Minute)

	assert.Equal(t, projectModel.ReplayModeOff, s.Replay(1).Mode, "до загрузки запись выключена")

	require.NoError(t, s.Refresh(context.Background()))
	assert.Equal(t, projectModel.ReplayModeError, s.Replay(1).Mode)
	assert.EqualValues(t, 500, s.Replay(1).HourLimit)
	assert.Equal(t, projectModel.ReplayModeOff, s.Replay(2).Mode)
	assert.Equal(t, projectModel.ReplayModeOff, s.Replay(999).Mode, "неизвестный проект")
}
