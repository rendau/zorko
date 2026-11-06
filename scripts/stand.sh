#!/usr/bin/env bash
# Тестовый стенд zorko в docker: Postgres, ClickHouse, Redpanda, MinIO (S3).
# Использование: scripts/stand.sh up | down | status
# Параметры стенда описаны в CLAUDE.md, раздел «Тестовый стенд».
set -euo pipefail

PG_IMAGE=postgres:17
CH_IMAGE=clickhouse/clickhouse-server:25.8
RP_IMAGE=redpandadata/redpanda:v25.2.1
# minio/minio больше не публикуется в Docker Hub — сборка Chainguard
MINIO_IMAGE=cgr.dev/chainguard/minio:latest

up() {
	docker run --rm -d --name zorko-pg \
		-e TZ=Asia/Almaty -e POSTGRES_USER=zorko -e POSTGRES_PASSWORD=zorko -e POSTGRES_DB=zorko \
		-p 54330:5432 "$PG_IMAGE" >/dev/null

	docker run --rm -d --name zorko-ch \
		-e TZ=Asia/Almaty -e CLICKHOUSE_USER=zorko -e CLICKHOUSE_PASSWORD=zorko -e CLICKHOUSE_DB=zorko \
		-e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 \
		--ulimit nofile=262144:262144 \
		-v "$(cd "$(dirname "$0")" && pwd)/clickhouse.xml:/etc/clickhouse-server/config.d/zorko.xml:ro" \
		-p 19000:9000 -p 18123:8123 "$CH_IMAGE" >/dev/null

	docker run --rm -d --name zorko-redpanda \
		-e TZ=Asia/Almaty \
		-p 19092:19092 "$RP_IMAGE" \
		redpanda start --mode dev-container --smp 1 --memory 1G --overprovisioned \
		--kafka-addr internal://0.0.0.0:9092,external://0.0.0.0:19092 \
		--advertise-kafka-addr internal://zorko-redpanda:9092,external://localhost:19092 >/dev/null

	# бакет zorko создаёт сам сервис при старте (EnsureBucket)
	docker run --rm -d --name zorko-minio -v /data \
		-e TZ=Asia/Almaty -e MINIO_ROOT_USER=zorko -e MINIO_ROOT_PASSWORD=zorko-secret \
		-p 19100:9000 -p 19101:9001 "$MINIO_IMAGE" server /data --console-address :9001 >/dev/null

	# ожидание готовности
	for _ in $(seq 1 60); do
		if docker exec zorko-pg pg_isready -U zorko -q 2>/dev/null &&
			docker exec zorko-ch clickhouse-client -u zorko --password zorko -q 'select 1' >/dev/null 2>&1 &&
			docker exec zorko-redpanda rpk cluster health 2>/dev/null | grep -q 'Healthy:.*true' &&
			curl -sf -o /dev/null http://localhost:19100/minio/health/ready; then
			echo "stand is up"
			return 0
		fi
		sleep 1
	done
	echo "stand did not become ready" >&2
	return 1
}

down() {
	docker stop zorko-pg zorko-ch zorko-redpanda zorko-minio >/dev/null 2>&1 || true
	echo "stand is down"
}

status() {
	docker ps --filter name=zorko- --format '{{.Names}}\t{{.Status}}\t{{.Ports}}'
}

case "${1:-}" in
up) up ;;
down) down ;;
status) status ;;
*)
	echo "usage: $0 up|down|status" >&2
	exit 1
	;;
esac
