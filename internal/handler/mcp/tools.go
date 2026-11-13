package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/lo"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	orgModel "github.com/rendau/zorko/internal/domain/org/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/errs"
	issueUsc "github.com/rendau/zorko/internal/usecase/issue"
	oauthUsc "github.com/rendau/zorko/internal/usecase/oauth"
	replayUsc "github.com/rendau/zorko/internal/usecase/replay"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100

	// справочники (организации, проекты) читаются целиком, страницами по lookupPageSize
	lookupPageSize = 100
	lookupMaxPages = 20
)

// toolSet — инструменты одного запроса: работают от имени человека подключения
type toolSet struct {
	h       *Handler
	session *oauthUsc.McpSession
}

func (s *toolSet) register(srv *mcpsdk.Server) {
	addTool(s, srv, "list_project",
		"Организации и их проекты, доступные тебе: id, slug, имя, платформа. С этого начинают — остальным инструментам нужны org и projects.",
		s.listProject)

	addTool(s, srv, "list_issue",
		"Проблемы (одинаковые ошибки, склеенные вместе) с событиями за период: заголовок, место, уровень, сколько событий и людей за период, когда впервые и в последний раз. Сортировка: last_seen (по умолчанию), first_seen, events, users.",
		s.listIssue)
	addTool(s, srv, "get_issue",
		"Одна проблема по id: заголовок, место, уровень, первая и последняя версия, сколько событий и людей за период.",
		s.getIssue)
	addTool(s, srv, "get_issue_stat",
		"График проблемы: число событий по часам или дням за период. Показывает, когда началось и растёт ли.",
		s.getIssueStat)
	addTool(s, srv, "get_issue_tags",
		"Распределение событий проблемы по тегам (браузер, ОС, версия, окружение, адрес страницы и др.): у кого и где она случается.",
		s.getIssueTags)

	addTool(s, srv, "list_event",
		"Сохранённые события проблемы (отдельные случаи ошибки), новые первыми: когда, версия, окружение, браузер, человек, адрес страницы.",
		s.listEvent)
	addTool(s, srv, "get_event",
		"Событие проблемы целиком: цепочка исключений со стеком и строками кода, действия перед ошибкой, теги, окружение. event_id — id события, latest (по умолчанию) или oldest.",
		s.getEvent)

	addTool(s, srv, "list_tag_key",
		"Какие ключи тегов есть у событий за период — для условий в query.",
		s.listTagKey)
	addTool(s, srv, "list_tag_value",
		"Какие значения встречаются у ключа тега за период (prefix — начало значения).",
		s.listTagValue)

	addTool(s, srv, "list_replay",
		"Записи сессий (что человек делал на сайте), новые первыми: когда, сколько длилась, адрес, человек, число ошибок.",
		s.listReplay)
	addTool(s, srv, "get_replay",
		"Одна запись сессии и ошибки в ней (проблема, событие, время). Само воспроизведение — по ссылке url в интерфейсе.",
		s.getReplay)
	addTool(s, srv, "list_issue_replay",
		"Записи сессий, в которых случилась проблема, по времени ошибки, новые первыми.",
		s.listIssueReplay)
}

// addTool регистрирует инструмент чтения: сессия человека — в контекст, вызов — в метрики и лог,
// ошибка — агенту без внутренних подробностей
func addTool[In, Out any](s *toolSet, srv *mcpsdk.Server, name, description string, fn func(context.Context, In) (Out, error)) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in In) (*mcpsdk.CallToolResult, Out, error) {
		started := time.Now()

		out, err := fn(s.h.sessionSvc.WithContext(ctx, s.session.Session), in)
		s.h.observe(name, err, time.Since(started))
		if err != nil {
			var zero Out
			return nil, zero, s.toolErr(ctx, name, err)
		}

		return nil, out, nil
	})
}

