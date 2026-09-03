package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"marketplace-api/internal/domain"
)

// AuthValidateCallsTotal — счётчик кросс-сервисных вызовов /auth/validate.
// outcome ∈ {success, client_error, server_error, timeout, unreachable}.
var AuthValidateCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "auth_validate_calls_total",
	Help: "Total auth-validate calls from marketplace-api to user-service.",
}, []string{"outcome"})

// UserClient вызывает /auth/validate в user-service и кэширует claims в рамках
// одного запроса. Реализует middleware.TokenVerifier.
type UserClient struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

func NewUserClient(baseURL string) *UserClient {
	return &UserClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
		maxRetries: 1,
	}
}

// VerifyAccessToken отправляет токен в user-service и возвращает claims.
// Ошибки:
//   - domain.ErrTokenExpired — user-service вернул 401 с TOKEN_EXPIRED
//   - domain.ErrTokenInvalid — user-service вернул 401 с другим кодом
//   - domain.ErrAuthUnavailable — сетевая ошибка / 5xx
func (c *UserClient) VerifyAccessToken(token string) (*domain.UserClaims, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return c.VerifyAccessTokenCtx(ctx, token)
}

func (c *UserClient) VerifyAccessTokenCtx(ctx context.Context, token string) (*domain.UserClaims, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		claims, outcome, err := c.doValidate(ctx, token)
		if err == nil {
			AuthValidateCallsTotal.WithLabelValues(outcome).Inc()
			return claims, nil
		}

		AuthValidateCallsTotal.WithLabelValues(outcome).Inc()
		lastErr = err

		// Ошибки авторизации не ретраим — это финальный ответ.
		if errors.Is(err, domain.ErrTokenInvalid) || errors.Is(err, domain.ErrTokenExpired) {
			return nil, err
		}

		if attempt < c.maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return nil, lastErr
}

type validateErrResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func (c *UserClient) doValidate(ctx context.Context, token string) (*domain.UserClaims, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/validate", nil)
	if err != nil {
		return nil, "client_error", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, "timeout", domain.ErrAuthUnavailable
		}
		return nil, "unreachable", domain.ErrAuthUnavailable
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		var claims domain.UserClaims
		if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
			return nil, "server_error", domain.ErrAuthUnavailable
		}
		return &claims, "success", nil
	case resp.StatusCode == http.StatusUnauthorized:
		var body validateErrResponse
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body.ErrorCode == "TOKEN_EXPIRED" {
			return nil, "client_error", domain.ErrTokenExpired
		}
		return nil, "client_error", domain.ErrTokenInvalid
	case resp.StatusCode >= 500:
		return nil, "server_error", domain.ErrAuthUnavailable
	default:
		return nil, "client_error", domain.ErrTokenInvalid
	}
}
