package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// Status возвращает HTTP-статус, который успешно записан в ответ.
func (sw *statusWriter) Status() int {
	if sw.status == 0 {
		return http.StatusOK
	}
	return sw.status
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := uuid.New().String()
			start := time.Now()

			sw := &statusWriter{ResponseWriter: w}
			sw.Header().Set("X-Request-Id", requestID)

			next.ServeHTTP(sw, r)

			logger.Info("request",
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("endpoint", r.URL.Path),
				slog.Int("status_code", sw.Status()),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}
