package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	"github.com/rendau/zorko/internal/errs"
	usecase "github.com/rendau/zorko/internal/usecase/ingest"
)

// Handler — приём событий по протоколу Sentry (ADR-001): /api/{project_id}/envelope/ и /store/
type Handler struct {
	usecase       *usecase.Usecase
	maxBodyBytes  int64
	maxEventBytes int64
}

func New(uc *usecase.Usecase, maxBodyBytes, maxEventBytes int64) *Handler {
	return &Handler{
		usecase:       uc,
		maxBodyBytes:  maxBodyBytes,
		maxEventBytes: maxEventBytes,
	}
}

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/{project_id}/envelope", h.envelope)
	mux.HandleFunc("POST /api/{project_id}/envelope/{$}", h.envelope)
	mux.HandleFunc("POST /api/{project_id}/store", h.store)
	mux.HandleFunc("POST /api/{project_id}/store/{$}", h.store)

	return corsMiddleware(mux)
}

func (h *Handler) envelope(w http.ResponseWriter, r *http.Request) {
	projectId, body, ok := h.readRequest(w, r)
	if !ok {
		return
	}

	env, err := parseEnvelope(body)
	if err != nil {
		h.reject(w, http.StatusBadRequest, resultBadRequest, err.Error())
		return
	}

	publicKey := publicKeyFromRequest(r)
	if publicKey == "" {
		publicKey = publicKeyFromDsn(env.Header.Dsn)
	}

	events := make([]*eventModel.RawEvent, 0, 1)
	for _, item := range env.Items {
		if item.Type != "event" {
			metricItemDropped.WithLabelValues(itemTypeLabel(item.Type)).Inc()
			continue
		}
		if int64(len(item.Payload)) > h.maxEventBytes {
			metricItemDropped.WithLabelValues("event_too_large").Inc()
			continue
		}
		events = append(events, &eventModel.RawEvent{
			EventId: eventId(env.Header.EventId, eventIdFromPayload(item.Payload)),
			Payload: item.Payload,
		})
	}

	responseId := env.Header.EventId
	if len(events) > 0 {
		responseId = events[0].EventId
	}

	h.ingest(w, r, projectId, publicKey, events, responseId)
}

func (h *Handler) store(w http.ResponseWriter, r *http.Request) {
	projectId, body, ok := h.readRequest(w, r)
	if !ok {
		return
	}

	if !json.Valid(body) {
		h.reject(w, http.StatusBadRequest, resultBadRequest, "event is not valid json")
		return
	}

	events := make([]*eventModel.RawEvent, 0, 1)
	if int64(len(body)) > h.maxEventBytes {
		metricItemDropped.WithLabelValues("event_too_large").Inc()
	} else {
		events = append(events, &eventModel.RawEvent{
			EventId: eventId(eventIdFromPayload(body)),
			Payload: body,
		})
	}

	responseId := ""
	if len(events) > 0 {
		responseId = events[0].EventId
	}

	h.ingest(w, r, projectId, publicKeyFromRequest(r), events, responseId)
}

func (h *Handler) readRequest(w http.ResponseWriter, r *http.Request) (int64, []byte, bool) {
	projectId, err := strconv.ParseInt(r.PathValue("project_id"), 10, 64)
	if err != nil || projectId <= 0 {
		h.reject(w, http.StatusBadRequest, resultBadRequest, "bad project id")
		return 0, nil, false
	}

	body, err := readBody(r, h.maxBodyBytes)
	switch {
	case errors.Is(err, errBodyTooLarge):
		h.reject(w, http.StatusRequestEntityTooLarge, resultTooLarge, "body too large")
		return 0, nil, false
	case err != nil:
		h.reject(w, http.StatusBadRequest, resultBadRequest, err.Error())
		return 0, nil, false
	}

	return projectId, body, true
}

func (h *Handler) ingest(w http.ResponseWriter, r *http.Request, projectId int64, publicKey string, events []*eventModel.RawEvent, responseId string) {
	// запрос SDK не должен обрываться вместе с соединением клиента посреди записи в очередь
	ctx := context.WithoutCancel(r.Context())

	err := h.usecase.Ingest(ctx, &usecase.Req{
		ProjectId:  projectId,
		PublicKey:  publicKey,
		OriginHost: originHost(r),
		ClientIp:   clientIp(r),
		Events:     events,
	})

	if rateErr, ok := errors.AsType[usecase.RateLimitedError](err); ok {
		seconds := strconv.Itoa(max(int(rateErr.RetryAfter.Seconds()), 1))
		w.Header().Set("Retry-After", seconds)
		w.Header().Set("X-Sentry-Rate-Limits", seconds+"::project")
		h.reject(w, http.StatusTooManyRequests, resultRateLimited, "rate limited")
		return
	}

	switch {
	case errors.Is(err, errs.IngestKeyInvalid):
		h.reject(w, http.StatusForbidden, resultForbidden, "invalid project key")
		return
	case errors.Is(err, errs.IngestOriginDenied):
		h.reject(w, http.StatusForbidden, resultForbidden, "origin is not allowed")
		return
	case err != nil:
		slog.Error("ingest: usecase.Ingest", "error", err, "project_id", projectId)
		h.reject(w, http.StatusServiceUnavailable, resultUnavailable, "temporarily unavailable")
		return
	}

	metricRequestTotal.WithLabelValues(resultAccepted).Inc()
	metricEventAccepted.Add(float64(len(events)))

	writeJson(w, http.StatusOK, map[string]string{"id": responseId})
}

func (h *Handler) reject(w http.ResponseWriter, status int, result, detail string) {
	metricRequestTotal.WithLabelValues(result).Inc()
	writeJson(w, status, map[string]string{"detail": detail})
}

func writeJson(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// corsMiddleware — браузеры шлют события с любых сайтов; какие сайты разрешены, решает ключ
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()

		origin := r.Header.Get("Origin")
		if origin != "" {
			header.Set("Access-Control-Allow-Origin", origin)
			header.Add("Vary", "Origin")
		} else {
			header.Set("Access-Control-Allow-Origin", "*")
		}
		header.Set("Access-Control-Expose-Headers", "X-Sentry-Rate-Limits, Retry-After")

		if r.Method == http.MethodOptions {
			header.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			if requested := r.Header.Get("Access-Control-Request-Headers"); requested != "" {
				header.Set("Access-Control-Allow-Headers", requested)
			} else {
				header.Set("Access-Control-Allow-Headers", "Content-Type, Content-Encoding, X-Sentry-Auth")
			}
			header.Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
