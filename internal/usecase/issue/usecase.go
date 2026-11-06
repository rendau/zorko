package issue

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/util"
)

const (
	defaultListPeriod  = 14 * 24 * time.Hour
	defaultEventPeriod = 30 * 24 * time.Hour
	sparklineBuckets   = 24
	maxTextMatches     = 5000
	tagTopValues       = 5
	tagKeysLimit       = 100
	tagValuesLimit     = 20
	maxPageSize        = 100
)

type Usecase struct {
	sessionSvc SessionServiceI
	projectSvc ProjectServiceI
	issueSvc   IssueServiceI
	eventSvc   EventReadServiceI
	parser     QueryParserI
	accessSvc  AccessServiceI
}

func New(sessionSvc SessionServiceI, projectSvc ProjectServiceI, issueSvc IssueServiceI, eventSvc EventReadServiceI, parser QueryParserI, accessSvc AccessServiceI) *Usecase {
	return &Usecase{sessionSvc: sessionSvc, projectSvc: projectSvc, issueSvc: issueSvc, eventSvc: eventSvc, parser: parser, accessSvc: accessSvc}
}

// List — проблемы с событиями за период (ADR-006): агрегат ClickHouse + строки Postgres,
// сортировка и страница в Go, мини-графики только для страницы
func (u *Usecase) List(ctx context.Context, req *ListReq) (*ListRep, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, errs.NotAuthorized
	}
	if err := util.RequirePageSize(req.ListParams, maxPageSize); err != nil {
		return nil, err
	}
	if !slices.Contains([]string{"", "last_seen", "first_seen", "events", "users"}, req.Sort) {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "sort: last_seen, first_seen, events, users", Fields: map[string]string{"sort": "invalid"}}
	}

	f, words, err := u.filter(ctx, req.Scope, req.Query, defaultListPeriod)
	if err != nil {
		return nil, err
	}
	empty := &ListRep{Items: []*ListItem{}}
	if len(f.ProjectIds) == 0 {
		return empty, nil
	}

	if len(words) > 0 {
		ids, err := u.issueSvc.IdsByText(ctx, f.ProjectIds, words, maxTextMatches)
		if err != nil {
			return nil, fmt.Errorf("issueSvc.IdsByText: %w", err)
		}
		if len(ids) == 0 {
			return empty, nil
		}
		f.IssueIds = &ids
	}

	aggs, truncated, err := u.eventSvc.IssueAggregates(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("eventSvc.IssueAggregates: %w", err)
	}

	issues, err := u.issueSvc.ListByIds(ctx, lo.Map(aggs, func(a *eventModel.IssueAgg, _ int) int64 { return a.IssueId }))
	if err != nil {
		return nil, fmt.Errorf("issueSvc.ListByIds: %w", err)
	}
	byId := lo.KeyBy(issues, func(v *issueModel.Main) int64 { return v.Id })

	// проблемы, удалённые вместе с проектом, в ClickHouse ещё живут до TTL — пропускаем
	items := lo.FilterMap(aggs, func(a *eventModel.IssueAgg, _ int) (*ListItem, bool) {
		issue, ok := byId[a.IssueId]
		return &ListItem{Issue: issue, Events: a.Events, Users: a.Users}, ok
	})
	sortItems(items, req.Sort)

	total := int64(len(items))
	from := min(req.Page*req.PageSize, total)
	to := min(from+req.PageSize, total)
	page := items[from:to]

	if len(page) > 0 {
		pageIds := lo.Map(page, func(v *ListItem, _ int) int64 { return v.Issue.Id })
		// ровно sparklineBuckets интервалов: правая граница — по целому числу шагов
		step := f.To.Sub(f.From) / sparklineBuckets
		sf := *f
		sf.IssueIds = &pageIds
		sf.To = sf.From.Add(step * sparklineBuckets)
		series, err := u.eventSvc.Series(ctx, &sf, step)
		if err != nil {
			return nil, fmt.Errorf("eventSvc.Series: %w", err)
		}
		for _, item := range page {
			item.Sparkline = lo.Ternary(series[item.Issue.Id] != nil, series[item.Issue.Id], make([]int64, sparklineBuckets))
		}
	}

	return &ListRep{Items: page, Total: total, Truncated: truncated}, nil
}

