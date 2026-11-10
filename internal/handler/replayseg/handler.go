// Package replayseg — тело сегмента записи для проигрывателя (ADR-015): обычная HTTP-ручка на
// мультиплексоре шлюза, потому что бинарное тело через grpc-gateway не пропустить.
//
//	GET /replay/{id}/segment/{n}?project_id=  →  zlib как есть (application/octet-stream)
package replayseg

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/samber/lo"

	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	"github.com/rendau/zorko/internal/errs"
)

type usecaseI interface {
	SegmentBody(ctx context.Context, projectId int64, replayId string, segmentId int) ([]byte, error)
}

type sessionServiceI interface {
	WithContext(ctx context.Context, session *sessionModel.Session) context.Context
}

// authenticatorI — сессия по токену в режиме AUTH_MODE (internal/service/authn, ADR-019)
type authenticatorI interface {
	Authenticate(ctx context.Context, token string) (*sessionModel.Session, error)
}

type Handler struct {
	usecase       usecaseI
	sessionSvc    sessionServiceI
	authenticator authenticatorI
}

func New(uc usecaseI, sessionSvc sessionServiceI, authenticator authenticatorI) *Handler {
	return &Handler{usecase: uc, sessionSvc: sessionSvc, authenticator: authenticator}
}

// Segment — подпись runtime.HandlerFunc grpc-gateway
func (h *Handler) Segment(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
	// сессия — как в gRPC (GrpcInterceptorSession): без токена или с негодным — пустая, права решает usecase
	var session *sessionModel.Session
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		var err error
		if session, err = h.authenticator.Authenticate(r.Context(), token); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	ctx := h.sessionSvc.WithContext(r.Context(), session)

	projectId, _ := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)
	segmentId, err := strconv.Atoi(pathParams["n"])
	if err != nil {
		writeError(w, http.StatusBadRequest, errs.InvalidRequest, "segment number expected")
		return
	}

	body, err := h.usecase.SegmentBody(ctx, projectId, pathParams["id"], segmentId)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	// сегмент не меняется; private — только браузер вошедшего, не общие кэши
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(body)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	code := errs.Err(errs.Code(err))

	status := http.StatusBadRequest
	switch code {
	case errs.NotAuthorized:
		status = http.StatusUnauthorized
	case errs.NoPermission, errs.UsrDisabled:
		status = http.StatusForbidden
	case errs.ReplayNotFound, errs.ReplayDisabled:
		status = http.StatusNotFound
	case errs.ServiceNA, errs.NotImplemented, errs.InvalidConfig:
		status = http.StatusServiceUnavailable
	}

	// сбой — уровнем error, отказ по делу — info (как интерцептор gRPC)
	level := lo.Ternary(errs.IsFailure(string(code)), slog.LevelError, slog.LevelInfo)
	slog.Log(r.Context(), level, "replay segment error", slog.String("error", err.Error()), slog.String("path", r.URL.Path))

	writeError(w, status, code, err.Error())
}

func writeError(w http.ResponseWriter, status int, code errs.Err, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code.Error(), "message": message})
}

func bearerToken(v string) string {
	parts := strings.Fields(v)
	switch {
	case len(parts) == 1:
		return parts[0]
	case len(parts) == 2 && strings.EqualFold(parts[0], "bearer"):
		return parts[1]
	}
	return ""
}
