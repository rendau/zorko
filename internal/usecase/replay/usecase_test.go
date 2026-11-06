package replay

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	sessionService "github.com/rendau/zorko/internal/domain/session/service"
	"github.com/rendau/zorko/internal/errs"
	accessModel "github.com/rendau/zorko/internal/service/access/model"
)

const replayId = "a1b2c3d4e5f60718293a4b5c6d7e8f90"

type projectFake struct{}

func (projectFake) List(_ context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error) {
	// организация 1: проекты 10 и 11; организация 2: проект 20
	all := []*projectModel.Main{{Id: 10, OrgId: 1}, {Id: 11, OrgId: 1}, {Id: 20, OrgId: 2}}
	items := lo.Filter(all, func(p *projectModel.Main, _ int) bool { return pars.OrgId == nil || p.OrgId == *pars.OrgId })
	return items, int64(len(items)), nil
}

type issueFake struct{}

func (issueFake) Get(_ context.Context, id int64, _ bool) (*issueModel.Main, bool, error) {
	return &issueModel.Main{Id: id, ProjectId: id / 10}, true, nil // проблема 100 — проекта 10, 110 — проекта 11
}

func (issueFake) ListByIds(_ context.Context, ids []int64) ([]*issueModel.Main, error) {
	return lo.Map(ids, func(id int64, _ int) *issueModel.Main {
		return &issueModel.Main{Id: id, ProjectId: id / 10, Title: "TypeError: x"}
	}), nil
}

type replayFake struct {
	listPars *replayModel.ListReq
	links    []*replayModel.Link
}

func (f *replayFake) List(_ context.Context, pars *replayModel.ListReq) ([]*replayModel.Summary, int64, error) {
	f.listPars = pars
	return []*replayModel.Summary{}, 0, nil
}

func (f *replayFake) Get(_ context.Context, projectId int64, id string) (*replayModel.Summary, error) {
	return &replayModel.Summary{ProjectId: projectId, ReplayId: id}, nil
}

func (f *replayFake) IssueReplays(_ context.Context, _, _ int64, _, _ int64) ([]*replayModel.Summary, int64, error) {
	return []*replayModel.Summary{{ReplayId: replayId}}, 1, nil
}

func (f *replayFake) Links(_ context.Context, _ int64, _ string) ([]*replayModel.Link, error) {
	return f.links, nil
}

func (f *replayFake) SegmentBody(_ context.Context, _ int64, _ string, _ int, _ int64) ([]byte, error) {
	return []byte{0x78, 0x9c}, nil
}

// accessFake — участник видит только проект 10; админ — всё
type accessFake struct{ session *sessionService.Service }

func (a accessFake) ProjectAccess(ctx context.Context) (*accessModel.ProjectAccess, error) {
	s := a.session.FromContext(ctx)
	if !s.IsAuthorized() {
		return nil, errs.NotAuthorized
	}
	if s.IsAdmin() {
		return &accessModel.ProjectAccess{All: true}, nil
	}
	return &accessModel.ProjectAccess{Ids: []int64{10}}, nil
}

func (a accessFake) CheckProject(ctx context.Context, projectId int64) error {
	access, err := a.ProjectAccess(ctx)
	if err != nil {
		return err
	}
	if !access.Allows(projectId) {
		return errs.NoPermission
	}
	return nil
}

func newUsecase() (*Usecase, *replayFake, *sessionService.Service) {
	sessions := sessionService.New("secret")
	replays := &replayFake{}
	return New(sessions, projectFake{}, issueFake{}, replays, accessFake{session: sessions}, 1<<20), replays, sessions
}

func member(s *sessionService.Service) context.Context {
	return s.WithContext(context.Background(), &sessionModel.Session{Id: 2})
}

func TestAccess(t *testing.T) {
	uc, _, sessions := newUsecase()
	anon := context.Background()
	ctx := member(sessions)
	page := commonModel.ListParams{PageSize: 20}

	_, err := uc.Get(anon, 10, replayId)
	assert.ErrorIs(t, err, errs.NotAuthorized)
	_, err = uc.SegmentBody(anon, 10, replayId, 0)
	assert.ErrorIs(t, err, errs.NotAuthorized)

	_, err = uc.Get(ctx, 11, replayId)
	assert.ErrorIs(t, err, errs.NoPermission, "запись чужого проекта")
	_, err = uc.SegmentBody(ctx, 11, replayId, 0)
	assert.ErrorIs(t, err, errs.NoPermission, "тело сегмента чужого проекта")
	_, _, err = uc.IssueReplays(ctx, 110, page)
	assert.ErrorIs(t, err, errs.NoPermission, "записи проблемы чужого проекта")

	body, err := uc.SegmentBody(ctx, 10, replayId, 0)
	require.NoError(t, err)
	assert.Equal(t, byte(0x78), body[0])
	_, total, err := uc.IssueReplays(ctx, 100, page)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
}

func TestListOnlyVisibleProjects(t *testing.T) {
	uc, replays, sessions := newUsecase()
	ctx := member(sessions)

	_, _, err := uc.List(ctx, &ListReq{ListParams: commonModel.ListParams{PageSize: 20}, OrgId: 1})
	require.NoError(t, err)
	assert.Equal(t, []int64{10}, replays.listPars.ProjectIds, "участник — только свои проекты организации")

	replays.listPars = nil
	_, _, err = uc.List(ctx, &ListReq{ListParams: commonModel.ListParams{PageSize: 20}, OrgId: 1, ProjectIds: []int64{20}})
	require.NoError(t, err)
	assert.Nil(t, replays.listPars, "проект другой организации — пустой ответ без запроса")

	admin := sessions.WithContext(context.Background(), &sessionModel.Session{Id: 1, Admin: true})
	_, _, err = uc.List(admin, &ListReq{ListParams: commonModel.ListParams{PageSize: 20}, OrgId: 1})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{10, 11}, replays.listPars.ProjectIds)

	_, _, err = uc.List(admin, &ListReq{ListParams: commonModel.ListParams{PageSize: 20}})
	assert.Error(t, err, "без организации")
	_, _, err = uc.List(admin, &ListReq{OrgId: 1})
	assert.ErrorIs(t, err, errs.IncorrectPageSize)
}

func TestGetErrorsOnlyFromSameProject(t *testing.T) {
	uc, replays, sessions := newUsecase()
	now := time.Now()
	replays.links = []*replayModel.Link{
		{IssueId: 100, EventId: "e1", Timestamp: now},
		{IssueId: 110, EventId: "e2", Timestamp: now}, // проблема другого проекта — не показывается
	}

	rep, err := uc.Get(member(sessions), 10, replayId)
	require.NoError(t, err)
	require.Len(t, rep.Errors, 1)
	assert.EqualValues(t, 100, rep.Errors[0].IssueId)
	assert.Equal(t, "TypeError: x", rep.Errors[0].Title)
}