// Get — проблема и её события и люди за период
func (u *Usecase) Get(ctx context.Context, id int64, period, environment string) (*GetRep, error) {
	issue, f, err := u.issueFilter(ctx, id, period, environment, "", defaultListPeriod)
	if err != nil {
		return nil, err
	}

	aggs, _, err := u.eventSvc.IssueAggregates(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("eventSvc.IssueAggregates: %w", err)
	}

	result := &GetRep{Issue: issue}
	if len(aggs) > 0 {
		result.Events, result.Users = aggs[0].Events, aggs[0].Users
	}
	return result, nil
}

// Stat — события проблемы по часам или дням; дни — по местному времени сервиса (TZ)
func (u *Usecase) Stat(ctx context.Context, id int64, period, interval, environment string) ([]*StatPoint, error) {
	_, f, err := u.issueFilter(ctx, id, period, environment, "", defaultListPeriod)
	if err != nil {
		return nil, err
	}

	var step time.Duration
	switch interval {
	case "", "hour":
		step = time.Hour
		f.From = f.From.Truncate(time.Hour)
	case "day":
		step = 24 * time.Hour
		local := f.From.In(time.Local)
		f.From = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
	default:
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "interval: hour, day", Fields: map[string]string{"interval": "invalid"}}
	}

	series, err := u.eventSvc.Series(ctx, f, step)
	if err != nil {
		return nil, fmt.Errorf("eventSvc.Series: %w", err)
	}

	counts := series[id]
	buckets := int((f.To.Sub(f.From) + step - 1) / step)
	return lo.Times(buckets, func(i int) *StatPoint {
		p := &StatPoint{Ts: f.From.Add(time.Duration(i) * step)}
		if i < len(counts) {
			p.Count = counts[i]
		}
		return p
	}), nil
}

// Tags — распределение значений тегов событий проблемы
func (u *Usecase) Tags(ctx context.Context, id int64, period, environment, query string) ([]*eventModel.TagDist, error) {
	_, f, err := u.issueFilter(ctx, id, period, environment, query, defaultListPeriod)
	if err != nil {
		return nil, err
	}

	result, err := u.eventSvc.TagTop(ctx, f, tagTopValues)
	if err != nil {
		return nil, fmt.Errorf("eventSvc.TagTop: %w", err)
	}
	slices.SortFunc(result, func(a, b *eventModel.TagDist) int {
		if a.Total != b.Total {
			return int(b.Total - a.Total)
		}
		return strings.Compare(a.Key, b.Key)
	})
	return result, nil
}

// ListEvents — сохранённые события проблемы
func (u *Usecase) ListEvents(ctx context.Context, issueId int64, query, period string, pars commonModel.ListParams) ([]*eventModel.EventSummary, int64, error) {
	if err := util.RequirePageSize(pars, maxPageSize); err != nil {
		return nil, 0, err
	}
	_, f, err := u.issueFilter(ctx, issueId, period, "", query, defaultEventPeriod)
	if err != nil {
		return nil, 0, err
	}

	items, total, err := u.eventSvc.ListEvents(ctx, f, pars.Page*pars.PageSize, pars.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("eventSvc.ListEvents: %w", err)
	}
	return items, total, nil
}

// GetEvent — событие проблемы; eventId = latest / oldest
func (u *Usecase) GetEvent(ctx context.Context, issueId int64, eventId string) (*eventModel.EventDetail, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, errs.NotAuthorized
	}
	if issueId == 0 {
		return nil, errs.IdRequired
	}
	issue, _, err := u.issueSvc.Get(ctx, issueId, true)
	if err != nil {
		return nil, fmt.Errorf("issueSvc.Get: %w", err)
	}
	if err = u.accessSvc.CheckProject(ctx, issue.ProjectId); err != nil {
		return nil, err
	}

	result, err := u.eventSvc.GetEvent(ctx, issue.ProjectId, issue.Id, eventId)
	if err != nil {
		return nil, fmt.Errorf("eventSvc.GetEvent: %w", err)
	}
	return result, nil
}

// TagKeys — ключи тегов для подсказок
func (u *Usecase) TagKeys(ctx context.Context, scope Scope) ([]string, error) {
	f, _, err := u.filter(ctx, scope, "", defaultListPeriod)
	if err != nil {
		return nil, err
	}
	result, err := u.eventSvc.TagKeys(ctx, f, tagKeysLimit)
	if err != nil {
		return nil, fmt.Errorf("eventSvc.TagKeys: %w", err)
	}
	return result, nil
}

