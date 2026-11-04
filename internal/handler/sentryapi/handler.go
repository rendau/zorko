// Package sentryapi — ручки Sentry API, которые зовут sentry-cli и @sentry/vite-plugin:
// заливка карт (chunk-upload, artifactbundle/assemble) и релизы. Сервер приёма, вход — токен CI.
package sentryapi

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	"github.com/rendau/zorko/internal/errs"
	usecase "github.com/rendau/zorko/internal/usecase/upload"
)

const (
	maxJsonBody    = 1 << 20
	maxChunkBody   = usecase.MaxRequestSize + 4<<20 // + заголовки multipart
	chunkIoTimeout = 5 * time.Minute
)

type Handler struct {
	usecase *usecase.Usecase
}

func New(uc *usecase.Usecase) *Handler {
	return &Handler{usecase: uc}
}

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern, name string, f func(w http.ResponseWriter, r *http.Request) int) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			status := f(w, r)
			metricRequestTotal.WithLabelValues(name, strconv.Itoa(status)).Inc()
		})
	}

	route("GET /api/0/{$}", "auth_info", h.authInfo)
	route("GET /api/0/organizations/{org}/{$}", "org", h.org)
	route("GET /api/0/organizations/{org}/chunk-upload/{$}", "chunk_options", h.chunkOptions)
	route("POST /api/0/organizations/{org}/chunk-upload/{$}", "chunk_upload", h.chunkUpload)
	route("POST /api/0/organizations/{org}/artifactbundle/assemble/{$}", "assemble", h.assemble)
	route("POST /api/0/organizations/{org}/releases/{$}", "release_create", h.releaseCreate)
	route("GET /api/0/organizations/{org}/releases/{version}/{$}", "release_get", h.releaseGet)
	route("PUT /api/0/organizations/{org}/releases/{version}/{$}", "release_update", h.releaseUpdate)
	route("POST /api/0/organizations/{org}/releases/{version}/deploys/{$}", "deploy_create", h.deployCreate)
	route("GET /api/0/organizations/{org}/releases/{version}/deploys/{$}", "deploy_list", h.emptyList)
	route("GET /api/0/organizations/{org}/repos/{$}", "repos", h.emptyList)
	route("/api/0/", "unknown", h.unknown)
	return mux
}

// helpers

func writeJson(w http.ResponseWriter, status int, v any) int {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
	return status
}

type detailRep struct {
	Detail string `json:"detail"`
}

// writeErr — ошибка в формате Sentry {"detail": "..."}: sentry-cli покажет текст в логе сборки
func writeErr(w http.ResponseWriter, r *http.Request, err error) int {
	status, detail := errStatus(err)
	if status >= 500 {
		slog.Error("sentryapi: failure", "path", r.URL.Path, "error", err)
	} else {
		slog.Info("sentryapi: rejected", "path", r.URL.Path, "status", status, "detail", detail)
	}
	return writeJson(w, status, detailRep{Detail: detail})
}

func errStatus(err error) (int, string) {
	desc, base := "", err
	if full, ok := errs.AsErrFull(err); ok {
		desc, base = full.Desc, full.Err
	}
	code, _ := errs.AsErr(base)
	text := func(def string) string { return lo.Ternary(desc != "", desc, def) }
	switch code {
	case errs.TokenInvalid:
		return http.StatusUnauthorized, "Invalid token"
	case errs.NoPermission:
		return http.StatusForbidden, text("You do not have permission to perform this action.")
	case errs.SourcemapsDisabled:
		return http.StatusNotFound, "source maps are not configured on this zorko server (S3_ENDPOINT)"
	case errs.ProjectNotFound, errs.OrgNotFound, errs.ObjectNotFound:
		return http.StatusNotFound, text("The requested resource does not exist")
	case errs.ReleaseNotFound:
		return http.StatusNotFound, "Release not found"
	case errs.InvalidRequest, errs.ChunkInvalid, errs.BundleInvalid:
		return http.StatusBadRequest, text("invalid request")
	}
	return http.StatusServiceUnavailable, "internal error"
}

func bearer(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("Authorization"))
	for _, prefix := range []string{"Bearer ", "bearer ", "Token ", "token "} {
		if strings.HasPrefix(v, prefix) {
			return strings.TrimSpace(v[len(prefix):])
		}
	}
	return ""
}