// toolErr — ошибка для агента. Отказ по делу (не найдено, нет прав, неверный запрос) — код и
// пояснение, уровень info; сбой — уровень error, агенту без внутренних подробностей.
func (s *toolSet) toolErr(ctx context.Context, tool string, err error) error {
	code := errs.Code(err)

	level := lo.Ternary(errs.IsFailure(code), slog.LevelError, slog.LevelInfo)
	slog.Log(ctx, level, "mcp: tool error", "tool", tool, "error", err.Error(), "grant_id", s.session.GrantId, "usr_id", s.session.Session.Id)

	if errs.IsFailure(code) {
		return errors.New(code + ": временный сбой сервиса, повтори позже")
	}
	if fullErr, ok := errs.AsErrFull(err); ok && fullErr.Desc != "" {
		return errors.New(code + ": " + fullErr.Desc)
	}
	if hint, ok := errorHints[errs.Err(code)]; ok {
		return errors.New(code + ": " + hint)
	}
	return errors.New(code)
}

var errorHints = map[errs.Err]string{
	errs.NotAuthorized:     "подключение больше не действует или человек отключён",
	errs.NoPermission:      "этот проект тебе недоступен",
	errs.ObjectNotFound:    "не найдено",
	errs.IdRequired:        "нужен id",
	errs.IncorrectPageSize: "page_size — от 1 до 100",
	errs.ReplayNotFound:    "запись не найдена (хранятся 30 дней)",
}

func invalid(format string, args ...any) error {
	return errs.ErrFull{Err: errs.InvalidRequest, Desc: fmt.Sprintf(format, args...)}
}

// ── Общие вход/выход ────────────────────────────────────

type PageIn struct {
	Page     int64 `json:"page,omitempty" jsonschema:"номер страницы, начиная с 0"`
	PageSize int64 `json:"page_size,omitempty" jsonschema:"размер страницы: по умолчанию 25, не больше 100"`
}

func (p PageIn) listParams() commonModel.ListParams {
	if p.PageSize <= 0 {
		p.PageSize = defaultPageSize
	}
	return commonModel.ListParams{Page: max(p.Page, 0), PageSize: min(p.PageSize, maxPageSize), WithTotalCount: true}
}

type PageOut struct {
	Page     int64 `json:"page"`
	PageSize int64 `json:"page_size"`
	Total    int64 `json:"total"`
}

func pageOut(lp commonModel.ListParams, total int64) PageOut {
	return PageOut{Page: lp.Page, PageSize: lp.PageSize, Total: total}
}

// ScopeIn — где искать: организация, проекты, окружение, период
type ScopeIn struct {
	Org         string   `json:"org,omitempty" jsonschema:"организация: id или slug; можно не указывать, если тебе доступна одна"`
	Projects    []string `json:"projects,omitempty" jsonschema:"проекты: id или slug; пусто — все доступные проекты организации"`
	Environment string   `json:"environment,omitempty" jsonschema:"окружение (production, staging…); пусто — все"`
	Period      string   `json:"period,omitempty" jsonschema:"период: 1h, 24h, 7d, 14d (по умолчанию), 30d"`
}

// timeOut — время в поясе сервиса (TZ) со смещением: Postgres отдаёт местное, ClickHouse — UTC, а
// агенту и человеку нужен один вид
func timeOut(v time.Time) string {
	if v.IsZero() {
		return ""
	}
	return v.In(time.Local).Format(time.RFC3339)
}

func timePtrOut(v *time.Time) string {
	if v == nil {
		return ""
	}
	return timeOut(*v)
}

// ── Ссылки на интерфейс ─────────────────────────────────

func (s *toolSet) issueURL(issueId int64) string {
	if s.h.conf.AdminURL == "" {
		return ""
	}
	return s.h.conf.AdminURL + "/issues/" + strconv.FormatInt(issueId, 10)
}

func (s *toolSet) eventURL(issueId int64, eventId string) string {
	if s.h.conf.AdminURL == "" {
		return ""
	}
	return s.issueURL(issueId) + "?event=" + url.QueryEscape(eventId)
}

func (s *toolSet) replayURL(projectId int64, replayId string) string {
	if s.h.conf.AdminURL == "" {
		return ""
	}
	return s.h.conf.AdminURL + "/replays/" + url.PathEscape(replayId) + "?project=" + strconv.FormatInt(projectId, 10)
}

// ── Организации и проекты ───────────────────────────────

// orgs — все организации, видимые человеку
func (s *toolSet) orgs(ctx context.Context) ([]*orgModel.Main, error) {
	result := make([]*orgModel.Main, 0)
	for page := range int64(lookupMaxPages) {
		items, _, err := s.h.orgUsecase.List(ctx, &orgModel.ListReq{
			ListParams: commonModel.ListParams{Page: page, PageSize: lookupPageSize},
		})
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(items) < lookupPageSize {
			break
		}
	}
	return result, nil
}

