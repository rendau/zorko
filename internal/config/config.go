package config

import (
	"time"

	"github.com/caarlos0/env/v9"
	_ "github.com/joho/godotenv/autoload"
)

var Conf = struct {
	Debug    bool   `env:"DEBUG" envDefault:"false"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	WithTracing   bool   `env:"WITH_TRACING" envDefault:"false"`
	JaegerAddress string `env:"JAEGER_ADDRESS"`

	GrpcPort string `env:"GRPC_PORT" envDefault:"5050"`
	HttpPort string `env:"HTTP_PORT" envDefault:"80"`
	HttpCors bool   `env:"HTTP_CORS" envDefault:"false"`

	// подпись токенов входа (HS256, ADR-005)
	JwtSecret string `env:"JWT_SECRET"`

	PgDsn string `env:"PG_DSN"`
	ChDsn string `env:"CH_DSN"`

	KafkaBrokers     []string `env:"KAFKA_BROKERS" envSeparator:","`
	KafkaTopicEvents string   `env:"KAFKA_TOPIC_EVENTS" envDefault:"zorko.events"`
	// описания сегментов записей сессий (ADR-012); группа обработки — KafkaConsumerGroup + "-replay"
	KafkaTopicReplays  string `env:"KAFKA_TOPIC_REPLAYS" envDefault:"zorko.replays"`
	KafkaConsumerGroup string `env:"KAFKA_CONSUMER_GROUP" envDefault:"zorko-processor"`
	// тема создаётся при старте, если её нет: разделов — потолок числа копий обработки
	KafkaTopicPartitions int32         `env:"KAFKA_TOPIC_PARTITIONS" envDefault:"12"`
	KafkaTopicRetention  time.Duration `env:"KAFKA_TOPIC_RETENTION" envDefault:"24h"`
	// предел объёма каждой темы на раздел, байт (retention.bytes; 0 — без предела): при наплыве
	// старые сообщения удаляются раньше срока, но диск брокера не переполняется.
	// 512 МБ × 12 разделов × 2 темы = 12 ГБ (+ по сегменту на раздел) — под диск 20 ГБ
	KafkaTopicRetentionBytes int64 `env:"KAFKA_TOPIC_RETENTION_BYTES" envDefault:"536870912"`

	IngestEnabled       bool   `env:"INGEST_ENABLED" envDefault:"true"`
	IngestPort          string `env:"INGEST_PORT" envDefault:"8000"`
	IngestPublicUrl     string `env:"INGEST_PUBLIC_URL"`
	IngestMaxBodyBytes  int64  `env:"INGEST_MAX_BODY_BYTES" envDefault:"20971520"`
	IngestMaxEventBytes int64  `env:"INGEST_MAX_EVENT_BYTES" envDefault:"1048576"`

	// интерфейс (SPA): статика из AdminDir на своём порту; адрес API — в /config.js
	AdminPort       string `env:"ADMIN_PORT" envDefault:"8001"`
	AdminDir        string `env:"ADMIN_DIR" envDefault:"./admin-ui"`
	AdminApiBaseUrl string `env:"ADMIN_API_BASE_URL"`

	ProcessorEnabled   bool          `env:"PROCESSOR_ENABLED" envDefault:"true"`
	ProcessorBatchSize int           `env:"PROCESSOR_BATCH_SIZE" envDefault:"500"`
	ProcessorBatchWait time.Duration `env:"PROCESSOR_BATCH_WAIT" envDefault:"1s"`

	IssueEventStorePerMinute int `env:"ISSUE_EVENT_STORE_PER_MINUTE" envDefault:"100"`

	// карты исходников (шаг 3, ADR-008..011); без S3_ENDPOINT — выключены
	S3Endpoint  string `env:"S3_ENDPOINT"`
	S3Region    string `env:"S3_REGION" envDefault:"us-east-1"`
	S3Bucket    string `env:"S3_BUCKET" envDefault:"zorko"`
	S3AccessKey string `env:"S3_ACCESS_KEY"`
	S3SecretKey string `env:"S3_SECRET_KEY"`
	S3Secure    bool   `env:"S3_SECURE" envDefault:"true"`

	// разобранные карты в памяти обработчика, МБ
	SourcemapCacheMb int `env:"SOURCEMAP_CACHE_MB" envDefault:"256"`
	// архив удаляется, если столько времени не пригодился ни одной ошибке
	SourcemapRetention time.Duration `env:"SOURCEMAP_RETENTION" envDefault:"720h"`
	// временные файлы сборки архивов; пусто — системный tmp
	AssembleTmpDir string `env:"ASSEMBLE_TMP_DIR"`
	// одновременных запросов к сайтам за картами на копию обработчика
	SiteFetchConcurrency int `env:"SITE_FETCH_CONCURRENCY" envDefault:"4"`

	// запись сессий (ADR-012..015); без S3_ENDPOINT — выключена
	// тело одного сегмента (сжатое), байт
	ReplayMaxSegmentBytes int64 `env:"REPLAY_MAX_SEGMENT_BYTES" envDefault:"10485760"`
	// сколько хранить записи в S3; TTL таблиц ClickHouse — 30 дней в миграции, менять вместе
	ReplayRetention time.Duration `env:"REPLAY_RETENTION" envDefault:"720h"`

	// предохранитель диска ClickHouse (ADR-017): занято больше процента — удаляются самые
	// старые дни данных zorko, сегодняшний не трогается; 0 — выключено
	ChDiskMaxUsedPercent int           `env:"CH_DISK_MAX_USED_PERCENT" envDefault:"80"`
	ChDiskCheckInterval  time.Duration `env:"CH_DISK_CHECK_INTERVAL" envDefault:"10m"`
}{}

func init() {
	if err := env.Parse(&Conf); err != nil {
		panic(err)
	}
}