func (h *Handler) auth(r *http.Request) (*usecase.Auth, error) {
	return h.usecase.Authorize(r.Context(), bearer(r), r.PathValue("org"))
}

func readJson(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJsonBody+1))
	if err != nil {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "read body: " + err.Error()}
	}
	if len(body) > maxJsonBody {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "body is too large"}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err = json.Unmarshal(body, v); err != nil {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "invalid json: " + err.Error()}
	}
	return nil
}

// idSlugList — sentry-cli шлёт проекты числами (id) или строками (slug)
type idSlugList []string

func (l *idSlugList) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, item := range raw {
		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			*l = append(*l, s)
			continue
		}
		var n json.Number
		if err := json.Unmarshal(item, &n); err != nil {
			return fmt.Errorf("project must be a slug or an id: %s", item)
		}
		*l = append(*l, n.String())
	}
	return nil
}

// handlers

func (h *Handler) unknown(w http.ResponseWriter, r *http.Request) int {
	slog.Warn("sentryapi: unknown endpoint", "method", r.Method, "path", r.URL.Path, "user_agent", r.UserAgent())
	return writeJson(w, http.StatusNotFound, detailRep{Detail: "The requested resource does not exist"})
}

func (h *Handler) emptyList(w http.ResponseWriter, r *http.Request) int {
	if _, err := h.auth(r); err != nil {
		return writeErr(w, r, err)
	}
	return writeJson(w, http.StatusOK, []any{})
}

type authInfoRep struct {
	User any            `json:"user"`
	Auth authDetailsRep `json:"auth"`
}

type authDetailsRep struct {
	Scopes []string `json:"scopes"`
}

func (h *Handler) authInfo(w http.ResponseWriter, r *http.Request) int {
	if _, err := h.auth(r); err != nil {
		return writeErr(w, r, err)
	}
	return writeJson(w, http.StatusOK, authInfoRep{Auth: authDetailsRep{Scopes: []string{"org:ci", "project:releases"}}})
}

type orgRep struct {
	Id       string      `json:"id"`
	Slug     string      `json:"slug"`
	Name     string      `json:"name"`
	Features []string    `json:"features"`
	Links    *orgLinkRep `json:"links,omitempty"`
}

type orgLinkRep struct {
	OrganizationUrl string `json:"organizationUrl"`
	RegionUrl       string `json:"regionUrl"`
}

// org — новый JS-клиент Sentry (пакет sentry, им пользуется @sentry/vite-plugin) узнаёт тут адрес региона
func (h *Handler) org(w http.ResponseWriter, r *http.Request) int {
	auth, err := h.auth(r)
	if err != nil {
		return writeErr(w, r, err)
	}
	rep := orgRep{Id: strconv.FormatInt(auth.OrgId, 10), Slug: auth.OrgSlug, Name: lo.Ternary(auth.OrgName != "", auth.OrgName, auth.OrgSlug), Features: []string{}}
	if u := usecase.PublicUrl(); u != "" {
		rep.Links = &orgLinkRep{OrganizationUrl: u, RegionUrl: u}
	}
	return writeJson(w, http.StatusOK, rep)
}

type chunkOptionsRep struct {
	Url              string   `json:"url"`
	ChunkSize        int64    `json:"chunkSize"`
	ChunksPerRequest int64    `json:"chunksPerRequest"`
	MaxRequestSize   int64    `json:"maxRequestSize"`
	MaxFileSize      int64    `json:"maxFileSize"`
	MaxWait          int64    `json:"maxWait"`
	Concurrency      int64    `json:"concurrency"`
	HashAlgorithm    string   `json:"hashAlgorithm"`
	Compression      []string `json:"compression"`
	Accept           []string `json:"accept"`
}

