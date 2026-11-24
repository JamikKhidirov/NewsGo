// Package metrics owns the Prometheus registry and all application metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry is the single registry exposed on /metrics.
var Registry = prometheus.NewRegistry()

var (
	// HTTPRequests counts requests by method, chi route pattern and status.
	HTTPRequests = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_http_requests_total",
		Help: "HTTP requests processed by GoNews.",
	}, []string{"method", "route", "status"})

	// HTTPDuration observes request latency.
	HTTPDuration = promauto.With(Registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gonews_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 17),
	}, []string{"method", "route"})

	// HTTPInFlight tracks concurrent requests.
	HTTPInFlight = promauto.With(Registry).NewGauge(prometheus.GaugeOpts{
		Name: "gonews_http_in_flight_requests",
		Help: "Requests currently being served.",
	})

	// HTTPResponseBytes counts response bytes.
	HTTPResponseBytes = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_http_response_bytes_total",
		Help: "Response bytes written by route.",
	}, []string{"route"})

	// CacheOps counts Redis cache hits, misses, writes and errors.
	CacheOps = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_cache_operations_total",
		Help: "Cache operations by result.",
	}, []string{"op", "result"})

	// KafkaMessages counts produced and consumed Kafka messages.
	KafkaMessages = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_kafka_messages_total",
		Help: "Kafka messages by direction, topic and result.",
	}, []string{"direction", "topic", "result"})

	// KafkaLag tracks the consumer lag reported by the client.
	KafkaLag = promauto.With(Registry).NewGaugeVec(prometheus.GaugeOpts{
		Name: "gonews_kafka_consumer_lag",
		Help: "Consumer lag by topic/partition.",
	}, []string{"topic", "partition"})

	// PushDeliveries counts push delivery attempts by platform and result.
	PushDeliveries = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_push_deliveries_total",
		Help: "Push deliveries by platform and result.",
	}, []string{"platform", "result"})

	// PushQueueDepth tracks events waiting for delivery.
	PushQueueDepth = promauto.With(Registry).NewGauge(prometheus.GaugeOpts{
		Name: "gonews_push_queue_depth",
		Help: "Push events waiting in the local queue.",
	})

	// IngestRuns counts completed ingestion runs.
	IngestRuns = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "gonews_ingest_runs_total",
		Help: "Ingestion runs completed.",
	})

	// IngestArticles counts inserted and updated articles.
	IngestArticles = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_ingest_articles_total",
		Help: "Articles processed by ingestion by result.",
	}, []string{"result"})

	// IngestDuration observes ingestion run duration.
	IngestDuration = promauto.With(Registry).NewHistogram(prometheus.HistogramOpts{
		Name:    "gonews_ingest_duration_seconds",
		Help:    "Ingestion run duration.",
		Buckets: prometheus.ExponentialBuckets(0.5, 2, 10),
	})

	// IngestLastFinished is the unix timestamp of the last completed run.
	IngestLastFinished = promauto.With(Registry).NewGauge(prometheus.GaugeOpts{
		Name: "gonews_ingest_run_last_finished_timestamp_seconds",
		Help: "Unix timestamp of the last completed ingestion run.",
	})

	// ProviderErrors counts provider failures by source.
	ProviderErrors = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_provider_errors_total",
		Help: "News provider errors by source.",
	}, []string{"source"})

	// ProviderFetched counts provider item counts.
	ProviderFetched = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_provider_fetched_total",
		Help: "News items fetched by source.",
	}, []string{"source"})

	// TranslationJobs counts translation outcomes.
	TranslationJobs = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "gonews_translation_jobs_total",
		Help: "Translation jobs by result.",
	}, []string{"result"})
)

func init() {
	Registry.MustRegister(collectors.NewGoCollector())
	Registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}
