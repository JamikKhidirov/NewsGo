package httpapi

import (
	"net/http"
	"time"

	"github.com/gonews/gonews/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics keeps process-level metadata for the API.
type Metrics struct {
	StartedAt time.Time
}

// IngestRun increments the ingestion counter.
func (m *Metrics) IngestRun() { metrics.IngestRuns.Inc() }

// Push increments the push counter (deliveries are counted by the worker).
func (m *Metrics) Push() {}

func (a *API) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorHandling:     promhttp.ContinueOnError,
	}).ServeHTTP(w, r)
}
