package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"marketplace-api/internal/domain"
)

type contextKey string

const CtxKeyUser contextKey = "user"

// TokenVerifier — интерфейс валидации access-токена.
// Реализуется HTTP-клиентом к user-service (`internal/clients/userclient.go`).
type TokenVerifier interface {
	VerifyAccessToken(token string) (*domain.UserClaims, error)
}

func Auth(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := parseToken(verifier, r.Header.Get("Authorization"))
			if err != nil {
				code, status := authErrInfo(err)
				writeAuthError(w, code, err.Error(), status)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), CtxKeyUser, claims)))
		})
	}
}

func AuthOptional(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, _ := parseToken(verifier, r.Header.Get("Authorization"))
			if claims != nil {
				r = r.WithContext(context.WithValue(r.Context(), CtxKeyUser, claims))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseToken(verifier TokenVerifier, authHeader string) (*domain.UserClaims, error) {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, domain.ErrUnauthorized
	}
	return verifier.VerifyAccessToken(strings.TrimPrefix(authHeader, "Bearer "))
}

func authErrInfo(err error) (string, int) {
	switch err {
	case domain.ErrTokenExpired:
		return "TOKEN_EXPIRED", http.StatusUnauthorized
	case domain.ErrUnauthorized:
		return "TOKEN_MISSING", http.StatusUnauthorized
	case domain.ErrAuthUnavailable:
		return "AUTH_UNAVAILABLE", http.StatusServiceUnavailable
	default:
		return "TOKEN_INVALID", http.StatusUnauthorized
	}
}

func writeAuthError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error_code": code,
		"message":    message,
	})
}

func UserFromCtx(ctx context.Context) *domain.UserClaims {
	v, _ := ctx.Value(CtxKeyUser).(*domain.UserClaims)
	return v
}
