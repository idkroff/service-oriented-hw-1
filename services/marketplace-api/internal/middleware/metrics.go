package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"marketplace-api/internal/metrics"
)

// Metrics — middleware, собирающий стандартные RED-метрики.
// RoutePattern читается ПОСЛЕ next.ServeHTTP, когда chi уже сматчил маршрут,
// иначе значение endpoint было бы пустым (см. .plan-review.md).
func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			pattern := chi.RouteContext(r.Context()).RoutePattern()
			if pattern == "" {
				pattern = "unknown"
			}
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
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
