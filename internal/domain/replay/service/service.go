package service

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"time"

	"github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/errs"
)

// MaxLinksPerReplay — отметок ошибок на шкале одной записи
const MaxLinksPerReplay = 500

const gcLockKey = 7302002 // zorko: уборка записей сессий

type Service struct {
	repoCh   RepoChI
	repoBlob RepoBlobI // nil — S3 не настроен: тела сегментов недоступны
	repoDb   RepoDbI
}

func New(repoCh RepoChI, repoBlob RepoBlobI, repoDb RepoDbI) *Service {
	return &Service{repoCh: repoCh, repoBlob: repoBlob, repoDb: repoDb}
}

// PutSegmentBody кладёт тело сегмента в S3; несжатое (голый JSON) сжимает zlib, чтобы в хранилище был
// один вид (ADR-012). Возвращает размер записанного.
func (s *Service) PutSegmentBody(ctx context.Context, seg *model.Segment, body []byte, compressed bool) (int64, error) {
	if s.repoBlob == nil {
		return 0, errs.ReplayDisabled
	}
	if !compressed {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		if _, err := zw.Write(body); err != nil {
			return 0, fmt.Errorf("zlib.Write: %w", err)
		}
		if err := zw.Close(); err != nil {
			return 0, fmt.Errorf("zlib.Close: %w", err)
		}
		body = buf.Bytes()
	}
	if err := s.repoBlob.Put(ctx, model.SegmentKey(seg.ReceivedAt, seg.ProjectId, seg.ReplayId, seg.SegmentId), body); err != nil {
		return 0, fmt.Errorf("repoBlob.Put: %w", err)
	}
	return int64(len(body)), nil
}

// SegmentBody — тело сегмента (zlib); нет описания или тела — errs.ReplayNotFound
func (s *Service) SegmentBody(ctx context.Context, projectId int64, replayId string, segmentId int, maxBytes int64) ([]byte, error) {
	if s.repoBlob == nil {
		return nil, errs.ReplayDisabled
	}
	replayId = model.NormalizeId(replayId)
	if replayId == "" || segmentId < 0 || segmentId > model.MaxSegmentId {
		return nil, errs.ReplayNotFound
	}
	receivedAt, found, err := s.repoCh.SegmentReceivedAt(ctx, projectId, replayId, segmentId)
	if err != nil {
		return nil, fmt.Errorf("repoCh.SegmentReceivedAt: %w", err)
	}
	if !found {
		return nil, errs.ReplayNotFound
	}
	data, found, err := s.repoBlob.Get(ctx, model.SegmentKey(receivedAt, projectId, replayId, segmentId), maxBytes)
	if err != nil {
		return nil, fmt.Errorf("repoBlob.Get: %w", err)
	}
	if !found {
		return nil, errs.ReplayNotFound
	}
	return data, nil
}

func (s *Service) InsertSegments(ctx context.Context, segments []*model.Segment) error {
	if err := s.repoCh.InsertSegments(ctx, segments); err != nil {
		return fmt.Errorf("repoCh.InsertSegments: %w", err)
	}
	return nil
}

func (s *Service) InsertLinks(ctx context.Context, links []*model.Link) error {
	if err := s.repoCh.InsertLinks(ctx, links); err != nil {
		return fmt.Errorf("repoCh.InsertLinks: %w", err)
	}
	return nil
}

func (s *Service) List(ctx context.Context, pars *model.ListReq) ([]*model.Summary, int64, error) {
	items, total, err := s.repoCh.List(ctx, pars)
	if err != nil {
		return nil, 0, fmt.Errorf("repoCh.List: %w", err)
	}
	return items, total, nil
}

// Get — запись проекта; нет — errs.ReplayNotFound
func (s *Service) Get(ctx context.Context, projectId int64, replayId string) (*model.Summary, error) {
	replayId = model.NormalizeId(replayId)
	if replayId == "" {
		return nil, errs.ReplayNotFound
	}
	item, found, err := s.repoCh.Get(ctx, projectId, replayId)
	if err != nil {
		return nil, fmt.Errorf("repoCh.Get: %w", err)
	}
	if !found {
		return nil, errs.ReplayNotFound
	}
	return item, nil
}

func (s *Service) IssueReplays(ctx context.Context, projectId, issueId int64, page, pageSize int64) ([]*model.Summary, int64, error) {
	items, total, err := s.repoCh.IssueReplays(ctx, projectId, issueId, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("repoCh.IssueReplays: %w", err)
	}
	return items, total, nil
}

func (s *Service) Links(ctx context.Context, projectId int64, replayId string) ([]*model.Link, error) {
	items, err := s.repoCh.Links(ctx, projectId, replayId, MaxLinksPerReplay)
	if err != nil {
		return nil, fmt.Errorf("repoCh.Links: %w", err)
	}
	return items, nil
}

// уборка (ADR-012): папки дней в S3 старше срока хранения; ClickHouse убирает себя сам по TTL

func (s *Service) GcLock(ctx context.Context) (func(), bool, error) {
	unlock, ok, err := s.repoDb.TryLock(ctx, gcLockKey)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.TryLock: %w", err)
	}
	return unlock, ok, nil
}

// DeleteOldDays удаляет дни, начавшиеся раньше before; возвращает число удалённых сегментов
func (s *Service) DeleteOldDays(ctx context.Context, before time.Time) (int, error) {
	if s.repoBlob == nil {
		return 0, nil
	}
	days, err := s.repoBlob.Days(ctx)
	if err != nil {
		return 0, fmt.Errorf("repoBlob.Days: %w", err)
	}
	total := 0
	for _, day := range days {
		// день целиком раньше границы: его конец (начало следующего) не позже before
		if day.AddDate(0, 0, 1).After(before) {
			continue
		}
		n, err := s.repoBlob.DeleteDay(ctx, day)
		total += n
		if err != nil {
			return total, fmt.Errorf("repoBlob.DeleteDay %s: %w", day.Format("20060102"), err)
		}
	}
	return total, nil
}
