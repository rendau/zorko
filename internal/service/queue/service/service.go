package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	queueModel "github.com/rendau/zorko/internal/service/queue/model"
	localModel "github.com/rendau/zorko/internal/service/queue/service/model"
)

type Config struct {
	Brokers         []string
	Topic           string
	ConsumerGroup   string
	TopicPartitions int32
	TopicRetention  time.Duration
	// предел объёма темы на раздел (retention.bytes): тема не займёт больше разделов × предел,
	// даже если за TopicRetention пришло больше; 0 — без предела
	TopicRetentionBytes int64

	WithProducer bool
	WithConsumer bool
	// FetchMaxWait — сколько брокер копит данные для пачки, пока их меньше FetchMinBytes
	FetchMaxWait time.Duration
	// LagInterval — как часто спрашивать у брокера отставание группы (метрика); 0 — 15s
	LagInterval time.Duration
}

type Service struct {
	conf     Config
	producer *kgo.Client
	consumer *kgo.Client

	// записи последнего PollEvents: подтверждаются CommitPolled (один цикл обработки на копию)
	polledMu sync.Mutex
	polled   []*kgo.Record

	closeOnce sync.Once

	wg sync.WaitGroup
}

func New(conf Config) (*Service, error) {
	s := &Service{conf: conf}

	common := []kgo.Opt{
		kgo.SeedBrokers(conf.Brokers...),
		kgo.DialTimeout(2 * time.Second),
	}

	var err error

	if conf.WithProducer {
		s.producer, err = kgo.NewClient(append(common,
			kgo.DefaultProduceTopic(conf.Topic),
			kgo.RequiredAcks(kgo.AllISRAcks()),
			kgo.ProducerBatchCompression(kgo.ZstdCompression(), kgo.Lz4Compression(), kgo.NoCompression()),
			kgo.ProducerLinger(5*time.Millisecond),
			// приём не должен висеть, если брокер недоступен: лучше 503 и SDK повторит позже
			kgo.RecordDeliveryTimeout(5*time.Second),
			kgo.ProduceRequestTimeout(5*time.Second),
		)...)
		if err != nil {
			return nil, fmt.Errorf("kgo.NewClient(producer): %w", err)
		}
	}

	if conf.WithConsumer {
		s.consumer, err = kgo.NewClient(append(common,
			kgo.ConsumerGroup(conf.ConsumerGroup),
			kgo.ConsumeTopics(conf.Topic),
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
			kgo.DisableAutoCommit(),
			// ребаланс не забирает разделы посреди пачки: до CommitPolled
			kgo.BlockRebalanceOnPoll(),
			kgo.FetchMinBytes(1<<20),
			kgo.FetchMaxWait(lo.Ternary(conf.FetchMaxWait > 0, conf.FetchMaxWait, time.Second)),
		)...)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("kgo.NewClient(consumer): %w", err)
		}
	}

	return s, nil
}

// EnsureTopic создаёт тему событий, если её нет, а у существующей выставляет срок и предел
// объёма хранения из конфига (они могли поменяться с прошлого запуска)
func (s *Service) EnsureTopic(ctx context.Context) error {
	client := lo.Ternary(s.producer != nil, s.producer, s.consumer)
	if client == nil {
		return nil
	}

	admin := kadm.NewClient(client)

	configs := map[string]*string{
		"retention.ms":    new(strconv.FormatInt(s.conf.TopicRetention.Milliseconds(), 10)),
		"retention.bytes": new(strconv.FormatInt(lo.Ternary(s.conf.TopicRetentionBytes > 0, s.conf.TopicRetentionBytes, -1), 10)),
	}

	_, err := admin.CreateTopic(ctx, s.conf.TopicPartitions, -1, configs, s.conf.Topic)
	if err == nil {
		return nil
	}
	if !errors.Is(err, kerr.TopicAlreadyExists) {
		return fmt.Errorf("kadm.CreateTopic: %w", err)
	}

	alter := lo.MapToSlice(configs, func(name string, value *string) kadm.AlterConfig {
		return kadm.AlterConfig{Op: kadm.SetConfig, Name: name, Value: value}
	})
	resps, err := admin.AlterTopicConfigs(ctx, alter, s.conf.Topic)
	if err != nil {
		return fmt.Errorf("kadm.AlterTopicConfigs: %w", err)
	}
	for _, r := range resps {
		if r.Err != nil {
			return fmt.Errorf("kadm.AlterTopicConfigs(%s): %w %s", r.Name, r.Err, r.ErrMessage)
		}
	}

	return nil
}