func (h *Handler) chunkOptions(w http.ResponseWriter, r *http.Request) int {
	auth, err := h.auth(r)
	if err != nil {
		return writeErr(w, r, err)
	}
	opts, err := h.usecase.ChunkOptions(auth)
	if err != nil {
		return writeErr(w, r, err)
	}
	return writeJson(w, http.StatusOK, chunkOptionsRep{
		Url: opts.Url, ChunkSize: opts.ChunkSize, ChunksPerRequest: opts.ChunksPerRequest,
		MaxRequestSize: opts.MaxRequestSize, MaxFileSize: opts.MaxFileSize, MaxWait: 60,
		Concurrency: opts.Concurrency, HashAlgorithm: opts.HashAlgorithm,
		Compression: opts.Compression, Accept: opts.Accept,
	})
}

func (h *Handler) chunkUpload(w http.ResponseWriter, r *http.Request) int {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(chunkIoTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(chunkIoTimeout))

	auth, err := h.auth(r)
	if err != nil {
		return writeErr(w, r, err)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxChunkBody)
	mr, err := r.MultipartReader()
	if err != nil {
		return writeErr(w, r, errs.ErrFull{Err: errs.InvalidRequest, Desc: "multipart/form-data expected"})
	}

	count := 0
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return writeErr(w, r, errs.ErrFull{Err: errs.InvalidRequest, Desc: "multipart: " + err.Error()})
		}
		data, err := readChunkPart(part)
		_ = part.Close()
		if err != nil {
			return writeErr(w, r, err)
		}
		if err = h.usecase.PutChunk(r.Context(), auth, strings.ToLower(part.FileName()), data); err != nil {
			return writeErr(w, r, err)
		}
		metricUploadBytes.Add(float64(len(data)))
		count++
		if count > usecase.ChunksPerRequest {
			return writeErr(w, r, errs.ErrFull{Err: errs.InvalidRequest, Desc: "too many chunks in one request"})
		}
	}
	return writeJson(w, http.StatusOK, struct{}{})
}

// readChunkPart — часть file (как есть) или file_gzip (сжата); распакованный кусок не больше ChunkSize
func readChunkPart(part interface {
	io.Reader
	FormName() string
}) ([]byte, error) {
	var src io.Reader = part
	switch part.FormName() {
	case "file":
	case "file_gzip":
		zr, err := gzip.NewReader(part)
		if err != nil {
			return nil, errs.ErrFull{Err: errs.ChunkInvalid, Desc: "gzip: " + err.Error()}
		}
		defer zr.Close()
		src = zr
	default:
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "unexpected form field: " + part.FormName()}
	}
	data, err := io.ReadAll(io.LimitReader(src, usecase.ChunkSize+1))
	if err != nil {
		return nil, errs.ErrFull{Err: errs.ChunkInvalid, Desc: "read chunk: " + err.Error()}
	}
	if len(data) > usecase.ChunkSize {
		return nil, errs.ErrFull{Err: errs.ChunkInvalid, Desc: fmt.Sprintf("chunk is larger than %d bytes", usecase.ChunkSize)}
	}
	return data, nil
}

type assembleReq struct {
	Checksum string     `json:"checksum"`
	Chunks   []string   `json:"chunks"`
	Projects idSlugList `json:"projects"`
	Version  string     `json:"version"`
	Dist     string     `json:"dist"`
}

type assembleRep struct {
	State         string   `json:"state"`
	MissingChunks []string `json:"missingChunks"`
	Detail        *string  `json:"detail"`
}

func (h *Handler) assemble(w http.ResponseWriter, r *http.Request) int {
	auth, err := h.auth(r)
	if err != nil {
		return writeErr(w, r, err)
	}
	req := &assembleReq{}
	if err = readJson(r, req); err != nil {
		return writeErr(w, r, err)
	}
	res, err := h.usecase.Assemble(r.Context(), auth, &usecase.AssembleReq{
		Checksum: req.Checksum, Chunks: req.Chunks, Projects: req.Projects, Version: req.Version, Dist: req.Dist,
	})
	if err != nil {
		return writeErr(w, r, err)
	}
	rep := assembleRep{State: res.State, MissingChunks: lo.Ternary(res.MissingChunks == nil, []string{}, res.MissingChunks)}
	if res.Detail != "" {
		rep.Detail = new(res.Detail)
	}
	return writeJson(w, http.StatusOK, rep)
}

// releases

type releaseReq struct {
	Version      string     `json:"version"`
	Projects     idSlugList `json:"projects"`
	DateReleased *time.Time `json:"dateReleased"`
}

