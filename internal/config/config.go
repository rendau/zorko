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

	PgDsn string `env:"PG_DSN"`
	ChDsn string `env:"CH_DSN"`

	KafkaBrokers       []string `env:"KAFKA_BROKERS" envSeparator:","`
	KafkaTopicEvents   string   `env:"KAFKA_TOPIC_EVENTS" envDefault:"zorko.events"`
	KafkaConsumerGroup string   `env:"KAFKA_CONSUMER_GROUP" envDefault:"zorko-processor"`

	IngestEnabled       bool   `env:"INGEST_ENABLED" envDefault:"true"`
	IngestPort          string `env:"INGEST_PORT" envDefault:"8000"`
	IngestPublicUrl     string `env:"INGEST_PUBLIC_URL"`
	IngestMaxBodyBytes  int64  `env:"INGEST_MAX_BODY_BYTES" envDefault:"20971520"`
	IngestMaxEventBytes int64  `env:"INGEST_MAX_EVENT_BYTES" envDefault:"1048576"`

	ProcessorEnabled   bool          `env:"PROCESSOR_ENABLED" envDefault:"true"`
	ProcessorBatchSize int           `env:"PROCESSOR_BATCH_SIZE" envDefault:"500"`
	ProcessorBatchWait time.Duration `env:"PROCESSOR_BATCH_WAIT" envDefault:"1s"`

	IssueEventStorePerMinute int `env:"ISSUE_EVENT_STORE_PER_MINUTE" envDefault:"100"`
}{}

func init() {
	if err := env.Parse(&Conf); err != nil {
		panic(err)
	}
}
