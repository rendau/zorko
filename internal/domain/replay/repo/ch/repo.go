// Package ch — описания сегментов и связи записей с проблемами в ClickHouse (ADR-012, ADR-014)
package ch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/domain/replay/model"
)

type Repo struct {
	con driver.Conn
}

func New(con driver.Conn) *Repo {
	return &Repo{con: con}
}

func (r *Repo) InsertSegments(ctx context.Context, segments []*model.Segment) error {
	if len(segments) == 0 {
		return nil
	}

	batch, err := r.con.PrepareBatch(ctx, `insert into replay_segment
		(project_id, replay_id, segment_id, replay_type, replay_start, timestamp, received_at, urls, error_ids,
		 environment, release, user_id, user_email, user_ip, browser, os, device, sdk_name, sdk_version, size)`)
	if err != nil {
		return fmt.Errorf("PrepareBatch(replay_segment): %w", err)
	}
	defer func() { _ = batch.Abort() }()

	for _, s := range segments {
		err = batch.Append(
			uint64(s.ProjectId), toUuid(s.ReplayId), uint16(s.SegmentId), s.Type, s.Start, s.Timestamp, s.ReceivedAt,
			lo.Ternary(s.Urls == nil, []string{}, s.Urls), lo.Map(s.ErrorIds, func(v string, _ int) uuid.UUID { return toUuid(v) }),
			s.Environment, s.Release, s.UserId, s.UserEmail, s.UserIp, s.Browser, s.Os, s.Device, s.SdkName, s.SdkVersion,
			uint32(s.Size),
		)
		if err != nil {
			return fmt.Errorf("Append(replay_segment): %w", err)
		}
	}

	if err = batch.Send(); err != nil {
		return fmt.Errorf("Send(replay_segment): %w", err)
	}
	return nil
}

func (r *Repo) InsertLinks(ctx context.Context, links []*model.Link) error {
	if len(links) == 0 {
		return nil
	}

	batch, err := r.con.PrepareBatch(ctx, `insert into replay_issue (project_id, issue_id, replay_id, event_id, timestamp)`)
	if err != nil {
		return fmt.Errorf("PrepareBatch(replay_issue): %w", err)
	}
	defer func() { _ = batch.Abort() }()

	for _, l := range links {
		err = batch.Append(uint64(l.ProjectId), uint64(l.IssueId), toUuid(l.ReplayId), toUuid(l.EventId), l.Timestamp)
		if err != nil {
			return fmt.Errorf("Append(replay_issue): %w", err)
		}
	}

	if err = batch.Send(); err != nil {
		return fmt.Errorf("Send(replay_issue): %w", err)
	}
	return nil
}

// summaryColumns — запись целиком по её сегментам; «первое» значение — у сегмента с меньшим номером,
// пользователь — последний известный (вход на сайт мог случиться посреди записи)
const summaryColumns = `project_id, replay_id,
	argMin(replay_type, segment_id), min(replay_start), max(timestamp),
	uniqExact(segment_id), max(segment_id),
	argMinIf(urls[1], segment_id, notEmpty(urls)),
	length(arrayDistinct(arrayFlatten(groupArray(error_ids)))),
	argMin(environment, segment_id), argMin(release, segment_id),
	argMaxIf(user_id, segment_id, user_id != ''), argMaxIf(user_email, segment_id, user_email != ''),
	argMin(user_ip, segment_id), argMin(browser, segment_id), argMin(os, segment_id), argMin(device, segment_id),
	argMin(sdk_name, segment_id), argMin(sdk_version, segment_id)`

// записи без нулевого сегмента не показываются: без снимка страницы их не проиграть
const havingSegmentZero = `countIf(segment_id = 0) > 0`

func scanSummary(scan func(dest ...any) error, extra ...any) (*model.Summary, error) {
	s := &model.Summary{}
	var projectId uint64
	var replayId uuid.UUID
	var segments uint64
	var maxSegment uint16
	var errorCount uint64
	dest := []any{&projectId, &replayId, &s.Type, &s.Start, &s.End, &segments, &maxSegment, &s.Url, &errorCount,
		&s.Environment, &s.Release, &s.UserId, &s.UserEmail, &s.UserIp, &s.Browser, &s.Os, &s.Device,
		&s.SdkName, &s.SdkVersion}
	if err := scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	s.ProjectId = int64(projectId)
	s.ReplayId = fromUuid(replayId)
	s.Segments = int(segments)
	s.MaxSegment = int(maxSegment)
	s.ErrorCount = int(errorCount)
	return s, nil
}

