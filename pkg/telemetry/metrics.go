package telemetry

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HttpRequestsTotal counts the number of HTTP requests processed
	HttpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "orbomate_http_requests_total",
			Help: "Total number of HTTP requests processed by orbo-mate",
		},
		[]string{"path", "method", "status"},
	)

	// HttpRequestDuration measures latency of HTTP requests
	HttpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "orbomate_http_request_duration_seconds",
			Help:    "Latency of HTTP requests processed by orbo-mate",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "method"},
	)

	// WebhooksProcessedTotal counts the number of review webhook triggers successfully scheduled/run
	WebhooksProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "orbomate_webhooks_processed_total",
			Help: "Total number of webhook review triggers processed successfully",
		},
		[]string{"status"},
	)
)

// PrometheusMiddleware is a chi-compatible middleware that instruments requests
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start).Seconds()
		statusStr := strconv.Itoa(ww.Status())
		path := r.URL.Path

		HttpRequestsTotal.WithLabelValues(path, r.Method, statusStr).Inc()
		HttpRequestDuration.WithLabelValues(path, r.Method).Observe(duration)
	})
}
