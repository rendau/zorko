// Package oauthcleanup — фоновая чистка OAuth-входа MCP (ADR-020): истёкшие токены и подключения,
// оставшиеся без токенов (брошенное согласие, истёкший refresh-токен).
package oauthcleanup

import "context"

type Cleaner interface {
	Cleanup(ctx context.Context)
	Start(ctx context.Context)
	Wait()
}