type releaseRep struct {
	Version      string              `json:"version"`
	ShortVersion string              `json:"shortVersion"`
	Url          *string             `json:"url"`
	DateCreated  time.Time           `json:"dateCreated"`
	DateReleased *time.Time          `json:"dateReleased"`
	LastEvent    *time.Time          `json:"lastEvent"`
	NewGroups    int64               `json:"newGroups"`
	Projects     []releaseProjectRep `json:"projects"`
	LastCommit   *struct{}           `json:"lastCommit"`
}

type releaseProjectRep struct {
	Id   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func encodeRelease(v *usecase.ReleaseInfo) releaseRep {
	return releaseRep{
		Version:      v.Version,
		ShortVersion: v.Version,
		DateCreated:  v.DateCreated.UTC(),
		DateReleased: v.DateReleased,
		Projects: lo.Map(v.Projects, func(p *projectModel.Main, _ int) releaseProjectRep {
			return releaseProjectRep{Id: strconv.FormatInt(p.Id, 10), Slug: p.Slug, Name: lo.Ternary(p.Name != "", p.Name, p.Slug)}
		}),
	}
}

func (h *Handler) releaseCreate(w http.ResponseWriter, r *http.Request) int {
	auth, err := h.auth(r)
	if err != nil {
		return writeErr(w, r, err)
	}
	req := &releaseReq{}
	if err = readJson(r, req); err != nil {
		return writeErr(w, r, err)
	}
	info, created, err := h.usecase.CreateRelease(r.Context(), auth, req.Version, req.Projects, req.DateReleased)
	if err != nil {
		return writeErr(w, r, err)
	}
	return writeJson(w, lo.Ternary(created, http.StatusCreated, http.StatusAlreadyReported), encodeRelease(info))
}

func (h *Handler) releaseGet(w http.ResponseWriter, r *http.Request) int {
	auth, err := h.auth(r)
	if err != nil {
		return writeErr(w, r, err)
	}
	info, err := h.usecase.GetRelease(r.Context(), auth, r.PathValue("version"))
	if err != nil {
		return writeErr(w, r, err)
	}
	return writeJson(w, http.StatusOK, encodeRelease(info))
}

func (h *Handler) releaseUpdate(w http.ResponseWriter, r *http.Request) int {
	auth, err := h.auth(r)
	if err != nil {
		return writeErr(w, r, err)
	}
	req := &releaseReq{}
	if err = readJson(r, req); err != nil {
		return writeErr(w, r, err)
	}
	info, err := h.usecase.UpdateRelease(r.Context(), auth, r.PathValue("version"), req.Projects, req.DateReleased)
	if err != nil {
		return writeErr(w, r, err)
	}
	return writeJson(w, http.StatusOK, encodeRelease(info))
}

type deployReq struct {
	Environment  string     `json:"environment"`
	Name         *string    `json:"name"`
	Url          *string    `json:"url"`
	DateStarted  *time.Time `json:"dateStarted"`
	DateFinished *time.Time `json:"dateFinished"`
}

type deployRep struct {
	Id           string     `json:"id"`
	Environment  string     `json:"environment"`
	Name         *string    `json:"name"`
	Url          *string    `json:"url"`
	DateStarted  *time.Time `json:"dateStarted"`
	DateFinished time.Time  `json:"dateFinished"`
}

// deployCreate — деплой принимается и не хранится (шаг 4); релиз должен существовать
func (h *Handler) deployCreate(w http.ResponseWriter, r *http.Request) int {
	auth, err := h.auth(r)
	if err != nil {
		return writeErr(w, r, err)
	}
	req := &deployReq{}
	if err = readJson(r, req); err != nil {
		return writeErr(w, r, err)
	}
	if _, err = h.usecase.GetRelease(r.Context(), auth, r.PathValue("version")); err != nil {
		return writeErr(w, r, err)
	}
	finished := time.Now().UTC()
	if req.DateFinished != nil {
		finished = *req.DateFinished
	}
	return writeJson(w, http.StatusCreated, deployRep{
		Id: "0", Environment: req.Environment, Name: req.Name, Url: req.Url,
		DateStarted: req.DateStarted, DateFinished: finished,
	})
}
