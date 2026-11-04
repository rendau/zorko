package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/rendau/zorko/internal/constant"
	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	artifactService "github.com/rendau/zorko/internal/domain/artifact/service"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	"github.com/rendau/zorko/internal/infra/httpx"
	"github.com/rendau/zorko/internal/service/sitefetch/model"
)

// Причины неудачи (совпадают с кодами кадров в symbolicate)
const (
	CodeHostNotAllowed  = "site_host_not_allowed"
	CodeTimeout         = "site_timeout"
	CodeNoSourcemapUrl  = "site_no_sourcemap_url"
	CodeTooLarge        = "site_too_large"
	CodeUnavailable     = "site_unavailable"
	CodeMapInvalidShape = "map_invalid"
	codeHttpPrefix      = "site_http_"

	stateOk    = "ok"
	stateError = "error"

	maxRedirects = 3
)

var (
	errBlockedAddress = errors.New("blocked address")
	errHostNotAllowed = errors.New("host is not allowed")
)

type Config struct {
	Concurrency    int
	RequestTimeout time.Duration
	MaxBytes       int64
	// AllowLoopback — только для тестов (httptest слушает 127.0.0.1)
	AllowLoopback bool
}

type Service struct {
	conf     Config
	project  projectServiceI
	artifact artifactServiceI
	client   *http.Client
	sem      chan struct{}
	group    singleflight.Group
}

func New(conf Config, project projectServiceI, artifact artifactServiceI) *Service {
	if conf.Concurrency <= 0 {
		conf.Concurrency = 4
	}
	if conf.RequestTimeout == 0 {
		conf.RequestTimeout = 5 * time.Second
	}
	if conf.MaxBytes == 0 {
		conf.MaxBytes = 50 << 20
	}
	s := &Service{conf: conf, project: project, artifact: artifact, sem: make(chan struct{}, conf.Concurrency)}
	s.client = httpx.New(httpx.Config{
		MaxConnsPerHost: conf.Concurrency,
		DialControl:     s.dialControl,
		NoProxy:         true, // через прокси проверка адреса при соединении не работает
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			if hosts, ok := req.Context().Value(hostsKey{}).([]string); ok && !hostAllowed(req.URL, hosts) {
				return errHostNotAllowed
			}
			return nil
		},
	})
	return s
}

type hostsKey struct{}

// dialControl — проверка адреса после DNS, перед соединением: подмена DNS разрешённого хоста не уведёт
// на адреса самого сервера и метаданные облака. Частные сети разрешены: защиту даёт список хостов.
func (s *Service) dialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errBlockedAddress
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return errBlockedAddress
	}
	ip = ip.Unmap()
	if ip.IsLoopback() && s.conf.AllowLoopback {
		return nil
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return errBlockedAddress
	}
	return nil
}

func hostAllowed(u *url.URL, hosts []string) bool {
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	h, hn := strings.ToLower(u.Host), strings.ToLower(u.Hostname())
	for _, allowed := range hosts {
		if allowed == h || allowed == hn {
			return true
		}
	}
	return false
}

func (s *Service) Fetch(ctx context.Context, projectId int64, absUrl string) (*model.Result, error) {
	p, found, err := s.project.Get(ctx, projectId, false)
	if err != nil {
		return nil, fmt.Errorf("project.Get: %w", err)
	}
	if !found || !p.SiteFetchEnabled {
		return nil, nil //nolint:nilnil // выключено
	}
	u, err := url.Parse(absUrl)
	if err != nil || !hostAllowed(u, p.SiteFetchHost) {
		return &model.Result{Code: CodeHostNotAllowed}, nil
	}
	u.Fragment = ""
	jsUrl := u.String()

	// память по адресу: не чаще интервала проекта — ни при удаче, ни при неудаче
	row, found, err := s.artifact.GetSiteFetch(ctx, projectId, jsUrl)
	if err != nil {
		return nil, fmt.Errorf("artifact.GetSiteFetch: %w", err)
	}
	interval := time.Duration(max(p.SiteFetchIntervalMin, 1)) * time.Minute
	if found && time.Since(row.FetchedAt) < interval {
		if row.State != stateOk {
			return &model.Result{Code: row.Detail}, nil
		}
		if row.BundleId != nil {
			if res, err := s.fromBundle(ctx, *row.BundleId); err != nil || res != nil {
				return res, err
			}
		}
	}

	// тысяча одинаковых ошибок — один запрос на сайт
	v, err, _ := s.group.Do(strconv.FormatInt(projectId, 10)+"|"+jsUrl, func() (any, error) {
		return s.fetchAndStore(ctx, p, u)
	})
	if err != nil {
		return nil, err
	}
	return v.(*model.Result), nil
}

