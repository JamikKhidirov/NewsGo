package httpapi

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// Metrics is a tiny dependency-free Prometheus-compatible metrics registry.
type Metrics struct {
	StartedAt  time.Time
	requests   atomic.Int64
	status2xx  atomic.Int64
	status4xx  atomic.Int64
	status5xx  atomic.Int64
	ingestRuns atomic.Int64
	pushes     atomic.Int64
}

func (m *Metrics) Observe(status int) {
	m.requests.Add(1)
	switch {
	case status >= 500:
		m.status5xx.Add(1)
	case status >= 400:
		m.status4xx.Add(1)
	default:
		m.status2xx.Add(1)
	}
}

// IngestRun increments the ingestion counter.
func (m *Metrics) IngestRun() { m.ingestRuns.Add(1) }

// Push increments the push counter.
func (m *Metrics) Push() { m.pushes.Add(1) }

func (a *API) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m := a.metrics
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP gonews_uptime_seconds Process uptime in seconds.\n")
	fmt.Fprintf(w, "# TYPE gonews_uptime_seconds gauge\n")
	fmt.Fprintf(w, "gonews_uptime_seconds %.0f\n", time.Since(m.StartedAt).Seconds())
	fmt.Fprintf(w, "# HELP gonews_http_requests_total Total HTTP requests.\n")
	fmt.Fprintf(w, "# TYPE gonews_http_requests_total counter\n")
	fmt.Fprintf(w, "gonews_http_requests_total %d\n", m.requests.Load())
	fmt.Fprintf(w, "# HELP gonews_http_responses_total HTTP responses by status class.\n")
	fmt.Fprintf(w, "# TYPE gonews_http_responses_total counter\n")
	fmt.Fprintf(w, "gonews_http_responses_total{class=\"2xx\"} %d\n", m.status2xx.Load())
	fmt.Fprintf(w, "gonews_http_responses_total{class=\"4xx\"} %d\n", m.status4xx.Load())
	fmt.Fprintf(w, "gonews_http_responses_total{class=\"5xx\"} %d\n", m.status5xx.Load())
	fmt.Fprintf(w, "# HELP gonews_ingest_runs_total Ingestion runs started.\n")
	fmt.Fprintf(w, "# TYPE gonews_ingest_runs_total counter\n")
	fmt.Fprintf(w, "gonews_ingest_runs_total %d\n", m.ingestRuns.Load())
	fmt.Fprintf(w, "# HELP gonews_pushes_total Push notifications sent.\n")
	fmt.Fprintf(w, "# TYPE gonews_pushes_total counter\n")
	fmt.Fprintf(w, "gonews_pushes_total %d\n", m.pushes.Load())
}