// TagValues — значения ключа для подсказок
func (u *Usecase) TagValues(ctx context.Context, scope Scope, key, prefix string) ([]string, error) {
	if key == "" {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "key is required", Fields: map[string]string{"key": "required"}}
	}
	f, _, err := u.filter(ctx, scope, "", defaultListPeriod)
	if err != nil {
		return nil, err
	}
	result, err := u.eventSvc.TagValues(ctx, f, key, prefix, tagValuesLimit)
	if err != nil {
		return nil, fmt.Errorf("eventSvc.TagValues: %w", err)
	}
	return result, nil
}

// filter — фильтр событий по организации, проектам, периоду, окружению и строке поиска; слова
// поиска возвращаются отдельно (ищутся по проблемам в Postgres)
func (u *Usecase) filter(ctx context.Context, scope Scope, query string, defPeriod time.Duration) (*eventModel.StatFilter, []string, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, nil, errs.NotAuthorized
	}
	if scope.OrgId == 0 {
		return nil, nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "org_id is required", Fields: map[string]string{"org_id": "required"}}
	}

	period, err := util.ParsePeriod(scope.Period, defPeriod)
	if err != nil {
		return nil, nil, err
	}

	q, err := u.parser.Parse(query)
	if err != nil {
		return nil, nil, err
	}

	projects, _, err := u.projectSvc.List(ctx, &projectModel.ListReq{OrgId: &scope.OrgId})
	if err != nil {
		return nil, nil, fmt.Errorf("projectSvc.List: %w", err)
	}
	projectIds := lo.Map(projects, func(p *projectModel.Main, _ int) int64 { return p.Id })
	if len(scope.ProjectIds) > 0 {
		// только проекты этой организации
		projectIds = lo.Intersect(projectIds, scope.ProjectIds)
	}
	// участник — только свои проекты
	access, err := u.accessSvc.ProjectAccess(ctx)
	if err != nil {
		return nil, nil, err
	}
	projectIds = access.Filter(projectIds)

	now := time.Now().UTC()
	return &eventModel.StatFilter{
		ProjectIds:  projectIds,
		From:        now.Add(-period),
		To:          now.Add(time.Second),
		Environment: scope.Environment,
		Conds:       lo.Map(q.Conds, decodeCond),
	}, q.Words, nil
}

// issueFilter — фильтр событий одной проблемы
func (u *Usecase) issueFilter(ctx context.Context, id int64, period, environment, query string, defPeriod time.Duration) (*issueModel.Main, *eventModel.StatFilter, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, nil, errs.NotAuthorized
	}
	if id == 0 {
		return nil, nil, errs.IdRequired
	}

	issue, _, err := u.issueSvc.Get(ctx, id, true)
	if err != nil {
		return nil, nil, fmt.Errorf("issueSvc.Get: %w", err)
	}
	if err = u.accessSvc.CheckProject(ctx, issue.ProjectId); err != nil {
		return nil, nil, err
	}

	d, err := util.ParsePeriod(period, defPeriod)
	if err != nil {
		return nil, nil, err
	}
	q, err := u.parser.Parse(query)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	return issue, &eventModel.StatFilter{
		ProjectIds:  []int64{issue.ProjectId},
		IssueIds:    &[]int64{issue.Id},
		From:        now.Add(-d),
		To:          now.Add(time.Second),
		Environment: environment,
		Conds:       lo.Map(q.Conds, decodeCond),
	}, nil
}

func sortItems(items []*ListItem, sort string) {
	key := func(v *ListItem) int64 {
		switch sort {
		case "first_seen":
			return v.Issue.FirstSeen.UnixMilli()
		case "events":
			return v.Events
		case "users":
			return v.Users
		default:
			return v.Issue.LastSeen.UnixMilli()
		}
	}
	slices.SortStableFunc(items, func(a, b *ListItem) int {
		ka, kb := key(a), key(b)
		switch {
		case ka > kb:
			return -1
		case ka < kb:
			return 1
		case a.Issue.Id > b.Issue.Id:
			return -1
		case a.Issue.Id < b.Issue.Id:
			return 1
		}
		return 0
	})
}