// listFilter — where (постоянное для записи) и having (по любому сегменту) выборки записей
func listFilter(pars *model.ListReq) (string, []any, string, []any) {
	where := []string{"project_id in ?"}
	whereArgs := []any{lo.Map(pars.ProjectIds, func(v int64, _ int) uint64 { return uint64(v) })}
	if pars.Environment != nil && *pars.Environment != "" {
		where = append(where, "environment = ?")
		whereArgs = append(whereArgs, *pars.Environment)
	}
	if pars.Release != nil && *pars.Release != "" {
		where = append(where, "release = ?")
		whereArgs = append(whereArgs, *pars.Release)
	}
	if pars.Type != nil && *pars.Type != "" {
		where = append(where, "replay_type = ?")
		whereArgs = append(whereArgs, *pars.Type)
	}

	having := []string{havingSegmentZero}
	havingArgs := make([]any, 0, 3)
	if pars.Query != nil && strings.TrimSpace(*pars.Query) != "" {
		q := strings.TrimSpace(*pars.Query)
		having = append(having, `sum(arrayExists(u -> positionCaseInsensitive(u, ?) > 0, urls)
			or positionCaseInsensitive(user_email, ?) > 0 or user_id = ?) > 0`)
		havingArgs = append(havingArgs, q, q, q)
	}
	if pars.HasErrors != nil {
		having = append(having, lo.Ternary(*pars.HasErrors,
			"length(arrayDistinct(arrayFlatten(groupArray(error_ids)))) > 0",
			"length(arrayDistinct(arrayFlatten(groupArray(error_ids)))) = 0"))
	}

	return strings.Join(where, " and "), whereArgs, strings.Join(having, " and "), havingArgs
}

