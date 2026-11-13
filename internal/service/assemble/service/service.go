package service

import (
	"archive/zip"
	"context"
	"crypto/sha1" //nolint:gosec // протокол sentry-cli
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	artifactService "github.com/rendau/zorko/internal/domain/artifact/service"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/service/assemble/model"
)

// Пределы против мусора (спека, раздел 3)
const (
	MaxBundleBytes     = 1 << 30
	MaxUnpackedBytes   = 2 << 30
	MaxFiles           = 20000
	MaxChunksPerBundle = MaxBundleBytes / (1 << 20) // куски не меньше 1 МБ, кроме последнего

	// MaxInFlightBytes — сколько распакованных байт файлов одновременно держат в памяти все сборки
	// копии. Файл читается целиком (SHA-1 нужен до записи в S3), и без предела восемь файлов по
	// 200 МБ заняли бы гигабайты. Файл больше предела идёт один.
	MaxInFlightBytes = 256 << 20
)

type Config struct {
	TmpDir      string
	Concurrency int
	// StaleAfter — «зависшая» сборка (копия упала) занимается заново
	StaleAfter time.Duration
	// ErrorRetryAfter — после ошибки повторная заливка того же архива начинает сборку заново
	ErrorRetryAfter time.Duration
	Timeout         time.Duration

	// пределы; нули → значения по умолчанию
	MaxBundleBytes   int64
	MaxFileBytes     int64
	MaxUnpackedBytes int64
	MaxFiles         int
	MaxInFlightBytes int64
}

type Service struct {
	conf     Config
	artifact artifactServiceI
	sem      chan struct{}
	fileSem  *semaphore.Weighted // распакованные байты файлов в работе, см. MaxInFlightBytes
	wg       sync.WaitGroup

	mu      sync.Mutex
	rootCtx context.Context
}

func New(conf Config, artifact artifactServiceI) *Service {
	if conf.Concurrency <= 0 {
		conf.Concurrency = 2
	}
	if conf.StaleAfter == 0 {
		conf.StaleAfter = 10 * time.Minute
	}
	if conf.ErrorRetryAfter == 0 {
		conf.ErrorRetryAfter = time.Minute
	}
	if conf.Timeout == 0 {
		conf.Timeout = 30 * time.Minute
	}
	if conf.MaxBundleBytes == 0 {
		conf.MaxBundleBytes = MaxBundleBytes
	}
	if conf.MaxFileBytes == 0 {
		conf.MaxFileBytes = artifactService.MaxFileBytes
	}
	if conf.MaxUnpackedBytes == 0 {
		conf.MaxUnpackedBytes = MaxUnpackedBytes
	}
	if conf.MaxFiles == 0 {
		conf.MaxFiles = MaxFiles
	}
	if conf.MaxInFlightBytes <= 0 {
		conf.MaxInFlightBytes = MaxInFlightBytes
	}
	return &Service{
		conf: conf, artifact: artifact, sem: make(chan struct{}, conf.Concurrency),
		fileSem: semaphore.NewWeighted(conf.MaxInFlightBytes), rootCtx: context.Background(),
	}
}

// Start — корневой контекст фоновых сборок (отменяется при остановке приложения)
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.rootCtx = ctx
	s.mu.Unlock()
}

func (s *Service) Wait() {
	s.wg.Wait()
}

