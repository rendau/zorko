//go:build integration

package issue

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	eventRepoCh "github.com/rendau/zorko/internal/domain/event/repo/ch"
	eventService "github.com/rendau/zorko/internal/domain/event/service"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	issueRepoDb "github.com/rendau/zorko/internal/domain/issue/repo/db"
	issueService "github.com/rendau/zorko/internal/domain/issue/service"
	projectRepoDb "github.com/rendau/zorko/internal/domain/project/repo/db"
	projectService "github.com/rendau/zorko/internal/domain/project/service"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	sessionService "github.com/rendau/zorko/internal/domain/session/service"
	usrRepoDb "github.com/rendau/zorko/internal/domain/usr/repo/db"
	usrService "github.com/rendau/zorko/internal/domain/usr/service"
	"github.com/rendau/zorko/internal/errs"
	accessService "github.com/rendau/zorko/internal/service/access/service"
	searchqueryService "github.com/rendau/zorko/internal/service/searchquery/service"
	"github.com/rendau/zorko/internal/testutil"
)

func TestList(t *testing.T) {
	pool, conn := testutil.PgPool(t), testutil.ChConn(t)
	ctx := context.Background()

	projectId := testutil.NewProject(t, pool)
	var orgId int64
	require.NoError(t, pool.QueryRow(ctx, `select org_id from project where id = $1`, projectId).Scan(&orgId))

	issues := issueService.New(issueRepoDb.New(pool))
	write := eventService.New(eventRepoCh.New(conn))
	sessions := sessionService.New("s")
	access := accessService.New(sessions, usrService.New(usrRepoDb.New(pool)))
	uc := New(sessions, projectService.New(projectRepoDb.New(pool)), issues, eventService.NewRead(eventRepoCh.New(conn)), searchqueryService.New(), access)

	// люди: участник без списка (все проекты) и участник с чужим проектом
	otherProjectId := testutil.NewProject(t, pool)
	// пустой список с allProjects — все проекты; без него — ни одного (ADR-019)
	newUsr := func(projectIds []int64) int64 {
		var id int64
		require.NoError(t, pool.QueryRow(ctx, `insert into usr (username, project_ids, all_projects) values ($1, $2, $3) returning id`,
			"u-"+uuid.NewString(), projectIds, len(projectIds) == 0).Scan(&id))
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from usr where id = $1`, id) })
		return id
	}
	allUsrId, strangerId := newUsr([]int64{}), newUsr([]int64{otherProjectId})

	now := time.Now().UTC().Truncate(time.Millisecond)
	minute := now.Truncate(time.Minute)

	// три проблемы: «старая и частая», «новая и редкая», «Safari»
	mk := func(hash, title string, firstSeen time.Time) int64 {
		res, err := issues.Record(ctx, []*issueModel.Hit{{
			ProjectId: projectId, Hash: hash, HashVersion: "v1", Title: title, Culprit: "cart.js в " + hash,
			Count: 1, FirstSeen: firstSeen, LastSeen: firstSeen,
		}}, 100, minute)
		require.NoError(t, err)
		return res[0].IssueId
	}
	oldFrequent := mk("a", "TypeError: price", now.Add(-10*24*time.Hour))
	newRare := mk("b", "ApiError: 502", now.Add(-time.Hour))
	safari := mk("c", "TypeError: total", now.Add(-5*24*time.Hour))

	var evs []*eventModel.Event
	add := func(issueId int64, n int, browser string, ago time.Duration) {
		for i := range n {
			evs = append(evs, &eventModel.Event{
				EventId: strings.ReplaceAll(uuid.NewString(), "-", ""), ProjectId: projectId, IssueId: issueId,
				Timestamp: now.Add(-ago - time.Duration(i)*time.Second), ReceivedAt: now, Environment: "production",
				User: eventModel.User{Id: browser + string(rune('a'+i%3))},
				Tags: map[string]string{"browser.name": browser, "environment": "production"}, Data: []byte(`{}`),
			})
		}
	}
	add(oldFrequent, 10, "Chrome", 2*time.Hour)
	add(newRare, 2, "Chrome", 10*time.Minute)
	add(safari, 4, "Safari", time.Hour)
	require.NoError(t, write.Store(ctx, evs, nil))

	authCtx := sessions.WithContext(ctx, &sessionModel.Session{Id: allUsrId})
	strangerCtx := sessions.WithContext(ctx, &sessionModel.Session{Id: strangerId})
	list := func(sort, q string) []int64 {
		rep, err := uc.List(authCtx, &ListReq{
			ListParams: commonModel.ListParams{PageSize: 50},
			Scope:      Scope{OrgId: orgId},
			Sort:       sort, Query: q,
		})
		require.NoError(t, err, sort+" "+q)
		ids := make([]int64, 0, len(rep.Items))
		for _, it := range rep.Items {
			require.Len(t, it.Sparkline, sparklineBuckets)
			ids = append(ids, it.Issue.Id)
		}
		return ids
	}

	assert.Equal(t, []int64{oldFrequent, safari, newRare}, list("events", ""))
	assert.Equal(t, []int64{newRare, safari, oldFrequent}, list("first_seen", ""), "новые проблемы первыми")
	assert.Equal(t, []int64{safari}, list("", "browser.name:Safari"))
	assert.Equal(t, []int64{oldFrequent, safari}, list("events", "TypeError"), "слово — по заголовку")
	assert.Equal(t, []int64{safari}, list("", "TypeError browser.name:Safari"))
	assert.Empty(t, list("", "nothing-like-this"))

	// чужая организация — пусто, не ошибка
	rep, err := uc.List(authCtx, &ListReq{ListParams: commonModel.ListParams{PageSize: 50}, Scope: Scope{OrgId: orgId, ProjectIds: []int64{999999999}}})
	require.NoError(t, err)
	assert.Empty(t, rep.Items)

	// страницы
	rep, err = uc.List(authCtx, &ListReq{ListParams: commonModel.ListParams{Page: 1, PageSize: 2}, Scope: Scope{OrgId: orgId}, Sort: "events"})
	require.NoError(t, err)
	assert.Equal(t, int64(3), rep.Total)
	require.Len(t, rep.Items, 1)
	assert.Equal(t, newRare, rep.Items[0].Issue.Id)

	_, err = uc.List(ctx, &ListReq{ListParams: commonModel.ListParams{PageSize: 50}, Scope: Scope{OrgId: orgId}})
	assert.ErrorIs(t, err, errs.NotAuthorized)

	// участник с другим проектом: чужих проблем не видит ни списком, ни по номеру
	rep, err = uc.List(strangerCtx, &ListReq{ListParams: commonModel.ListParams{PageSize: 50}, Scope: Scope{OrgId: orgId}})
	require.NoError(t, err)
	assert.Empty(t, rep.Items)
	_, err = uc.Get(strangerCtx, oldFrequent, "24h", "")
	assert.ErrorIs(t, err, errs.NoPermission)
	_, err = uc.GetEvent(strangerCtx, oldFrequent, "latest")
	assert.ErrorIs(t, err, errs.NoPermission)

	// карточка и ряд
	got, err := uc.Get(authCtx, oldFrequent, "24h", "")
	require.NoError(t, err)
	assert.Equal(t, int64(10), got.Events)
	assert.Equal(t, int64(3), got.Users)

	points, err := uc.Stat(authCtx, oldFrequent, "7d", "day", "")
	require.NoError(t, err)
	var sum int64
	for _, p := range points {
		sum += p.Count
	}
	assert.Equal(t, int64(10), sum)
	assert.Equal(t, 0, points[0].Ts.In(time.Local).Hour(), "дни — по местному времени")
}
