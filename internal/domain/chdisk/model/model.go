// Package model — диск ClickHouse и дни данных zorko на нём (ADR-017)
package model

import "time"

// Disk — диск ClickHouse. Free — без запаса keep_free_space_bytes (его ClickHouse не займёт).
// Pending — неактивные куски: уже удалены или слиты, ClickHouse сотрёт их с диска через
// несколько минут (old_parts_lifetime) — это место уже освобождается.
type Disk struct {
	Free    uint64
	Total   uint64
	Pending uint64
}

// Used — сколько занято с учётом уже освобождаемого
func (d *Disk) Used() uint64 {
	busy := d.Total - min(d.Free, d.Total)
	return busy - min(d.Pending, busy)
}

// UsedPercent — доля занятого, 0..100
func (d *Disk) UsedPercent() float64 {
	if d.Total == 0 {
		return 0
	}
	return float64(d.Used()) * 100 / float64(d.Total)
}

// Partition — партиция таблицы zorko за один день (partition by toYYYYMMDD)
type Partition struct {
	Table string
	Id    string // 20261106
	Bytes uint64
}

// Day — один день данных во всех таблицах zorko
type Day struct {
	Day        time.Time // начало дня, UTC
	Bytes      uint64
	Partitions []*Partition
}
