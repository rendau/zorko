// Package mcp — встроенный MCP-сервер (Model Context Protocol) для AI-агентов поверх usecase-слоя
// (ADR-020): агент читает то же, что человек видит в интерфейсе, — проекты, проблемы, события со
// стеком, теги, записи сессий. Инструментов записи нет.
//
// Streamable HTTP на своём порту (MCP_ENABLED / MCP_PORT), без сессий на сервере: каждый запрос
// самодостаточен и проверяется по своему токену, поэтому копий сервиса может быть сколько угодно, а
// перезапуск не рвёт подключения. Вход — только OAuth (internal/usecase/oauth): человек разрешает
// клиента на странице согласия интерфейса, клиент получает права этого человека.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	oauthService "github.com/rendau/zorko/internal/domain/oauth/service"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	"github.com/rendau/zorko/internal/errs"
	issueUsc "github.com/rendau/zorko/internal/usecase/issue"
	oauthUsc "github.com/rendau/zorko/internal/usecase/oauth"
	orgUsc "github.com/rendau/zorko/internal/usecase/org"
	projectUsc "github.com/rendau/zorko/internal/usecase/project"
	replayUsc "github.com/rendau/zorko/internal/usecase/replay"
)

// serverInstructions отдаётся клиенту при initialize — как агенту работать с сервером
const serverInstructions = `zorko — трекер ошибок фронтенда и мобильных приложений (SDK Sentry). Здесь только чтение: что и где падает, с каким стеком, у кого и после каких действий.

Как устроено: организация → проекты (одно приложение) → проблемы (одинаковые ошибки, склеенные по отпечатку) → события (отдельные случаи ошибки). К событию может быть запись сессии — что человек делал на сайте до ошибки.

Правила работы:
1. Видны только проекты, доступные человеку, который подключил тебя. Начни с list_project: там организации и проекты с их id и slug.
2. org и projects принимают id или slug. org можно не указывать, если организация одна.
3. Период — period: 1h, 24h, 7d, 14d (по умолчанию), 30d. События хранятся 30 дней.
4. Строка поиска query — как в Sentry: слова ищутся в заголовке проблемы, условия на теги — ключ:значение (level:error, browser.name:Chrome, release:shop@1.4.2, url:*cart*), отрицание — !ключ:значение, * — любая часть. Какие бывают ключи и значения — list_tag_key и list_tag_value.
5. Чтобы разобрать ошибку: list_issue → get_issue (сколько, у кого, с какой версии) → get_event с event_id=latest (стек со строками кода, действия перед ошибкой, окружение). Стек уже переведён по картам исходников, если они залиты; sourcemap_error у кадра — причина, почему не переведён.
6. Пагинация с нуля: page начинается с 0, page_size по умолчанию 25, не больше 100.
7. В ответах есть url — ссылка на ту же страницу в интерфейсе zorko; давай её человеку.
8. В событиях бывают личные данные людей (почта, IP, адреса страниц): не выноси их дальше, чем нужно для разбора ошибки.`

// Config — что обработчику нужно знать об установке
type Config struct {
	// AdminURL — адрес интерфейса: для ссылок в ответах инструментов
	AdminURL string
	Version  string
}

type sessionServiceI interface {
	WithContext(ctx context.Context, session *sessionModel.Session) context.Context
}

// Observer — вызов инструмента для метрик: имя, ошибка (nil — успех), длительность
type Observer func(tool string, err error, duration time.Duration)

type Handler struct {
	conf       Config
	sessionSvc sessionServiceI

	oauthUsecase   *oauthUsc.Usecase
	orgUsecase     *orgUsc.Usecase
	projectUsecase *projectUsc.Usecase
	issueUsecase   *issueUsc.Usecase
	replayUsecase  *replayUsc.Usecase

	observe Observer
	// схемы инструментов считаются один раз: сервер собирается заново на каждый запрос
	schemaCache *mcpsdk.SchemaCache
}