// fromBundle — файлы ранее скачанного архива; архив удалён — nil (скачать заново)
func (s *Service) fromBundle(ctx context.Context, bundleId int64) (*model.Result, error) {
	files, err := s.artifact.FilesOfBundle(ctx, bundleId, nil)
	if err != nil {
		return nil, fmt.Errorf("artifact.FilesOfBundle: %w", err)
	}
	res := &model.Result{BundleId: bundleId}
	for _, f := range files {
		switch f.Type {
		case artifactModel.FileTypeMinified:
			res.JsBlob = f.Blob
		case artifactModel.FileTypeSourceMap:
			res.MapBlob, res.MapUrl = f.Blob, f.Url
		}
	}
	if res.MapBlob == "" {
		return nil, nil //nolint:nilnil // архива уже нет
	}
	return res, nil
}

func (s *Service) fetchAndStore(ctx context.Context, p *projectModel.Main, jsUrl *url.URL) (*model.Result, error) {
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.sem }()

	ctx = context.WithValue(ctx, hostsKey{}, p.SiteFetchHost)
	res, err := s.fetch(ctx, p, jsUrl)
	if err != nil {
		return nil, err
	}

	row := &artifactModel.SiteFetch{ProjectId: p.Id, Url: jsUrl.String(), State: stateOk}
	if res.Code != "" {
		row.State, row.Detail = stateError, res.Code
		slog.Info("sitefetch: no source map", "project_id", p.Id, "url", row.Url, "code", res.Code)
	} else {
		row.BundleId = new(res.BundleId)
		slog.Info("sitefetch: source map fetched", "project_id", p.Id, "url", row.Url, "map", res.MapUrl)
	}
	metricFetchTotal.WithLabelValues(resultLabel(res.Code)).Inc()
	if err = s.artifact.SaveSiteFetch(ctx, row); err != nil {
		return nil, fmt.Errorf("artifact.SaveSiteFetch: %w", err)
	}
	return res, nil
}

func resultLabel(code string) string {
	if code == "" {
		return "ok"
	}
	if strings.HasPrefix(code, codeHttpPrefix) {
		return "site_http"
	}
	return code
}

