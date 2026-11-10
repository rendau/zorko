package app

import (
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Раздача интерфейса (ADR-007): предсжатые .br/.gz как в ruto/kusec, на лету ничего не сжимается.
const (
	// файлы сборки (бандл с хешем в имени, иконки, шрифты) — сутки в кэше браузера
	adminAssetCacheControl = "public, max-age=86400"
	// index.html и адреса SPA — проверка при каждом открытии (Last-Modified → 304): новый релиз виден сразу
	adminIndexCacheControl = "no-cache"
)

type precompressedEncoding struct {
	name string
	ext  string
}

// precompressedEncodings — копии, которые admin/scripts/compress.mjs пишет рядом с файлами сборки;
// предпочтительная первой
var precompressedEncodings = []precompressedEncoding{
	{name: "br", ext: ".br"},
	{name: "gzip", ext: ".gz"},
}

// AdminConfig — рантайм-конфиг SPA (window.__APP_CONFIG__ в /config.js): один билд на все окружения
type AdminConfig struct {
	ApiBaseUrl string          `json:"apiBaseUrl"`
	Auth       AdminAuthConfig `json:"auth"`
}

// AdminAuthConfig — способ входа (AUTH_MODE, ADR-019) и что интерфейсу нужно для него знать
type AdminAuthConfig struct {
	Mode            string `json:"mode"`
	OidcIssuer      string `json:"oidcIssuer,omitempty"`
	OidcClientId    string `json:"oidcClientId,omitempty"`
	OidcScopes      string `json:"oidcScopes,omitempty"`
	ProxyLoginUrl   string `json:"proxyLoginUrl,omitempty"`
	ProxyRefreshUrl string `json:"proxyRefreshUrl,omitempty"`
	ProxyLogoutUrl  string `json:"proxyLogoutUrl,omitempty"`
}

// AdminHttpServerCreate — сервер интерфейса; nil, если каталога сборки нет (интерфейс выключен)
func AdminHttpServerCreate(port, dir string, cfg AdminConfig) *http.Server {
	if stat, err := os.Stat(filepath.Join(dir, "index.html")); err != nil || stat.IsDir() {
		slog.Warn("admin: index.html not found, admin-http-server disabled", "dir", dir)
		return nil
	}

	return &http.Server{
		Addr:              ":" + port,
		Handler:           recoverMiddleware(newAdminHandler(dir, cfg)),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 * 1024,
	}
}

func newAdminHandler(dir string, cfg AdminConfig) http.Handler {
	indexPath := filepath.Join(dir, "index.html")
	configJs := adminConfigJs(cfg)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		requestPath := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), "/")

		if requestPath == "config.js" {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(configJs)
			return
		}

		filePath, cacheControl := indexPath, adminIndexCacheControl
		if requestPath != "." && requestPath != "index.html" {
			fullPath := filepath.Join(dir, requestPath)
			if stat, err := os.Stat(fullPath); err == nil && !stat.IsDir() {
				filePath, cacheControl = fullPath, adminAssetCacheControl
			}
		}

		w.Header().Set("Cache-Control", cacheControl)
		serveAdminFile(w, r, filePath)
	})
}

// adminConfigJs — рантайм-конфиг SPA: один билд на все окружения
func adminConfigJs(cfg AdminConfig) []byte {
	value, _ := json.Marshal(cfg)
	return []byte("window.__APP_CONFIG__ = " + string(value) + ";\n")
}

func serveAdminFile(w http.ResponseWriter, r *http.Request, path string) {
	w.Header().Add("Vary", "Accept-Encoding")

	acceptEncoding := r.Header.Get("Accept-Encoding")
	for _, enc := range precompressedEncodings {
		if acceptsEncoding(acceptEncoding, enc.name) && serveEncodedFile(w, r, path, enc) {
			return
		}
	}

	// не http.ServeFile: тот перенаправляет /index.html на /
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, path, stat.ModTime(), f)
}

// serveEncodedFile отдаёт enc-копию файла, если она есть. Content-Type и Last-Modified — от
// оригинала, чтобы закэшированная копия ревалидировалась одинаково при любой кодировке
func serveEncodedFile(w http.ResponseWriter, r *http.Request, path string, enc precompressedEncoding) bool {
	f, err := os.Open(path + enc.ext)
	if err != nil {
		return false
	}
	defer f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		return false
	}

	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Encoding", enc.name)
	http.ServeContent(w, r, path, stat.ModTime(), f)

	return true
}

func acceptsEncoding(header, name string) bool {
	for part := range strings.SplitSeq(header, ",") {
		coding, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(coding), name) {
			continue
		}

		q, found := strings.CutPrefix(strings.TrimSpace(params), "q=")
		if !found {
			return true
		}
		weight, err := strconv.ParseFloat(q, 64)

		return err == nil && weight > 0
	}

	return false
}
