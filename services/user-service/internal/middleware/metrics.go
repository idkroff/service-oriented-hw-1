package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"user-service/internal/metrics"
)

func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)

			// RoutePattern доступен только после ServeHTTP, когда chi уже сматчил роут.
			pattern := chi.RouteContext(r.Context()).RoutePattern()
			if pattern == "" {
				pattern = "unknown"
			}
			status := sw.Status()
			statusStr := strconv.Itoa(status)
			duration := time.Since(start).Seconds()

			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, pattern, statusStr).Inc()
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, pattern).Observe(duration)

			switch {
			case status >= 500:
				metrics.HTTPRequestErrorsTotal.WithLabelValues(r.Method, pattern, "server_error").Inc()
			case status >= 400:
				metrics.HTTPRequestErrorsTotal.WithLabelValues(r.Method, pattern, "client_error").Inc()
			}
		})
	}
}
