// Package events provides the event bus used for push notifications and news
// fan-out. Production uses Kafka; when GONEWS_KAFKA_BROKERS is empty an
// in-process bus with identical semantics is used so the service and its tests
// run without external infrastructure.
package events

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gonews/gonews/internal/metrics"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Handler processes one event payload.
type Handler func(ctx context.Context, payload []byte) error

// Bus is the event transport.
type Bus interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
	Subscribe(ctx context.Context, topic, group string, handler Handler)
	Healthy(ctx context.Context) error
	Close()
	Name() string
}

// New builds a Kafka bus when brokers are configured, otherwise a local bus.
func New(brokers []string, group string, topics []string, log *slog.Logger) (Bus, error) {
	if log == nil {
		log = slog.Default()
	}
	if len(brokers) == 0 {
		log.Info("kafka is not configured; using in-process event bus")
		return NewLocal(log), nil
	}
	return NewKafka(brokers, group, topics, log)
}

// --- Kafka ---

type Kafka struct {
	client   *kgo.Client
	log      *slog.Logger
	mu       sync.Mutex
	handlers map[string]Handler
	started  bool
}

// NewKafka creates a producer/consumer pair sharing one franz-go client.
func NewKafka(brokers []string, group string, topics []string, log *slog.Logger) (*Kafka, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ClientID("gonews"),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.AllowAutoTopicCreation(),
		kgo.SessionTimeout(15 * time.Second),
		kgo.HeartbeatInterval(3 * time.Second),
		kgo.ProduceRequestTimeout(10 * time.Second),
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, err
	}
	log.Info("kafka connected", "brokers", brokers, "group", group, "topics", topics)
	return &Kafka{client: client, log: log, handlers: map[string]Handler{}}, nil
}

func (k *Kafka) Name() string { return "kafka" }

func (k *Kafka) Publish(ctx context.Context, topic, key string, payload []byte) error {
	rec := &kgo.Record{Topic: topic, Key: []byte(key), Value: payload}
	res := k.client.ProduceSync(ctx, rec)
	if err := res.FirstErr(); err != nil {
		metrics.KafkaMessages.WithLabelValues("produce", topic, "error").Inc()
		k.log.Warn("kafka publish failed", "topic", topic, "error", err)
		return err
	}
	metrics.KafkaMessages.WithLabelValues("produce", topic, "ok").Inc()
	return nil
}

func (k *Kafka) Subscribe(ctx context.Context, topic, _ string, handler Handler) {
	k.mu.Lock()
	k.handlers[topic] = handler
	start := !k.started
	k.started = true
	k.mu.Unlock()
	if start {
		go k.consume(ctx)
	}
}

func (k *Kafka) consume(ctx context.Context) {
	for {
		fetches := k.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}
		for _, fetchErr := range fetches.Errors() {
			k.log.Warn("kafka fetch error", "topic", fetchErr.Topic, "error", fetchErr.Err)
		}
		fetches.EachRecord(func(rec *kgo.Record) {
			k.mu.Lock()
			handler := k.handlers[rec.Topic]
			k.mu.Unlock()
			if handler == nil {
				return
			}
			started := time.Now()
			err := handler(ctx, rec.Value)
			if err != nil {
				metrics.KafkaMessages.WithLabelValues("consume", rec.Topic, "error").Inc()
				k.log.Warn("kafka handler failed", "topic", rec.Topic, "partition", rec.Partition, "offset", rec.Offset, "error", err)
				return
			}
			metrics.KafkaMessages.WithLabelValues("consume", rec.Topic, "ok").Inc()
			k.log.Debug("kafka event handled", "topic", rec.Topic, "offset", rec.Offset, "duration_ms", time.Since(started).Milliseconds())
		})
		if err := k.client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			k.log.Warn("kafka commit failed", "error", err)
		}
	}
}

func (k *Kafka) Healthy(ctx context.Context) error {
	return k.client.Ping(ctx)
}

func (k *Kafka) Close() { k.client.Close() }

// --- Local bus ---

type localMessage struct {
	topic   string
	payload []byte
}

type Local struct {
	ch       chan localMessage
	log      *slog.Logger
	mu       sync.Mutex
	handlers map[string]Handler
	once     sync.Once
	closed   chan struct{}
}

func NewLocal(log *slog.Logger) *Local {
	return &Local{
		ch:       make(chan localMessage, 4096),
		log:      log,
		handlers: map[string]Handler{},
		closed:   make(chan struct{}),
	}
}

func (l *Local) Name() string { return "local" }

func (l *Local) Publish(_ context.Context, topic, _ string, payload []byte) error {
	select {
	case l.ch <- localMessage{topic: topic, payload: payload}:
		metrics.KafkaMessages.WithLabelValues("produce", topic, "ok").Inc()
		metrics.PushQueueDepth.Set(float64(len(l.ch)))
		return nil
	default:
		metrics.KafkaMessages.WithLabelValues("produce", topic, "dropped").Inc()
		l.log.Warn("local event queue full; dropping event", "topic", topic)
		return nil
	}
}

func (l *Local) Subscribe(ctx context.Context, topic, _ string, handler Handler) {
	l.mu.Lock()
	l.handlers[topic] = handler
	l.mu.Unlock()
	l.once.Do(func() { go l.run(ctx) })
}

func (l *Local) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.closed:
			return
		case msg := <-l.ch:
			metrics.PushQueueDepth.Set(float64(len(l.ch)))
			l.mu.Lock()
			handler := l.handlers[msg.topic]
			l.mu.Unlock()
			if handler == nil {
				continue
			}
			if err := handler(ctx, msg.payload); err != nil {
				metrics.KafkaMessages.WithLabelValues("consume", msg.topic, "error").Inc()
				l.log.Warn("local event handler failed", "topic", msg.topic, "error", err)
			} else {
				metrics.KafkaMessages.WithLabelValues("consume", msg.topic, "ok").Inc()
			}
		}
	}
}

func (l *Local) Healthy(context.Context) error { return nil }

func (l *Local) Close() {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
}
