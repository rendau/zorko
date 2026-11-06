// Package ch — диск ClickHouse и партиции таблиц zorko (ADR-017)
package ch

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/rendau/zorko/internal/domain/chdisk/model"
)

type Repo struct {
	con driver.Conn
}

func New(con driver.Conn) *Repo {
	return &Repo{con: con}
}

// Disk — диск default (на нём все таблицы); Pending — неактивные куски всех баз
func (r *Repo) Disk(ctx context.Context) (*model.Disk, error) {
	d := &model.Disk{}
	err := r.con.QueryRow(ctx, `select
		(select free_space from system.disks where name = 'default'),
		(select total_space from system.disks where name = 'default'),
		(select sum(bytes_on_disk) from system.parts where not active and disk_name = 'default')`,
	).Scan(&d.Free, &d.Total, &d.Pending)
	if err != nil {
		return nil, fmt.Errorf("select system.disks: %w", err)
	}
	return d, nil
}

// Partitions — активные партиции таблиц текущей базы
func (r *Repo) Partitions(ctx context.Context, tables []string) ([]*model.Partition, error) {
	rows, err := r.con.Query(ctx, `select table, partition_id, sum(bytes_on_disk)
		from system.parts
		where active and database = currentDatabase() and has(?, table)
		group by table, partition_id`, tables)
	if err != nil {
		return nil, fmt.Errorf("select system.parts: %w", err)
	}
	defer rows.Close()

	var result []*model.Partition
	for rows.Next() {
		p := &model.Partition{}
		if err = rows.Scan(&p.Table, &p.Id, &p.Bytes); err != nil {
			return nil, fmt.Errorf("scan system.parts: %w", err)
		}
		result = append(result, p)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows system.parts: %w", err)
	}
	return result, nil
}

// DropPartition — table и id проверены вызывающим (имена из списка, id — дата YYYYMMDD):
// DDL не принимает параметры
func (r *Repo) DropPartition(ctx context.Context, table, id string) error {
	if err := r.con.Exec(ctx, fmt.Sprintf("alter table %s drop partition id '%s'", table, id)); err != nil {
		return fmt.Errorf("drop partition %s/%s: %w", table, id, err)
	}
	return nil
}
