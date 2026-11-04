package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func initChConn(dsn string) (driver.Conn, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("clickhouse.ParseDSN: %w", err)
	}

	opts.DialTimeout = 2 * time.Second
	opts.MaxOpenConns = 10
	opts.MaxIdleConns = 5
	opts.ConnMaxLifetime = time.Hour
	opts.Compression = &clickhouse.Compression{Method: clickhouse.CompressionLZ4}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("clickhouse.Open: %w", err)
	}

	ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ctxCancel()

	if err = conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("clickhouse.Ping: %w", err)
	}

	return conn, nil
}