// projects — все проекты организации, видимые человеку; orgId == nil — всех организаций
func (s *toolSet) projects(ctx context.Context, orgId *int64) ([]*projectModel.Main, error) {
	result := make([]*projectModel.Main, 0)
	for page := range int64(lookupMaxPages) {
		items, _, err := s.h.projectUsecase.List(ctx, &projectModel.ListReq{
			ListParams: commonModel.ListParams{Page: page, PageSize: lookupPageSize},
			OrgId:      orgId,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(items) < lookupPageSize {
			break
		}
	}
	return result, nil
}

// resolveOrg находит организацию по id или slug; без указания — единственную доступную
func (s *toolSet) resolveOrg(ctx context.Context, ref string) (*orgModel.Main, error) {
	orgs, err := s.orgs(ctx)
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, invalid("тебе не доступна ни одна организация: попроси админа zorko выдать проекты")
	}

	slugs := strings.Join(lo.Map(orgs, func(o *orgModel.Main, _ int) string { return o.Slug }), ", ")

	ref = strings.TrimSpace(ref)
	if ref == "" {
		if len(orgs) == 1 {
			return orgs[0], nil
		}
		return nil, invalid("укажи org — доступно несколько организаций: %s", slugs)
	}

	id, _ := strconv.ParseInt(ref, 10, 64)
	org, found := lo.Find(orgs, func(o *orgModel.Main) bool {
		return o.Slug == ref || (id != 0 && o.Id == id)
	})
	if !found {
		return nil, invalid("организация %q не найдена, доступны: %s", ref, slugs)
	}
	return org, nil
}

// scope — разобранный ScopeIn: организация и её проекты
type scope struct {
	org *orgModel.Main
	// все видимые проекты организации — для имён в ответах
	projects []*projectModel.Main
	// выбранные в запросе; пусто — все
	projectIds []int64
}

func (s *toolSet) resolveScope(ctx context.Context, in ScopeIn) (*scope, error) {
	org, err := s.resolveOrg(ctx, in.Org)
	if err != nil {
		return nil, err
	}
	projects, err := s.projects(ctx, &org.Id)
	if err != nil {
		return nil, err
	}

	result := &scope{org: org, projects: projects}
	for _, ref := range in.Projects {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		id, _ := strconv.ParseInt(ref, 10, 64)
		project, found := lo.Find(projects, func(p *projectModel.Main) bool {
			return p.Slug == ref || (id != 0 && p.Id == id)
		})
		if !found {
			return nil, invalid("проект %q не найден в организации %s, доступны: %s", ref, org.Slug,
				strings.Join(lo.Map(projects, func(p *projectModel.Main, _ int) string { return p.Slug }), ", "))
		}
		result.projectIds = append(result.projectIds, project.Id)
	}
	result.projectIds = lo.Uniq(result.projectIds)

	return result, nil
}

func (sc *scope) issueScope(in ScopeIn) issueUsc.Scope {
	return issueUsc.Scope{
		OrgId:       sc.org.Id,
		ProjectIds:  sc.projectIds,
		Environment: in.Environment,
		Period:      in.Period,
	}
}

func (sc *scope) projectSlugs() map[int64]string {
	return lo.SliceToMap(sc.projects, func(p *projectModel.Main) (int64, string) { return p.Id, p.Slug })
}

// projectSlug — slug проекта для ответа; недоступен — пусто (id в ответе есть всегда)
func (s *toolSet) projectSlug(ctx context.Context, projectId int64) string {
	project, err := s.h.projectUsecase.Get(ctx, projectId)
	if err != nil {
		return ""
	}
	return project.Slug
}

type ProjectOut struct {
	Id       int64  `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Platform string `json:"platform,omitempty"`
	// off — запись сессий выключена, error — только при ошибке, session — и часть всех сессий
	ReplayMode string `json:"replay_mode,omitempty"`
}

// projectToOut — секретов проекта (заголовок для скачивания карт с сайта) в ответе нет
func projectToOut(v *projectModel.Main, _ int) ProjectOut {
	return ProjectOut{Id: v.Id, Slug: v.Slug, Name: v.Name, Platform: v.Platform, ReplayMode: v.ReplayMode}
}

type OrgOut struct {
	Id       int64        `json:"id"`
	Slug     string       `json:"slug"`
	Name     string       `json:"name"`
	Projects []ProjectOut `json:"projects"`
}

type ListProjectIn struct {
	Org string `json:"org,omitempty" jsonschema:"организация: id или slug; пусто — все доступные"`
}

type ListProjectOut struct {
	Orgs []OrgOut `json:"orgs"`
}

func (s *toolSet) listProject(ctx context.Context, in ListProjectIn) (ListProjectOut, error) {
	orgs, err := s.orgs(ctx)
	if err != nil {
		return ListProjectOut{}, err
	}
	if strings.TrimSpace(in.Org) != "" {
		org, err := s.resolveOrg(ctx, in.Org)
		if err != nil {
			return ListProjectOut{}, err
		}
		orgs = []*orgModel.Main{org}
	}

	projects, err := s.projects(ctx, nil)
	if err != nil {
		return ListProjectOut{}, err
	}
	byOrg := lo.GroupBy(projects, func(p *projectModel.Main) int64 { return p.OrgId })

	return ListProjectOut{Orgs: lo.Map(orgs, func(o *orgModel.Main, _ int) OrgOut {
		return OrgOut{Id: o.Id, Slug: o.Slug, Name: o.Name, Projects: lo.Map(byOrg[o.Id], projectToOut)}
	})}, nil
}

// ── Проблемы ────────────────────────────────────────────

type IssueOut struct {
	Id        int64  `json:"id"`
	ProjectId int64  `json:"project_id"`
	Project   string `json:"project,omitempty" jsonschema:"slug проекта"`
	Title     string `json:"title"`
	Culprit   string `json:"culprit,omitempty" jsonschema:"место в коде: файл или функция"`
	Type      string `json:"type,omitempty" jsonschema:"тип исключения"`
	Value     string `json:"value,omitempty" jsonschema:"текст исключения"`
	Level     string `json:"level,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Status    string `json:"status,omitempty"`
	FirstSeen string `json:"first_seen,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
	TimesSeen int64  `json:"times_seen" jsonschema:"событий за всё время"`
	Events    int64  `json:"events" jsonschema:"событий за период"`
	Users     int64  `json:"users" jsonschema:"людей за период"`
	// версии приложения, в которых проблему увидели впервые и в последний раз
	FirstRelease string `json:"first_release,omitempty"`
	LastRelease  string `json:"last_release,omitempty"`
	Url          string `json:"url,omitempty" jsonschema:"страница проблемы в интерфейсе zorko"`
}

func (s *toolSet) issueToOut(v *issueModel.Main, project string, events, users int64) IssueOut {
	return IssueOut{
		Id:           v.Id,
		ProjectId:    v.ProjectId,
		Project:      project,
		Title:        v.Title,
		Culprit:      v.Culprit,
		Type:         v.Type,
		Value:        clip(v.Value),
		Level:        v.Level,
		Platform:     v.Platform,
		Status:       v.Status,
		FirstSeen:    timeOut(v.FirstSeen),
		LastSeen:     timeOut(v.LastSeen),
		TimesSeen:    v.TimesSeen,
		Events:       events,
		Users:        users,
		FirstRelease: v.FirstRelease,
		LastRelease:  v.LastRelease,
		Url:          s.issueURL(v.Id),
	}
}

type ListIssueIn struct {
	ScopeIn
	Query string `json:"query,omitempty" jsonschema:"строка поиска: слова из заголовка и условия ключ:значение (level:error, release:shop@1.4.2, url:*cart*, !browser.name:Safari)"`
	Sort  string `json:"sort,omitempty" jsonschema:"last_seen (по умолчанию) | first_seen | events | users"`
	PageIn
}

type ListIssueOut struct {
	Items []IssueOut `json:"items"`
	PageOut
	// проблем за период больше, чем сервис считает за раз: счётчики и порядок — по части из них
	Truncated bool `json:"truncated,omitempty"`
}

func (s *toolSet) listIssue(ctx context.Context, in ListIssueIn) (ListIssueOut, error) {
	sc, err := s.resolveScope(ctx, in.ScopeIn)
	if err != nil {
		return ListIssueOut{}, err
	}

	lp := in.listParams()
	rep, err := s.h.issueUsecase.List(ctx, &issueUsc.ListReq{
		ListParams: lp,
		Scope:      sc.issueScope(in.ScopeIn),
		Query:      in.Query,
		Sort:       in.Sort,
	})
	if err != nil {
		return ListIssueOut{}, err
	}

	slugs := sc.projectSlugs()
	return ListIssueOut{
		Items: lo.Map(rep.Items, func(v *issueUsc.ListItem, _ int) IssueOut {
			return s.issueToOut(v.Issue, slugs[v.Issue.ProjectId], v.Events, v.Users)
		}),
		PageOut:   pageOut(lp, rep.Total),
		Truncated: rep.Truncated,
	}, nil
}

type GetIssueIn struct {
	Id          int64  `json:"id" jsonschema:"id проблемы"`
	Period      string `json:"period,omitempty" jsonschema:"период для счётчиков: 1h, 24h, 7d, 14d (по умолчанию), 30d"`
	Environment string `json:"environment,omitempty" jsonschema:"окружение; пусто — все"`
}

func (s *toolSet) getIssue(ctx context.Context, in GetIssueIn) (IssueOut, error) {
	rep, err := s.h.issueUsecase.Get(ctx, in.Id, in.Period, in.Environment)
	if err != nil {
		return IssueOut{}, err
	}
	return s.issueToOut(rep.Issue, s.projectSlug(ctx, rep.Issue.ProjectId), rep.Events, rep.Users), nil
}

type GetIssueStatIn struct {
	Id          int64  `json:"id" jsonschema:"id проблемы"`
	Period      string `json:"period,omitempty" jsonschema:"1h, 24h, 7d, 14d (по умолчанию), 30d"`
	Interval    string `json:"interval,omitempty" jsonschema:"шаг: hour или day; по умолчанию hour для 1h и 24h, иначе day"`
	Environment string `json:"environment,omitempty" jsonschema:"окружение; пусто — все"`
}

type StatPointOut struct {
	Ts    string `json:"ts"`
	Count int64  `json:"count"`
}

type GetIssueStatOut struct {
	Interval string         `json:"interval"`
	Points   []StatPointOut `json:"points"`
}

func (s *toolSet) getIssueStat(ctx context.Context, in GetIssueStatIn) (GetIssueStatOut, error) {
	interval := in.Interval
	if interval == "" {
		interval = lo.Ternary(in.Period == "1h" || in.Period == "24h", "hour", "day")
	}

	points, err := s.h.issueUsecase.Stat(ctx, in.Id, in.Period, interval, in.Environment)
	if err != nil {
		return GetIssueStatOut{}, err
	}
	return GetIssueStatOut{
		Interval: interval,
		Points: lo.Map(points, func(p *issueUsc.StatPoint, _ int) StatPointOut {
			return StatPointOut{Ts: timeOut(p.Ts), Count: p.Count}
		}),
	}, nil
}

type GetIssueTagsIn struct {
	Id          int64  `json:"id" jsonschema:"id проблемы"`
	Period      string `json:"period,omitempty" jsonschema:"1h, 24h, 7d, 14d (по умолчанию), 30d"`
	Environment string `json:"environment,omitempty" jsonschema:"окружение; пусто — все"`
	Query       string `json:"query,omitempty" jsonschema:"условия на теги событий (ключ:значение)"`
}

type TagValueOut struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type TagDistOut struct {
	Key    string        `json:"key"`
	Total  int64         `json:"total" jsonschema:"событий с этим тегом"`
	Values []TagValueOut `json:"values" jsonschema:"самые частые значения"`
}

type GetIssueTagsOut struct {
	Tags []TagDistOut `json:"tags"`
}

func (s *toolSet) getIssueTags(ctx context.Context, in GetIssueTagsIn) (GetIssueTagsOut, error) {
	tags, err := s.h.issueUsecase.Tags(ctx, in.Id, in.Period, in.Environment, in.Query)
	if err != nil {
		return GetIssueTagsOut{}, err
	}
	return GetIssueTagsOut{Tags: lo.Map(tags, func(t *eventModel.TagDist, _ int) TagDistOut {
		return TagDistOut{
			Key:   t.Key,
			Total: t.Total,
			Values: lo.Map(t.Values, func(v eventModel.TagValueCount, _ int) TagValueOut {
				return TagValueOut{Value: clip(v.Value), Count: v.Count}
			}),
		}
	})}, nil
}

// ── События ─────────────────────────────────────────────

type EventSummaryOut struct {
	EventId     string `json:"event_id"`
	Timestamp   string `json:"timestamp,omitempty"`
	Level       string `json:"level,omitempty"`
	Release     string `json:"release,omitempty"`
	Environment string `json:"environment,omitempty"`
	Browser     string `json:"browser,omitempty"`
	Os          string `json:"os,omitempty"`
	Device      string `json:"device,omitempty"`
	UserId      string `json:"user_id,omitempty"`
	UserEmail   string `json:"user_email,omitempty"`
	UserIp      string `json:"user_ip,omitempty"`
	PageUrl     string `json:"page_url,omitempty" jsonschema:"адрес страницы, где случилась ошибка"`
	Title       string `json:"title,omitempty"`
	Message     string `json:"message,omitempty"`
}

func eventSummaryToOut(v *eventModel.EventSummary, _ int) EventSummaryOut {
	return EventSummaryOut{
		EventId:     v.EventId,
		Timestamp:   timeOut(v.Timestamp),
		Level:       v.Level,
		Release:     v.Release,
		Environment: v.Environment,
		Browser:     v.Browser,
		Os:          v.Os,
		Device:      v.Device,
		UserId:      v.UserId,
		UserEmail:   v.UserEmail,
		UserIp:      v.UserIp,
		PageUrl:     clip(v.Url),
		Title:       clip(v.Title),
		Message:     clip(v.Message),
	}
}

type ListEventIn struct {
	IssueId int64  `json:"issue_id" jsonschema:"id проблемы"`
	Query   string `json:"query,omitempty" jsonschema:"условия на теги событий (ключ:значение)"`
	Period  string `json:"period,omitempty" jsonschema:"1h, 24h, 7d, 14d, 30d (по умолчанию)"`
	PageIn
}

type ListEventOut struct {
	Items []EventSummaryOut `json:"items"`
	PageOut
}

func (s *toolSet) listEvent(ctx context.Context, in ListEventIn) (ListEventOut, error) {
	lp := in.listParams()
	items, total, err := s.h.issueUsecase.ListEvents(ctx, in.IssueId, in.Query, in.Period, lp)
	if err != nil {
		return ListEventOut{}, err
	}
	return ListEventOut{Items: lo.Map(items, eventSummaryToOut), PageOut: pageOut(lp, total)}, nil
}

type GetEventIn struct {
	IssueId int64  `json:"issue_id" jsonschema:"id проблемы"`
	EventId string `json:"event_id,omitempty" jsonschema:"id события, latest (по умолчанию) или oldest"`
}

type GetEventOut struct {
	Summary EventSummaryOut `json:"summary"`
	Detail  EventParsedOut  `json:"detail"`
	// соседние события проблемы по времени — чтобы листать
	PrevEventId string `json:"prev_event_id,omitempty" jsonschema:"событие раньше"`
	NextEventId string `json:"next_event_id,omitempty" jsonschema:"событие позже"`
	Url         string `json:"url,omitempty" jsonschema:"это событие в интерфейсе zorko"`
}

func (s *toolSet) getEvent(ctx context.Context, in GetEventIn) (GetEventOut, error) {
	eventId := lo.CoalesceOrEmpty(strings.TrimSpace(in.EventId), "latest")

	event, err := s.h.issueUsecase.GetEvent(ctx, in.IssueId, eventId)
	if err != nil {
		return GetEventOut{}, err
	}
	return GetEventOut{
		Summary:     eventSummaryToOut(&event.EventSummary, 0),
		Detail:      parseEvent(event.Data),
		PrevEventId: event.PrevEventId,
		NextEventId: event.NextEventId,
		Url:         s.eventURL(in.IssueId, event.EventId),
	}, nil
}

// ── Теги ────────────────────────────────────────────────

type ListTagKeyOut struct {
	Keys []string `json:"keys"`
}

func (s *toolSet) listTagKey(ctx context.Context, in ScopeIn) (ListTagKeyOut, error) {
	sc, err := s.resolveScope(ctx, in)
	if err != nil {
		return ListTagKeyOut{}, err
	}
	keys, err := s.h.issueUsecase.TagKeys(ctx, sc.issueScope(in))
	if err != nil {
		return ListTagKeyOut{}, err
	}
	return ListTagKeyOut{Keys: lo.Ternary(keys == nil, []string{}, keys)}, nil
}

type ListTagValueIn struct {
	ScopeIn
	Key    string `json:"key" jsonschema:"ключ тега (из list_tag_key)"`
	Prefix string `json:"prefix,omitempty" jsonschema:"начало значения; пусто — самые частые"`
}

type ListTagValueOut struct {
	Values []string `json:"values"`
}

func (s *toolSet) listTagValue(ctx context.Context, in ListTagValueIn) (ListTagValueOut, error) {
	sc, err := s.resolveScope(ctx, in.ScopeIn)
	if err != nil {
		return ListTagValueOut{}, err
	}
	values, err := s.h.issueUsecase.TagValues(ctx, sc.issueScope(in.ScopeIn), in.Key, in.Prefix)
	if err != nil {
		return ListTagValueOut{}, err
	}
	return ListTagValueOut{Values: lo.Map(values, func(v string, _ int) string { return clip(v) })}, nil
}

// ── Записи сессий ───────────────────────────────────────

type ReplayOut struct {
	ReplayId  string `json:"replay_id"`
	ProjectId int64  `json:"project_id"`
	Project   string `json:"project,omitempty" jsonschema:"slug проекта"`
	// buffer — запись началась из-за ошибки (минута до неё и дальше), session — обычная сессия
	Type        string `json:"type,omitempty"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
	DurationSec int64  `json:"duration_sec"`
	PageUrl     string `json:"page_url,omitempty" jsonschema:"первый адрес записи"`
	ErrorCount  int    `json:"error_count"`
	Environment string `json:"environment,omitempty"`
	Release     string `json:"release,omitempty"`
	UserId      string `json:"user_id,omitempty"`
	UserEmail   string `json:"user_email,omitempty"`
	UserIp      string `json:"user_ip,omitempty"`
	Browser     string `json:"browser,omitempty"`
	Os          string `json:"os,omitempty"`
	Device      string `json:"device,omitempty"`
	ErrorAt     string `json:"error_at,omitempty" jsonschema:"когда в записи случилась проблема (только в list_issue_replay)"`
	Url         string `json:"url,omitempty" jsonschema:"проигрыватель записи в интерфейсе zorko"`
}

func (s *toolSet) replayToOut(v *replayModel.Summary, project string) ReplayOut {
	out := ReplayOut{
		ReplayId:    v.ReplayId,
		ProjectId:   v.ProjectId,
		Project:     project,
		Type:        v.Type,
		Start:       timeOut(v.Start),
		End:         timeOut(v.End),
		PageUrl:     clip(v.Url),
		ErrorCount:  v.ErrorCount,
		Environment: v.Environment,
		Release:     v.Release,
		UserId:      v.UserId,
		UserEmail:   v.UserEmail,
		UserIp:      v.UserIp,
		Browser:     v.Browser,
		Os:          v.Os,
		Device:      v.Device,
		ErrorAt:     timePtrOut(v.ErrorAt),
		Url:         s.replayURL(v.ProjectId, v.ReplayId),
	}
	if !v.Start.IsZero() && v.End.After(v.Start) {
		out.DurationSec = int64(v.End.Sub(v.Start).Seconds())
	}
	return out
}

type ListReplayIn struct {
	Org         string   `json:"org,omitempty" jsonschema:"организация: id или slug; можно не указывать, если тебе доступна одна"`
	Projects    []string `json:"projects,omitempty" jsonschema:"проекты: id или slug; пусто — все доступные проекты организации"`
	Environment string   `json:"environment,omitempty" jsonschema:"окружение; пусто — все"`
	Release     string   `json:"release,omitempty" jsonschema:"версия приложения; пусто — все"`
	Query       string   `json:"query,omitempty" jsonschema:"часть адреса страницы, id или почта человека"`
	HasErrors   *bool    `json:"has_errors,omitempty" jsonschema:"true — только записи с ошибками, false — только без"`
	Type        string   `json:"type,omitempty" jsonschema:"buffer (началась из-за ошибки) или session; пусто — все"`
	PageIn
}

type ListReplayOut struct {
	Items []ReplayOut `json:"items"`
	PageOut
}

func (s *toolSet) listReplay(ctx context.Context, in ListReplayIn) (ListReplayOut, error) {
	sc, err := s.resolveScope(ctx, ScopeIn{Org: in.Org, Projects: in.Projects})
	if err != nil {
		return ListReplayOut{}, err
	}

	lp := in.listParams()
	items, total, err := s.h.replayUsecase.List(ctx, &replayUsc.ListReq{
		ListParams:  lp,
		OrgId:       sc.org.Id,
		ProjectIds:  sc.projectIds,
		Environment: in.Environment,
		Release:     in.Release,
		Query:       in.Query,
		HasErrors:   in.HasErrors,
		Type:        in.Type,
	})
	if err != nil {
		return ListReplayOut{}, err
	}

	slugs := sc.projectSlugs()
	return ListReplayOut{
		Items: lo.Map(items, func(v *replayModel.Summary, _ int) ReplayOut {
			return s.replayToOut(v, slugs[v.ProjectId])
		}),
		PageOut: pageOut(lp, total),
	}, nil
}

type GetReplayIn struct {
	Project  string `json:"project" jsonschema:"проект записи: id или slug (project_id из list_replay)"`
	ReplayId string `json:"replay_id" jsonschema:"id записи"`
}

type ReplayErrorOut struct {
	IssueId   int64  `json:"issue_id"`
	Title     string `json:"title,omitempty"`
	EventId   string `json:"event_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Url       string `json:"url,omitempty" jsonschema:"событие в интерфейсе zorko"`
}

type GetReplayOut struct {
	Replay ReplayOut        `json:"replay"`
	Errors []ReplayErrorOut `json:"errors" jsonschema:"ошибки в записи: проблема, событие, время"`
}

func (s *toolSet) getReplay(ctx context.Context, in GetReplayIn) (GetReplayOut, error) {
	project, err := s.resolveProject(ctx, in.Project)
	if err != nil {
		return GetReplayOut{}, err
	}

	rep, err := s.h.replayUsecase.Get(ctx, project.Id, in.ReplayId)
	if err != nil {
		return GetReplayOut{}, err
	}
	return GetReplayOut{
		Replay: s.replayToOut(rep.Summary, project.Slug),
		Errors: lo.Map(rep.Errors, func(e *replayUsc.ErrorMark, _ int) ReplayErrorOut {
			return ReplayErrorOut{
				IssueId:   e.IssueId,
				Title:     clip(e.Title),
				EventId:   e.EventId,
				Timestamp: timeOut(e.Timestamp),
				Url:       s.eventURL(e.IssueId, e.EventId),
			}
		}),
	}, nil
}

// resolveProject находит проект по id или slug среди всех видимых человеку
func (s *toolSet) resolveProject(ctx context.Context, ref string) (*projectModel.Main, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, invalid("укажи project: id или slug проекта")
	}

	projects, err := s.projects(ctx, nil)
	if err != nil {
		return nil, err
	}

	id, _ := strconv.ParseInt(ref, 10, 64)
	matched := lo.Filter(projects, func(p *projectModel.Main, _ int) bool {
		return p.Slug == ref || (id != 0 && p.Id == id)
	})
	switch len(matched) {
	case 0:
		return nil, invalid("проект %q не найден среди доступных тебе", ref)
	case 1:
		return matched[0], nil
	}
	// один slug в разных организациях
	return nil, invalid("проект %q есть в нескольких организациях — укажи его id: %s", ref,
		strings.Join(lo.Map(matched, func(p *projectModel.Main, _ int) string { return strconv.FormatInt(p.Id, 10) }), ", "))
}

type ListIssueReplayIn struct {
	IssueId int64 `json:"issue_id" jsonschema:"id проблемы"`
	PageIn
}

func (s *toolSet) listIssueReplay(ctx context.Context, in ListIssueReplayIn) (ListReplayOut, error) {
	lp := in.listParams()
	items, total, err := s.h.replayUsecase.IssueReplays(ctx, in.IssueId, lp)
	if err != nil {
		return ListReplayOut{}, err
	}

	project := ""
	if len(items) > 0 {
		project = s.projectSlug(ctx, items[0].ProjectId)
	}
	return ListReplayOut{
		Items: lo.Map(items, func(v *replayModel.Summary, _ int) ReplayOut {
			return s.replayToOut(v, project)
		}),
		PageOut: pageOut(lp, total),
	}, nil
}
