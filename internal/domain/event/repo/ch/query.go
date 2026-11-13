package ch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/domain/event/model"
)

// maxIssueAggs — предел проблем в агрегате списка (ADR-006); +1 — чтобы узнать, что обрезано
const maxIssueAggs = 50_000

// where — условия фильтра; все значения — параметрами
func where(f *model.StatFilter) (string, []any) {
	// время — миллисекундами: time.Time библиотека подставляет с точностью до секунды
	parts := []string{"project_id in ?", "timestamp >= fromUnixTimestamp64Milli(?, 'UTC')", "timestamp < fromUnixTimestamp64Milli(?, 'UTC')"}
	args := []any{lo.Map(f.ProjectIds, func(v int64, _ int) uint64 { return uint64(v) }), f.From.UnixMilli(), f.To.UnixMilli()}

	if f.IssueIds != nil {
		parts = append(parts, "issue_id in ?")
		args = append(args, lo.Map(*f.IssueIds, func(v int64, _ int) uint64 { return uint64(v) }))
	}
	if f.Environment != "" {
		parts = append(parts, "environment = ?")
		args = append(args, f.Environment)
	}
	for _, c := range f.Conds {
		switch c.Op {
		case "eq":
			parts = append(parts, "tags[?] = ?")
		case "ne":
			parts = append(parts, "tags[?] != ?")
		case "like":
			parts = append(parts, "tags[?] like ?")
		case "not_like":
			parts = append(parts, "tags[?] not like ?")
		default:
			continue
		}
		args = append(args, c.Key, c.Value)
	}

	return strings.Join(parts, " and "), args
}

// IssueAggregates — проблемы с событиями за период: сколько раз и у скольких людей (ADR-006)
func (r *Repo) IssueAggregates(ctx context.Context, f *model.StatFilter) ([]*model.IssueAgg, bool, error) {
	if len(f.ProjectIds) == 0 || (f.IssueIds != nil && len(*f.IssueIds) == 0) {
		return nil, false, nil
	}

	cond, args := where(f)
	rows, err := r.con.Query(ctx, `
		select issue_id, count(), uniqIf(user_key, user_key != ''), max(timestamp)
		from event_stat
		where `+cond+`
		group by issue_id
		order by count() desc
		limit ?`, append(args, maxIssueAggs+1)...)
	if err != nil {
		return nil, false, fmt.Errorf("query issue aggregates: %w", err)
	}
	defer rows.Close()

	result := make([]*model.IssueAgg, 0, 64)
	for rows.Next() {
		var issueId, events, users uint64
		var last time.Time
		if err = rows.Scan(&issueId, &events, &users, &last); err != nil {
			return nil, false, fmt.Errorf("scan issue aggregate: %w", err)
		}
		result = append(result, &model.IssueAgg{IssueId: int64(issueId), Events: int64(events), Users: int64(users), LastSeen: last})
	}
	if err = rows.Err(); err != nil {
		return nil, false, fmt.Errorf("rows issue aggregates: %w", err)
	}

	truncated := len(result) > maxIssueAggs
	if truncated {
		result = result[:maxIssueAggs]
	}
	return result, truncated, nil
}