// List — записи видимых проектов, новые первыми
func (r *Repo) List(ctx context.Context, pars *model.ListReq) ([]*model.Summary, int64, error) {
	if len(pars.ProjectIds) == 0 {
		return []*model.Summary{}, 0, nil
	}
	where, whereArgs, having, havingArgs := listFilter(pars)
	args := append(append([]any{}, whereArgs...), havingArgs...)

	var total uint64
	if err := r.con.QueryRow(ctx, `select count() from (
		select replay_id from replay_segment where `+where+` group by project_id, replay_id having `+having+`)`,
		args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count replays: %w", err)
	}

	limit := lo.Ternary(pars.PageSize > 0, pars.PageSize, 20)
	rows, err := r.con.Query(ctx, `select `+summaryColumns+` from replay_segment
		where `+where+`
		group by project_id, replay_id
		having `+having+`
		order by min(replay_start) desc, replay_id desc
		limit ? offset ?`, append(args, limit, pars.Page*limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query replays: %w", err)
	}
	defer rows.Close()

	result := make([]*model.Summary, 0, limit)
	for rows.Next() {
		s, err := scanSummary(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan replay: %w", err)
		}
		result = append(result, s)
	}
	return result, int64(total), rows.Err()
}

// Get — запись проекта; нет нулевого сегмента — found=false
func (r *Repo) Get(ctx context.Context, projectId int64, replayId string) (*model.Summary, bool, error) {
	rows, err := r.con.Query(ctx, `select `+summaryColumns+` from replay_segment
		where project_id = ? and replay_id = ?
		group by project_id, replay_id
		having `+havingSegmentZero, uint64(projectId), toUuid(replayId))
	if err != nil {
		return nil, false, fmt.Errorf("query replay: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	s, err := scanSummary(rows.Scan)
	if err != nil {
		return nil, false, fmt.Errorf("scan replay: %w", err)
	}
	return s, true, nil
}

// IssueReplays — записи с ошибками проблемы; ErrorAt — первая ошибка проблемы в записи, новые первыми.
//
// В два шага: страница и общее число — по узким колонкам (связи replay_issue и наличие нулевого
// сегмента по первичному ключу), сводка по широким колонкам — только для записей страницы. Раньше
// сводка считалась по всем записям проблемы, и дважды: для числа и для страницы.
func (r *Repo) IssueReplays(ctx context.Context, projectId, issueId int64, page, pageSize int64) ([]*model.Summary, int64, error) {
	linkArgs := []any{uint64(projectId), uint64(issueId)}
	// записи проблемы, которые можно проиграть (есть нулевой сегмент), и время первой ошибки в каждой
	linked := `from (
			select replay_id, min(timestamp) as error_at from replay_issue
			where project_id = ? and issue_id = ? group by replay_id
		) as l
		where l.replay_id in (
			select replay_id from replay_segment
			where project_id = ? and segment_id = 0
			  and replay_id in (select replay_id from replay_issue where project_id = ? and issue_id = ?)
		)`
	linkedArgs := append(append(append([]any{}, linkArgs...), uint64(projectId)), linkArgs...)

	var total uint64
	if err := r.con.QueryRow(ctx, `select count() `+linked, linkedArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count issue replays: %w", err)
	}

	limit := lo.Ternary(pageSize > 0, pageSize, 20)
	rows, err := r.con.Query(ctx, `select l.replay_id, l.error_at `+linked+`
		order by l.error_at desc, l.replay_id desc
		limit ? offset ?`, append(linkedArgs, limit, page*limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query issue replays: %w", err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0, limit)
	errorAt := make(map[uuid.UUID]time.Time, limit)
	for rows.Next() {
		var id uuid.UUID
		var at time.Time
		if err = rows.Scan(&id, &at); err != nil {
			return nil, 0, fmt.Errorf("scan issue replay: %w", err)
		}
		ids = append(ids, id)
		errorAt[id] = at
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows issue replays: %w", err)
	}
	if len(ids) == 0 {
		return []*model.Summary{}, int64(total), nil
	}

	summaries, err := r.summaries(ctx, projectId, ids)
	if err != nil {
		return nil, 0, err
	}

	// порядок страницы; запись, сегменты которой успели уйти по TTL, пропускается
	result := lo.FilterMap(ids, func(id uuid.UUID, _ int) (*model.Summary, bool) {
		s, ok := summaries[id]
		if ok {
			s.ErrorAt = new(errorAt[id])
		}
		return s, ok
	})
	return result, int64(total), nil
}

// summaries — сводки записей проекта по их номерам
func (r *Repo) summaries(ctx context.Context, projectId int64, ids []uuid.UUID) (map[uuid.UUID]*model.Summary, error) {
	rows, err := r.con.Query(ctx, `select `+summaryColumns+` from replay_segment
		where project_id = ? and replay_id in ?
		group by project_id, replay_id
		having `+havingSegmentZero, uint64(projectId), ids)
	if err != nil {
		return nil, fmt.Errorf("query replay summaries: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]*model.Summary, len(ids))
	for rows.Next() {
		s, err := scanSummary(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan replay summary: %w", err)
		}
		result[toUuid(s.ReplayId)] = s
	}
	return result, rows.Err()
}

// Links — ошибки записи (для отметок на шкале), по времени; не больше limit
func (r *Repo) Links(ctx context.Context, projectId int64, replayId string, limit int) ([]*model.Link, error) {
	rows, err := r.con.Query(ctx, `select issue_id, event_id, min(timestamp) as ts from replay_issue
		where project_id = ? and replay_id = ?
		group by issue_id, event_id
		order by ts limit ?`, uint64(projectId), toUuid(replayId), limit)
	if err != nil {
		return nil, fmt.Errorf("query replay links: %w", err)
	}
	defer rows.Close()

	result := make([]*model.Link, 0)
	for rows.Next() {
		var issueId uint64
		var eventId uuid.UUID
		l := &model.Link{ProjectId: projectId, ReplayId: replayId}
		if err = rows.Scan(&issueId, &eventId, &l.Timestamp); err != nil {
			return nil, fmt.Errorf("scan replay link: %w", err)
		}
		l.IssueId = int64(issueId)
		l.EventId = fromUuid(eventId)
		result = append(result, l)
	}
	return result, rows.Err()
}

// SegmentReceivedAt — когда принят сегмент: по этому дню лежит его тело в S3. Повтор отправки —
// та же строка с другим временем; тело перезаписано последним, берём последнее.
func (r *Repo) SegmentReceivedAt(ctx context.Context, projectId int64, replayId string, segmentId int) (time.Time, bool, error) {
	rows, err := r.con.Query(ctx, `select max(received_at) from replay_segment
		where project_id = ? and replay_id = ? and segment_id = ? having count() > 0`,
		uint64(projectId), toUuid(replayId), uint16(segmentId))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("query segment: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return time.Time{}, false, rows.Err()
	}
	var receivedAt time.Time
	if err = rows.Scan(&receivedAt); err != nil {
		return time.Time{}, false, fmt.Errorf("scan segment: %w", err)
	}
	return receivedAt, true, nil
}

// SegmentsReceivedAt — когда принят каждый сегмент записи (см. SegmentReceivedAt): одним запросом
// на всю запись, для проигрывателя, который забирает её сегменты пачкой
func (r *Repo) SegmentsReceivedAt(ctx context.Context, projectId int64, replayId string) (map[int]time.Time, error) {
	rows, err := r.con.Query(ctx, `select segment_id, max(received_at) from replay_segment
		where project_id = ? and replay_id = ? group by segment_id`, uint64(projectId), toUuid(replayId))
	if err != nil {
		return nil, fmt.Errorf("query segments: %w", err)
	}
	defer rows.Close()

	result := make(map[int]time.Time, 64)
	for rows.Next() {
		var segmentId uint16
		var receivedAt time.Time
		if err = rows.Scan(&segmentId, &receivedAt); err != nil {
			return nil, fmt.Errorf("scan segments: %w", err)
		}
		result[int(segmentId)] = receivedAt
	}
	return result, rows.Err()
}

// toUuid — 32 hex как UUID; некорректный id не роняет вставку
func toUuid(id string) uuid.UUID {
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil
	}
	return u
}

func fromUuid(u uuid.UUID) string {
	return strings.ReplaceAll(u.String(), "-", "")
}
