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

	KafkaBrokers       []string `env:"KAFKA_BROKERS" envSeparator:","`
	KafkaTopicEvents   string   `env:"KAFKA_TOPIC_EVENTS" envDefault:"zorko.events"`
	KafkaConsumerGroup string   `env:"KAFKA_CONSUMER_GROUP" envDefault:"zorko-processor"`
	// тема создаётся при старте, если её нет: разделов — потолок числа копий обработки
	KafkaTopicPartitions int32         `env:"KAFKA_TOPIC_PARTITIONS" envDefault:"12"`
	KafkaTopicRetention  time.Duration `env:"KAFKA_TOPIC_RETENTION" envDefault:"24h"`

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
}{}

func init() {
	if err := env.Parse(&Conf); err != nil {
		panic(err)
	}
}
