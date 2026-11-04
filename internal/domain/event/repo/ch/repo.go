package ch

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"github.com/rendau/zorko/internal/domain/event/model"
)

type Repo struct {
	con driver.Conn
}

func New(con driver.Conn) *Repo {
	return &Repo{con: con}
}

// InsertStats — каждое событие коротко (ADR-003), одной вставкой
func (r *Repo) InsertStats(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := r.con.PrepareBatch(ctx, `insert into event_stat
		(project_id, issue_id, event_id, timestamp, received_at, level, environment, release, user_key, tags)`)
	if err != nil {
		return fmt.Errorf("PrepareBatch(event_stat): %w", err)
	}
	defer func() { _ = batch.Abort() }()

	for _, ev := range events {
		err = batch.Append(
			uint64(ev.ProjectId), uint64(ev.IssueId), eventUuid(ev.EventId), ev.Timestamp, ev.ReceivedAt,
			ev.Level, ev.Environment, ev.Release, ev.User.Key(), ev.Tags,
		)
		if err != nil {
			return fmt.Errorf("Append(event_stat): %w", err)
		}
	}

	if err = batch.Send(); err != nil {
		return fmt.Errorf("Send(event_stat): %w", err)
	}
	return nil
}

// InsertEvents — полные записи, одной вставкой
func (r *Repo) InsertEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := r.con.PrepareBatch(ctx, `insert into event
		(project_id, issue_id, event_id, timestamp, received_at, level, environment, release, user_key, tags,
		 platform, title, culprit, message, exception_type, exception_value, transaction,
		 user_id, user_email, user_ip, browser, os, device, sdk_name, sdk_version, data)`)
	if err != nil {
		return fmt.Errorf("PrepareBatch(event): %w", err)
	}
	defer func() { _ = batch.Abort() }()

	for _, ev := range events {
		err = batch.Append(
			uint64(ev.ProjectId), uint64(ev.IssueId), eventUuid(ev.EventId), ev.Timestamp, ev.ReceivedAt,
			ev.Level, ev.Environment, ev.Release, ev.User.Key(), ev.Tags,
			ev.Platform, ev.Grouping.Title, ev.Grouping.Culprit, ev.Message, ev.Grouping.Type, ev.Grouping.Value,
			ev.Transaction, ev.User.Id, ev.User.Email, ev.User.IpAddress, ev.Browser, ev.Os, ev.Device,
			ev.SdkName, ev.SdkVersion, string(ev.Data),
		)
		if err != nil {
			return fmt.Errorf("Append(event): %w", err)
		}
	}

	if err = batch.Send(); err != nil {
		return fmt.Errorf("Send(event): %w", err)
	}
	return nil
}

// eventUuid — id события Sentry (32 hex) как UUID; некорректный id не роняет вставку
func eventUuid(id string) uuid.UUID {
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil
	}
	return u
}