func (s *Service) Assemble(ctx context.Context, req *model.Request) (*model.Result, error) {
	if !artifactService.IsSha1(req.Checksum) {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "checksum must be sha1 hex"}
	}
	if len(req.Chunks) == 0 || len(req.Chunks) > MaxChunksPerBundle {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: fmt.Sprintf("chunks: from 1 to %d expected", MaxChunksPerBundle)}
	}
	for _, c := range req.Chunks {
		if !artifactService.IsSha1(c) {
			return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "chunk checksum must be sha1 hex: " + c}
		}
	}

	// готовый архив, идущая сборка или свежая ошибка — куски не нужны (после сборки их уже нет)
	existing, found, err := s.artifact.GetBundleByChecksum(ctx, req.OrgId, req.Checksum)
	if err != nil {
		return nil, fmt.Errorf("artifact.GetBundleByChecksum: %w", err)
	}
	if found {
		if existing.State == artifactModel.StateOk {
			if err = s.artifact.AddBundleProjects(ctx, existing.Id, req.ProjectIds); err != nil {
				return nil, fmt.Errorf("artifact.AddBundleProjects: %w", err)
			}
			return &model.Result{State: model.StateOk}, nil
		}
		if res, busy := s.busyResult(existing); busy {
			return res, nil
		}
	}

	missing, err := s.artifact.MissingChunks(ctx, req.Chunks)
	if err != nil {
		return nil, fmt.Errorf("artifact.MissingChunks: %w", err)
	}
	if len(missing) > 0 {
		return &model.Result{State: model.StateNotFound, MissingChunks: missing}, nil
	}

	b, claimed, err := s.artifact.ClaimBundle(ctx, &artifactModel.NewBundle{
		OrgId: req.OrgId, Checksum: req.Checksum, Source: artifactModel.SourceUpload,
		Release: req.Release, Dist: req.Dist, ProjectIds: req.ProjectIds,
	}, s.conf.StaleAfter, s.conf.ErrorRetryAfter)
	if err != nil {
		return nil, fmt.Errorf("artifact.ClaimBundle: %w", err)
	}
	if !claimed {
		return resultOf(b), nil
	}

	s.wg.Go(func() { s.run(b, req) })
	return &model.Result{State: model.StateCreated}, nil
}

// busyResult — сборка ещё идёт или недавно упала: ответить её состоянием, не занимая заново
func (s *Service) busyResult(b *artifactModel.Bundle) (*model.Result, bool) {
	now := time.Now()
	switch b.State {
	case artifactModel.StateAssembling:
		if now.Sub(b.CreatedAt) < s.conf.StaleAfter {
			return &model.Result{State: model.StateAssembling}, true
		}
	case artifactModel.StateError:
		if b.AssembledAt != nil && now.Sub(*b.AssembledAt) < s.conf.ErrorRetryAfter {
			return &model.Result{State: model.StateError, Detail: b.Detail}, true
		}
	}
	return nil, false
}

func resultOf(b *artifactModel.Bundle) *model.Result {
	switch b.State {
	case artifactModel.StateOk:
		return &model.Result{State: model.StateOk}
	case artifactModel.StateError:
		return &model.Result{State: model.StateError, Detail: b.Detail}
	default:
		return &model.Result{State: model.StateAssembling}
	}
}

func (s *Service) run(b *artifactModel.Bundle, req *model.Request) {
	s.mu.Lock()
	root := s.rootCtx
	s.mu.Unlock()

	select {
	case s.sem <- struct{}{}:
	case <-root.Done():
		return
	}
	defer func() { <-s.sem }()

	ctx, cancel := context.WithTimeout(root, s.conf.Timeout)
	defer cancel()

	start := time.Now()
	err := s.assembleBundle(ctx, b, req)
	metricAssembleDuration.Observe(time.Since(start).Seconds())
	if err == nil {
		metricAssembleTotal.WithLabelValues(resultOk).Inc()
		return
	}

	detail := "internal error"
	result := resultFailed
	if full, ok := errs.AsErrFull(err); ok && errors.Is(full.Err, errs.BundleInvalid) {
		detail, result = full.Desc, resultInvalid
		slog.Info("assemble: bundle rejected", "bundle_id", b.Id, "org_id", req.OrgId, "detail", detail)
	} else {
		slog.Error("assemble: failed", "bundle_id", b.Id, "org_id", req.OrgId, "error", err)
	}
	metricAssembleTotal.WithLabelValues(result).Inc()

	// состояние пишем и после отмены корневого контекста — иначе sentry-cli будет ждать до таймаута
	failCtx, failCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer failCancel()
	if err = s.artifact.FailBundle(failCtx, b.Id, detail); err != nil {
		slog.Error("assemble: FailBundle", "bundle_id", b.Id, "error", err)
	}
}