func (s *Service) fetch(ctx context.Context, p *projectModel.Main, jsUrl *url.URL) (*model.Result, error) {
	js, header, code := s.get(ctx, p, jsUrl)
	if code != "" {
		return &model.Result{Code: code}, nil
	}

	ref := header.Get("SourceMap")
	if ref == "" {
		ref = header.Get("X-SourceMap")
	}
	if ref == "" {
		ref = sourceMappingUrl(js)
	}
	if ref == "" {
		return &model.Result{Code: CodeNoSourcemapUrl}, nil
	}

	var mapData []byte
	mapUrl := jsUrl.String() + ".map"
	if strings.HasPrefix(ref, "data:") {
		if mapData = decodeDataUrl(ref); mapData == nil {
			return &model.Result{Code: CodeMapInvalidShape}, nil
		}
	} else {
		refUrl, err := url.Parse(ref)
		if err != nil {
			return &model.Result{Code: CodeNoSourcemapUrl}, nil
		}
		mu := jsUrl.ResolveReference(refUrl)
		if !hostAllowed(mu, p.SiteFetchHost) {
			return &model.Result{Code: CodeHostNotAllowed}, nil
		}
		if mapData, _, code = s.get(ctx, p, mu); code != "" {
			return &model.Result{Code: code}, nil
		}
		mapUrl = mu.String()
	}

	jsBlob, err := s.artifact.StoreFile(ctx, js)
	if err != nil {
		return nil, fmt.Errorf("artifact.StoreFile: %w", err)
	}
	mapBlob, err := s.artifact.StoreFile(ctx, mapData)
	if err != nil {
		return nil, fmt.Errorf("artifact.StoreFile: %w", err)
	}

	b, claimed, err := s.artifact.ClaimBundle(ctx, &artifactModel.NewBundle{
		OrgId: p.OrgId, Checksum: artifactService.Sha1Hex([]byte(jsBlob + mapBlob)), Source: artifactModel.SourceSite,
		ProjectIds: []int64{p.Id},
	}, 10*time.Minute, time.Minute)
	if err != nil {
		return nil, fmt.Errorf("artifact.ClaimBundle: %w", err)
	}
	jsPath, mapPath := "~"+jsUrl.Path, tildePath(mapUrl)
	if claimed {
		err = s.artifact.CompleteBundle(ctx, b.Id, "", "", []*artifactModel.File{
			{Url: jsPath, Type: artifactModel.FileTypeMinified, Sourcemap: mapPath, Blob: jsBlob, Size: int64(len(js))},
			{Url: mapPath, Type: artifactModel.FileTypeSourceMap, Blob: mapBlob, Size: int64(len(mapData))},
		})
		if err != nil {
			return nil, fmt.Errorf("artifact.CompleteBundle: %w", err)
		}
	}
	return &model.Result{BundleId: b.Id, JsBlob: jsBlob, MapBlob: mapBlob, MapUrl: mapPath}, nil
}

func tildePath(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return "~" + u.Path
	}
	return raw
}

// get — тело ответа или код причины
func (s *Service) get(ctx context.Context, p *projectModel.Main, u *url.URL) ([]byte, http.Header, string) {
	rctx, cancel := context.WithTimeout(ctx, s.conf.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, CodeUnavailable
	}
	req.Header.Set("User-Agent", "zorko/"+constant.Version+" (source map fetcher)")
	if p.SiteFetchHeaderName != "" && p.SiteFetchHeaderValue != "" {
		req.Header.Set(p.SiteFetchHeaderName, p.SiteFetchHeaderValue)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, errCode(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, nil, codeHttpPrefix + strconv.Itoa(resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, s.conf.MaxBytes+1))
	if err != nil {
		return nil, nil, errCode(err)
	}
	if int64(len(body)) > s.conf.MaxBytes {
		return nil, nil, CodeTooLarge
	}
	return body, resp.Header, ""
}

func errCode(err error) string {
	switch {
	case errors.Is(err, errBlockedAddress), errors.Is(err, errHostNotAllowed):
		return CodeHostNotAllowed
	case errors.Is(err, context.DeadlineExceeded):
		return CodeTimeout
	}
	if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
		return CodeTimeout
	}
	return CodeUnavailable
}

// sourceMappingUrl — ссылка на карту из последнего комментария //# sourceMappingURL= (или //@, /*#)
func sourceMappingUrl(js []byte) string {
	tail := js
	if len(tail) > 4096 {
		tail = tail[len(tail)-4096:]
	}
	idx := bytes.LastIndex(tail, []byte("sourceMappingURL="))
	if idx < 2 {
		if idx = bytes.LastIndex(js, []byte("sourceMappingURL=data:")); idx < 2 {
			return ""
		}
		tail = js
	}
	prefix := tail[max(0, idx-4):idx]
	if !bytes.Contains(prefix, []byte("//#")) && !bytes.Contains(prefix, []byte("//@")) && !bytes.Contains(prefix, []byte("/*#")) &&
		!bytes.Contains(prefix, []byte("/*@")) {
		return ""
	}
	ref := string(tail[idx+len("sourceMappingURL="):])
	if i := strings.IndexAny(ref, "\n\r \t"); i >= 0 {
		ref = ref[:i]
	}
	return strings.TrimSuffix(ref, "*/")
}