// Series — число событий по интервалам step от f.From; пустые интервалы — нули.
// Ключ ответа — issue_id (для одной проблемы — один ряд).
func (r *Repo) Series(ctx context.Context, f *model.StatFilter, step time.Duration) (map[int64][]int64, error) {
	buckets := int(f.To.Sub(f.From) / step)
	if f.To.Sub(f.From)%step != 0 {
		buckets++
	}
	result := map[int64][]int64{}
	if len(f.ProjectIds) == 0 || buckets <= 0 || (f.IssueIds != nil && len(*f.IssueIds) == 0) {
		return result, nil
	}

	cond, args := where(f)
	stepSec := int64(step / time.Second)
	rows, err := r.con.Query(ctx, `
		select issue_id, intDiv(toUnixTimestamp(timestamp) - ?, ?) as b, count()
		from event_stat
		where `+cond+`
		group by issue_id, b`, append([]any{f.From.Unix(), stepSec}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("query series: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var issueId, count uint64
		var b int64
		if err = rows.Scan(&issueId, &b, &count); err != nil {
			return nil, fmt.Errorf("scan series: %w", err)
		}
		series, ok := result[int64(issueId)]
		if !ok {
			series = make([]int64, buckets)
			result[int64(issueId)] = series
		}
		if b >= 0 && int(b) < buckets {
			series[b] += int64(count)
		}
	}
	return result, rows.Err()
}

// TagTop — по каждому ключу тега: сколько событий с ним и топ значений.
//
// Одним проходом: ключи и значения карты разворачиваются парой, счёт по (ключ, значение)
// сворачивается по ключу — сумма даёт «всего», первые top по убыванию — значения. Раньше было два
// запроса по тем же строкам, и значение искалось в карте (tags[k]) на каждую пару: на проблеме с
// 300 тыс. событий — 3,4 с ЦПУ ClickHouse, одним проходом — 0,66 с.
func (r *Repo) TagTop(ctx context.Context, f *model.StatFilter, top int) ([]*model.TagDist, error) {
	if len(f.ProjectIds) == 0 {
		return nil, nil
	}
	cond, args := where(f)

	// при равном счёте — по значению, чтобы ответ не менялся от запроса к запросу
	rows, err := r.con.Query(ctx, `
		select k, total, arrayMap(x -> x.1, vs), arrayMap(x -> x.2, vs)
		from (
			select k, sum(c) as total,
			       arraySlice(arraySort(x -> (-toInt64(x.2), x.1), groupArray((v, c))), 1, ?) as vs
			from (
				select k, v, count() as c
				from event_stat array join mapKeys(tags) as k, mapValues(tags) as v
				where `+cond+`
				group by k, v
			)
			group by k
		)`, append([]any{top}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("query tag top: %w", err)
	}
	defer rows.Close()

	result := make([]*model.TagDist, 0, 16)
	for rows.Next() {
		var k string
		var total uint64
		var values []string
		var counts []uint64
		if err = rows.Scan(&k, &total, &values, &counts); err != nil {
			return nil, fmt.Errorf("scan tag top: %w", err)
		}
		result = append(result, &model.TagDist{
			Key:   k,
			Total: int64(total),
			Values: lo.Map(values, func(v string, i int) model.TagValueCount {
				return model.TagValueCount{Value: v, Count: int64(counts[i])}
			}),
		})
	}
	return result, rows.Err()
}

// TagKeys — ключи тегов по частоте
func (r *Repo) TagKeys(ctx context.Context, f *model.StatFilter, limit int) ([]string, error) {
	if len(f.ProjectIds) == 0 {
		return nil, nil
	}
	cond, args := where(f)
	rows, err := r.con.Query(ctx, `
		select k from event_stat array join mapKeys(tags) as k
		where `+cond+` group by k order by count() desc limit ?`, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("query tag keys: %w", err)
	}
	defer rows.Close()

	result := make([]string, 0, limit)
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scan tag keys: %w", err)
		}
		result = append(result, k)
	}
	return result, rows.Err()
}

// tagColumns — теги, значение которых лежит и отдельной колонкой (нормализация ставит их из тех же
// полей события). Читать колонку раз в десять дешевле, чем доставать значение из карты тегов:
// список окружений запрашивается при каждом открытии списка проблем
var tagColumns = map[string]string{
	"environment": "environment",
	"release":     "release",
	"level":       "level",
}

// TagValues — значения ключа по частоте; prefix — начало значения (без учёта регистра)
func (r *Repo) TagValues(ctx context.Context, f *model.StatFilter, key, prefix string, limit int) ([]string, error) {
	if len(f.ProjectIds) == 0 {
		return nil, nil
	}
	cond, args := where(f)

	var query string
	if column, ok := tagColumns[key]; ok {
		query = `select ` + column + ` as v from event_stat
			where ` + cond + ` and v != '' and v ilike ?
			group by v order by count() desc limit ?`
		args = append(args, escapeLike(prefix)+"%", limit)
	} else {
		query = `select tags[?] as v from event_stat
			where ` + cond + ` and mapContains(tags, ?) and v ilike ?
			group by v order by count() desc limit ?`
		args = append(append([]any{key}, args...), key, escapeLike(prefix)+"%", limit)
	}

	rows, err := r.con.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tag values: %w", err)
	}
	defer rows.Close()

	result := make([]string, 0, limit)
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan tag values: %w", err)
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

const eventSummaryColumns = `toString(event_id), timestamp, level, release, environment, browser, os, device,
	user_id, user_email, user_ip, tags['url'], title, message`

func scanSummary(scan func(dest ...any) error, extra ...any) (*model.EventSummary, error) {
	s := &model.EventSummary{}
	dest := append([]any{&s.EventId, &s.Timestamp, &s.Level, &s.Release, &s.Environment, &s.Browser, &s.Os,
		&s.Device, &s.UserId, &s.UserEmail, &s.UserIp, &s.Url, &s.Title, &s.Message}, extra...)
	if err := scan(dest...); err != nil {
		return nil, err
	}
	s.EventId = strings.ReplaceAll(s.EventId, "-", "")
	return s, nil
}

// ListEvents — сохранённые события проблемы, новые первыми
func (r *Repo) ListEvents(ctx context.Context, f *model.StatFilter, offset, limit int64) ([]*model.EventSummary, int64, error) {
	if len(f.ProjectIds) == 0 {
		return nil, 0, nil
	}
	cond, args := where(f)

	var total uint64
	if err := r.con.QueryRow(ctx, `select count() from event final where `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	rows, err := r.con.Query(ctx, `
		select `+eventSummaryColumns+` from event final
		where `+cond+`
		order by timestamp desc, event_id desc
		limit ? offset ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	result := make([]*model.EventSummary, 0, limit)
	for rows.Next() {
		s, err := scanSummary(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan event: %w", err)
		}
		result = append(result, s)
	}
	return result, int64(total), rows.Err()
}

// GetEvent — событие проблемы по id; eventId = "latest" / "oldest" — последнее / первое.
//
// В два шага: сначала ключ события по лёгким колонкам (eventKey), потом строка с телом — по полному
// ключу сортировки. Одним запросом (`select …, data … order by timestamp desc limit 1`) ClickHouse
// читал тела (data, до 1 МБ) всех событий проблемы и упирался в потолок памяти сервера (code 241).
// Замер на стенде (ClickHouse 25.8, лимит 2 ГБ), 500 событий по 1 МБ: было 400–925 МБ памяти на
// запрос, стало до 25 МБ — и от числа событий проблемы память больше не зависит.
func (r *Repo) GetEvent(ctx context.Context, projectId, issueId int64, eventId string) (*model.EventDetail, bool, error) {
	ts, id, found, err := r.eventKey(ctx, projectId, issueId, eventId)
	if err != nil || !found {
		return nil, false, err
	}

	// без final: строки с одним ключом — повторная доставка того же события, годится любая.
	// prewhere — явно: тело читается только у строки, прошедшей условие, а не у всей гранулы
	rows, err := r.con.Query(ctx, `
		select `+eventSummaryColumns+`, data from event
		prewhere project_id = ? and issue_id = ? and timestamp = fromUnixTimestamp64Milli(?, 'UTC') and event_id = ?
		limit 1`, uint64(projectId), uint64(issueId), ts.UnixMilli(), id)
	if err != nil {
		return nil, false, fmt.Errorf("query event: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, false, rows.Err()
	}

	detail := &model.EventDetail{}
	summary, err := scanSummary(rows.Scan, &detail.Data)
	if err != nil {
		return nil, false, fmt.Errorf("scan event: %w", err)
	}
	detail.EventSummary = *summary
	rows.Close()

	if detail.PrevEventId, err = r.neighbor(ctx, projectId, issueId, summary, "<", "desc"); err != nil {
		return nil, false, err
	}
	if detail.NextEventId, err = r.neighbor(ctx, projectId, issueId, summary, ">", "asc"); err != nil {
		return nil, false, err
	}
	return detail, true, nil
}

// eventKey — ключ события (timestamp, event_id) без чтения тела; eventId — как в GetEvent.
// Без final: дубли строки имеют тот же ключ, а без него ClickHouse читает в порядке ключа
// (последнее событие — с конца, не перебирая всю проблему) и использует индекс по event_id.
func (r *Repo) eventKey(ctx context.Context, projectId, issueId int64, eventId string) (time.Time, uuid.UUID, bool, error) {
	query := `select timestamp, event_id from event where project_id = ? and issue_id = ?`
	args := []any{uint64(projectId), uint64(issueId)}

	switch eventId {
	case "latest":
		query += ` order by timestamp desc, event_id desc limit 1`
	case "oldest":
		query += ` order by timestamp, event_id limit 1`
	default:
		id, err := uuid.Parse(eventId)
		if err != nil {
			return time.Time{}, uuid.Nil, false, nil
		}
		query += ` and event_id = ? limit 1`
		args = append(args, id)
	}

	rows, err := r.con.Query(ctx, query, args...)
	if err != nil {
		return time.Time{}, uuid.Nil, false, fmt.Errorf("query event key: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return time.Time{}, uuid.Nil, false, rows.Err()
	}

	var ts time.Time
	var id uuid.UUID
	if err = rows.Scan(&ts, &id); err != nil {
		return time.Time{}, uuid.Nil, false, fmt.Errorf("scan event key: %w", err)
	}
	return ts, id, true, nil
}

func (r *Repo) neighbor(ctx context.Context, projectId, issueId int64, s *model.EventSummary, cmp, dir string) (string, error) {
	id, _ := uuid.Parse(s.EventId)
	rows, err := r.con.Query(ctx, `
		select toString(event_id) from event final
		where project_id = ? and issue_id = ? and (timestamp, event_id) `+cmp+` (fromUnixTimestamp64Milli(?, 'UTC'), ?)
		order by timestamp `+dir+`, event_id `+dir+` limit 1`,
		uint64(projectId), uint64(issueId), s.Timestamp.UnixMilli(), id)
	if err != nil {
		return "", fmt.Errorf("query neighbor: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return "", rows.Err()
	}
	var v string
	if err = rows.Scan(&v); err != nil {
		return "", fmt.Errorf("scan neighbor: %w", err)
	}
	return strings.ReplaceAll(v, "-", ""), nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