func invalid(format string, args ...any) error {
	return errs.ErrFull{Err: errs.BundleInvalid, Desc: fmt.Sprintf(format, args...)}
}

func (s *Service) assembleBundle(ctx context.Context, b *artifactModel.Bundle, req *model.Request) error {
	tmp, err := os.CreateTemp(s.conf.TmpDir, "zorko-bundle-*.zip")
	if err != nil {
		return fmt.Errorf("CreateTemp: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	// куски по порядку → временный файл, отпечаток на лету
	hasher := sha1.New() //nolint:gosec
	var size int64
	for _, c := range req.Chunks {
		n, err := s.copyChunk(ctx, c, io.MultiWriter(tmp, hasher), s.conf.MaxBundleBytes-size)
		size += n
		if err != nil {
			return err
		}
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != req.Checksum {
		return invalid("bundle checksum mismatch: expected %s, got %s", req.Checksum, got)
	}

	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		return invalid("not a zip archive: %v", err)
	}
	mf, err := readManifest(zr, s.conf.MaxFiles)
	if err != nil {
		return err
	}
	files, err := s.storeFiles(ctx, zr, mf)
	if err != nil {
		return err
	}

	release, dist := req.Release, req.Dist
	if release == "" {
		release, dist = mf.Release, mf.Dist
	}
	if err = s.artifact.CompleteBundle(ctx, b.Id, release, dist, files); err != nil {
		return fmt.Errorf("artifact.CompleteBundle: %w", err)
	}
	if err = s.artifact.DeleteChunks(ctx, req.Chunks); err != nil {
		slog.Warn("assemble: DeleteChunks", "bundle_id", b.Id, "error", err) // уберёт уборка
	}
	slog.Info("assemble: bundle ready", "bundle_id", b.Id, "org_id", req.OrgId, "files", len(files), "bytes", size,
		"release", release)
	return nil
}

func (s *Service) copyChunk(ctx context.Context, checksum string, w io.Writer, left int64) (int64, error) {
	r, err := s.artifact.OpenChunk(ctx, checksum)
	if err != nil {
		return 0, fmt.Errorf("artifact.OpenChunk: %w", err)
	}
	defer r.Close()
	n, err := io.Copy(w, io.LimitReader(r, left+1))
	if err != nil {
		return n, fmt.Errorf("read chunk %s: %w", checksum, err)
	}
	if n > left {
		return n, invalid("bundle is larger than %d bytes", s.conf.MaxBundleBytes)
	}
	return n, nil
}

// manifestRep — manifest.json архива sentry-cli
type manifestRep struct {
	Files   map[string]manifestFileRep `json:"files"`
	Org     string                     `json:"org"`
	Release string                     `json:"release"`
	Dist    string                     `json:"dist"`
}

type manifestFileRep struct {
	Url     string            `json:"url"`
	Type    string            `json:"type"`
	Headers map[string]string `json:"headers"`
}

func readManifest(zr *zip.Reader, maxFiles int) (*manifestRep, error) {
	f, err := zr.Open("manifest.json")
	if err != nil {
		return nil, invalid("manifest.json not found")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return nil, invalid("manifest.json: %v", err)
	}
	mf := &manifestRep{}
	if err = json.Unmarshal(data, mf); err != nil {
		return nil, invalid("manifest.json: %v", err)
	}
	if len(mf.Files) > maxFiles {
		return nil, invalid("too many files: %d, at most %d", len(mf.Files), maxFiles)
	}
	return mf, nil
}

var knownTypes = map[string]bool{
	artifactModel.FileTypeMinified:  true,
	artifactModel.FileTypeSourceMap: true,
	artifactModel.FileTypeSource:    true,
}

func header(h map[string]string, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// fileJob — файл архива, прошедший проверку пределов
type fileJob struct {
	path  string
	mf    manifestFileRep
	entry *zip.File
	size  int64 // распакованный размер из оглавления архива
}

func (s *Service) storeFiles(ctx context.Context, zr *zip.Reader, mf *manifestRep) ([]*artifactModel.File, error) {
	entries := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		entries[f.Name] = f
	}

	// пределы — по размерам из оглавления архива, до чтения: негодный архив не занимает память.
	// Настоящий размер с заявленным сверяется при чтении (readEntry).
	jobs := make([]fileJob, 0, len(mf.Files))
	var unpacked uint64
	for path, mfFile := range mf.Files {
		if !knownTypes[mfFile.Type] || mfFile.Url == "" {
			continue // indexed_ram_bundle и прочее — шаг 6
		}
		entry, ok := entries[path]
		if !ok {
			return nil, invalid("file %s from manifest is missing in the archive", path)
		}
		if entry.UncompressedSize64 > uint64(s.conf.MaxFileBytes) {
			return nil, invalid("file %s is larger than %d bytes", path, s.conf.MaxFileBytes)
		}
		if unpacked += entry.UncompressedSize64; unpacked > uint64(s.conf.MaxUnpackedBytes) {
			return nil, invalid("archive unpacks to more than %d bytes", s.conf.MaxUnpackedBytes)
		}
		jobs = append(jobs, fileJob{path: path, mf: mfFile, entry: entry, size: int64(entry.UncompressedSize64)})
	}

	files := make([]*artifactModel.File, 0, len(jobs))
	var mu sync.Mutex
	var acquireErr error

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for _, job := range jobs {
		// память под файл занимается до его чтения и держится до конца записи в S3.
		// Файл больше всего предела берёт предел целиком: дожидается остальных и идёт один.
		weight := min(job.size, s.conf.MaxInFlightBytes)
		if acquireErr = s.fileSem.Acquire(gCtx, weight); acquireErr != nil {
			break // сборка отменена или другой файл не прошёл: причину вернёт g.Wait
		}
		g.Go(func() error {
			defer s.fileSem.Release(weight)

			data, err := readEntry(job.entry, job.size)
			if err != nil {
				return invalid("file %s: %v", job.path, err)
			}
			blob, err := s.artifact.StoreFile(gCtx, data)
			if err != nil {
				return fmt.Errorf("artifact.StoreFile: %w", err)
			}
			f := &artifactModel.File{
				Url:       job.mf.Url,
				Type:      job.mf.Type,
				DebugId:   artifactService.NormalizeDebugId(header(job.mf.Headers, "debug-id")),
				Sourcemap: header(job.mf.Headers, "sourcemap"),
				Blob:      blob,
				Size:      int64(len(data)),
			}
			mu.Lock()
			files = append(files, f)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if acquireErr != nil {
		return nil, fmt.Errorf("fileSem.Acquire: %w", acquireErr)
	}
	return files, nil
}

// readEntry — файл архива целиком, в буфере ровно под заявленный размер: io.ReadAll растит буфер с
// перекладыванием и на большом файле занимает вдвое больше. Заявленный размер сверяется с настоящим.
func readEntry(entry *zip.File, size int64) ([]byte, error) {
	rc, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data := make([]byte, size)
	if _, err = io.ReadFull(rc, data); err != nil {
		return nil, err // файл короче заявленного — io.ErrUnexpectedEOF
	}
	// дочитать до конца: файл не длиннее заявленного, и контрольную сумму zip сверяет только на конце
	var extra [1]byte
	if n, err := io.ReadFull(rc, extra[:]); n > 0 {
		return nil, errors.New("size does not match the archive header")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data, nil
}