func New(
	conf Config,
	sessionSvc sessionServiceI,
	oauthUsecase *oauthUsc.Usecase,
	orgUsecase *orgUsc.Usecase,
	projectUsecase *projectUsc.Usecase,
	issueUsecase *issueUsc.Usecase,
	replayUsecase *replayUsc.Usecase,
	observe Observer,
) *Handler {
	if observe == nil {
		observe = func(string, error, time.Duration) {}
	}
	conf.AdminURL = strings.TrimSuffix(conf.AdminURL, "/")

	return &Handler{
		conf:           conf,
		sessionSvc:     sessionSvc,
		oauthUsecase:   oauthUsecase,
		orgUsecase:     orgUsecase,
		projectUsecase: projectUsecase,
		issueUsecase:   issueUsecase,
		replayUsecase:  replayUsecase,
		observe:        observe,
		schemaCache:    mcpsdk.NewSchemaCache(),
	}
}

type ctxKeyT int

// ctxKeySession — кто пришёл (*oauthUsc.McpSession), кладёт authMiddleware
const ctxKeySession ctxKeyT = iota

// HTTPHandler — адрес MCP с проверкой токена на каждом запросе и OAuth-адреса рядом с ним
func (h *Handler) HTTPHandler() http.Handler {
	streamable := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		session, _ := r.Context().Value(ctxKeySession).(*oauthUsc.McpSession)
		if session == nil {
			return nil
		}
		return h.newServer(session)
	}, &mcpsdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})

	oauth := newOAuthEndpoints(h.oauthUsecase)

	mux := http.NewServeMux()
	oauth.register(mux)
	mux.Handle("/", h.authMiddleware(streamable, oauth))

	return withAccessLog(mux)
}

// authMiddleware проверяет OAuth-токен на каждом HTTP-запросе
func (h *Handler) authMiddleware(next http.Handler, oauth *oauthEndpoints) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential := bearerToken(r.Header.Get("Authorization"))
		if credential == "" {
			unauthorized(w, oauth, "", "zorko mcp: an OAuth token is required (Authorization: Bearer ...)")
			return
		}
		// строка не нашего вида — фон сканеров: отсекается до базы
		if !strings.HasPrefix(credential, oauthService.AccessTokenPrefix) {
			slog.Info("mcp: credential rejected", "credential", "unknown", "client_ip", clientIP(r))
			unauthorized(w, oauth, "invalid_token", "zorko mcp: invalid token")
			return
		}

		session, err := h.oauthUsecase.McpSessionFromToken(r.Context(), credential)
		if err != nil {
			if !errors.Is(err, errs.NotAuthorized) {
				// сбой (БД), а не отказ: клиент не должен из-за него заново проходить вход
				slog.Error("mcp: authenticate", "client_ip", clientIP(r), "error", err)
				http.Error(w, "zorko mcp: temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			unauthorized(w, oauth, "invalid_token", "zorko mcp: invalid or expired token")
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeySession, session)))
	})
}

// unauthorized отвечает 401 и указывает клиенту на описание защищаемого ресурса (RFC 9728) — с
// него клиент начинает вход
func unauthorized(w http.ResponseWriter, oauth *oauthEndpoints, oauthErr, msg string) {
	value := fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q`, oauth.resourceMetadataURL(), oauthUsc.Scope)
	if oauthErr != "" {
		value += fmt.Sprintf(`, error=%q`, oauthErr)
	}
	w.Header().Set("WWW-Authenticate", value)
	http.Error(w, msg, http.StatusUnauthorized)
}

func bearerToken(headerValue string) string {
	parts := strings.Fields(headerValue)
	switch {
	case len(parts) == 1:
		return parts[0]
	case len(parts) == 2 && strings.EqualFold(parts[0], "bearer"):
		return parts[1]
	}
	return ""
}

// newServer — MCP-сервер одного запроса: инструменты работают от имени человека подключения
func (h *Handler) newServer(session *oauthUsc.McpSession) *mcpsdk.Server {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "zorko",
		Title:   "zorko — трекер ошибок",
		Version: h.conf.Version,
	}, &mcpsdk.ServerOptions{
		Instructions: serverInstructions,
		SchemaCache:  h.schemaCache,
	})

	(&toolSet{h: h, session: session}).register(srv)

	return srv
}