// PublishEvents пишет события и ждёт подтверждения брокером всех записей
func (s *Service) PublishEvents(ctx context.Context, events []*queueModel.Event) error {
	if s.producer == nil {
		return errors.New("producer is disabled")
	}

	records := lo.Map(events, func(v *queueModel.Event, _ int) *kgo.Record {
		return localModel.EncodeEvent(v, s.conf.Topic)
	})

	if err := s.producer.ProduceSync(ctx, records...).FirstErr(); err != nil {
		return fmt.Errorf("producer.ProduceSync: %w", err)
	}

	return nil
}

// PollEvents ждёт следующую пачку (до max записей). Перед новым вызовом — CommitPolled.
func (s *Service) PollEvents(ctx context.Context, max int) ([]*queueModel.Event, error) {
	if s.consumer == nil {
		return nil, errors.New("consumer is disabled")
	}

	fetches := s.consumer.PollRecords(ctx, max)
	if fetches.IsClientClosed() {
		return nil, kgo.ErrClientClosed
	}
	if err := ctx.Err(); err != nil {
		// пачки не будет: без AllowRebalance клиент не сможет выйти из группы при Close
		s.consumer.AllowRebalance()
		return nil, err
	}

	var fetchErr error
	fetches.EachError(func(topic string, partition int32, err error) {
		fetchErr = errors.Join(fetchErr, fmt.Errorf("fetch %s/%d: %w", topic, partition, err))
	})

	records := fetches.Records()

	s.polledMu.Lock()
	s.polled = records
	s.polledMu.Unlock()

	if len(records) == 0 && fetchErr != nil {
		s.consumer.AllowRebalance()
		return nil, fetchErr
	}

	return lo.Map(records, localModel.DecodeEvent), nil
}

// CommitPolled подтверждает записи последнего PollEvents и разрешает ребаланс
func (s *Service) CommitPolled(ctx context.Context) error {
	if s.consumer == nil {
		return errors.New("consumer is disabled")
	}
	defer s.consumer.AllowRebalance()

	s.polledMu.Lock()
	records := s.polled
	s.polled = nil
	s.polledMu.Unlock()

	if len(records) == 0 {
		return nil
	}

	if err := s.consumer.CommitRecords(ctx, records...); err != nil {
		return fmt.Errorf("consumer.CommitRecords: %w", err)
	}

	return nil
}

// Start — замер отставания группы обработки до отмены ctx (только при включённом consumer)
func (s *Service) Start(ctx context.Context) {
	if s.consumer == nil {
		return
	}

	admin := kadm.NewClient(s.consumer)
	interval := lo.Ternary(s.conf.LagInterval > 0, s.conf.LagInterval, 15*time.Second)

	s.wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			s.measureLag(ctx, admin, interval)

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

func (s *Service) measureLag(ctx context.Context, admin *kadm.Client, timeout time.Duration) {
	lagCtx, lagCtxCancel := context.WithTimeout(ctx, timeout)
	defer lagCtxCancel()

	lags, err := admin.Lag(lagCtx, s.conf.ConsumerGroup)
	group := lags[s.conf.ConsumerGroup]
	if err == nil {
		err = group.Error()
	}
	if err != nil {
		if ctx.Err() == nil {
			metricConsumerLagErrorTotal.WithLabelValues(s.conf.Topic).Inc()
			slog.Warn("queue: consumer lag", "error", err)
		}
		return
	}

	metricConsumerLag.WithLabelValues(s.conf.Topic).Set(float64(group.Lag.Total()))
}

func (s *Service) Wait() {
	s.wg.Wait()
}

// Close дожидается отправки буфера продюсера и выходит из группы; повторный вызов ничего не делает
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		if s.producer != nil {
			ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = s.producer.Flush(ctx)
			ctxCancel()
			s.producer.Close()
		}
		if s.consumer != nil {
			s.consumer.AllowRebalance()
			s.consumer.Close()
		}
	})
}
