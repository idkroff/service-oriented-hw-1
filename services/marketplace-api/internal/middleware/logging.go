package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := uuid.New().String()
			start := time.Now()

			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			rw.Header().Set("X-Request-Id", requestID)

			var bodyLog map[string]any
			if r.Method == http.MethodPost || r.Method == http.MethodPut {
				bodyBytes, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

				if len(bodyBytes) > 0 && len(bodyBytes) < 4096 {
					if err := json.Unmarshal(bodyBytes, &bodyLog); err == nil {
						maskSensitive(bodyLog)
					}
				}
			}

			next.ServeHTTP(rw, r)

			duration := time.Since(start).Milliseconds()
			userID := ""
			if claims := UserFromCtx(r.Context()); claims != nil {
				userID = claims.UserID.String()
			}

			attrs := []any{
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("endpoint", r.URL.Path),
				slog.Int("status_code", rw.status),
				slog.Int64("duration_ms", duration),
				slog.String("user_id", userID),
				slog.String("timestamp", start.UTC().Format(time.RFC3339)),
			}
			if bodyLog != nil {
				attrs = append(attrs, slog.Any("request_body", bodyLog))
			}

			logger.Info("request", attrs...)
		})
	}
}

func maskSensitive(m map[string]any) {
	for k, v := range m {
		if strings.ToLower(k) == "password" {
			m[k] = "***"
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			maskSensitive(nested)
		}
	}
}
