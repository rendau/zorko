// Package httpx — единая фабрика HTTP-клиентов: таймауты соединения и TLS, лимит соединений на хост,
// InsecureSkipVerify (внутренние сервисы с самоподписанными сертификатами). Нулевые поля → дефолты.
package httpx

import (
	"crypto/tls"
	"net"
	"net/http"
	"syscall"
	"time"
)

const (
	defaultDialTimeout         = 2 * time.Second
	defaultTLSHandshakeTimeout = 2 * time.Second
	defaultMaxConnsPerHost     = 20
)

type Config struct {
	DialTimeout         time.Duration
	TLSHandshakeTimeout time.Duration
	MaxConnsPerHost     int
	// Timeout — общий таймаут запроса; 0 — без него (время ограничивает контекст)
	Timeout time.Duration
	// DialControl — проверка адреса после DNS, перед соединением (защита от SSRF)
	DialControl func(network, address string, c syscall.RawConn) error
	// CheckRedirect — как в http.Client
	CheckRedirect func(req *http.Request, via []*http.Request) error
	// NoProxy — не брать прокси из окружения
	NoProxy bool
}

func NewTransport(c Config) *http.Transport {
	dialTimeout := c.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = defaultDialTimeout
	}
	tlsTimeout := c.TLSHandshakeTimeout
	if tlsTimeout == 0 {
		tlsTimeout = defaultTLSHandshakeTimeout
	}
	maxConns := c.MaxConnsPerHost
	if maxConns == 0 {
		maxConns = defaultMaxConnsPerHost
	}

	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second, Control: c.DialControl}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   tlsTimeout,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // внутренние сервисы с самоподписанными сертификатами
		MaxConnsPerHost:       maxConns,
		MaxIdleConnsPerHost:   maxConns,
		MaxIdleConns:          maxConns * 4,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	if c.NoProxy {
		tr.Proxy = nil
	}
	return tr
}

func New(c Config) *http.Client {
	return &http.Client{
		Transport:     NewTransport(c),
		Timeout:       c.Timeout,
		CheckRedirect: c.CheckRedirect,
	}
}
