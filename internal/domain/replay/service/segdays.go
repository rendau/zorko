package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// сколько помнить дни сегментов записи: открытие проигрывателя — секунды, запас — на докачку
	segDaysTtl = 2 * time.Minute
	// записей в памяти; запись — до 1001 сегмента по ~40 байт
	segDaysMax = 256
)

// segDays помнит, в какой день принят каждый сегмент записи (по дню лежит его тело в S3).
// Проигрыватель забирает запись пачкой, по запросу на сегмент: без памяти запись из 300 сегментов —
// это 300 одинаковых по смыслу запросов в ClickHouse, с ней — один на всю запись.
type segDays struct {
	repoCh RepoChI

	mu    sync.Mutex
	items map[segDaysKey]segDaysItem
	// одновременные запросы сегментов одной записи ждут одну загрузку
	group singleflight.Group
}

type segDaysKey struct {
	projectId int64
	replayId  string
}

type segDaysItem struct {
	loadedAt time.Time
	days     map[int]time.Time
}

func newSegDays(repoCh RepoChI) *segDays {
	return &segDays{repoCh: repoCh, items: make(map[segDaysKey]segDaysItem, segDaysMax)}
}

// receivedAt — день приёма сегмента; found=false — такого сегмента нет
func (c *segDays) receivedAt(ctx context.Context, projectId int64, replayId string, segmentId int) (time.Time, bool, error) {
	key := segDaysKey{projectId: projectId, replayId: replayId}

	c.mu.Lock()
	item, ok := c.items[key]
	c.mu.Unlock()

	if ok && time.Since(item.loadedAt) < segDaysTtl {
		if at, found := item.days[segmentId]; found {
			return at, true, nil
		}
		// сегмента в памяти нет: запись могла ещё идти, когда её запомнили, — спрашиваем точечно
		at, found, err := c.repoCh.SegmentReceivedAt(ctx, projectId, replayId, segmentId)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("repoCh.SegmentReceivedAt: %w", err)
		}
		return at, found, nil
	}

	v, err, _ := c.group.Do(strconv.FormatInt(projectId, 10)+"/"+replayId, func() (any, error) {
		// загрузку ждут и другие запросы: обрыв соединения первого не должен её отменять
		days, err := c.repoCh.SegmentsReceivedAt(context.WithoutCancel(ctx), projectId, replayId)
		if err != nil {
			return nil, fmt.Errorf("repoCh.SegmentsReceivedAt: %w", err)
		}
		// несуществующие записи не запоминаются: перебор номеров не должен вытеснять настоящие
		if len(days) > 0 {
			c.put(key, days)
		}
		return days, nil
	})
	if err != nil {
		return time.Time{}, false, err
	}

	at, found := v.(map[int]time.Time)[segmentId]
	return at, found, nil
}

func (c *segDays) put(key segDaysKey, days map[int]time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.items) >= segDaysMax {
		for k, v := range c.items {
			if time.Since(v.loadedAt) >= segDaysTtl {
				delete(c.items, k)
			}
		}
		// все свежие — сбрасываем целиком: это память на две минуты, а не хранилище
		if len(c.items) >= segDaysMax {
			clear(c.items)
		}
	}
	c.items[key] = segDaysItem{loadedAt: time.Now(), days: days}
}
