package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marketplace-api/internal/domain"
)

func TestUserClient_Validate_Success(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/validate", r.URL.Path)
		assert.Equal(t, "Bearer good-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"user_id":"` + userID.String() + `","role":"USER"}`))
	}))
	defer srv.Close()

	c := NewUserClient(srv.URL)
	claims, err := c.VerifyAccessToken("good-token")
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, domain.Role("USER"), claims.Role)
}

func TestUserClient_Validate_TokenInvalid(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error_code":"TOKEN_INVALID","message":"bad"}`))
	}))
	defer srv.Close()

	c := NewUserClient(srv.URL)
	_, err := c.VerifyAccessToken("bad")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrTokenInvalid))
}

func TestUserClient_Validate_TokenExpired(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error_code":"TOKEN_EXPIRED","message":"expired"}`))
	}))
	defer srv.Close()

	c := NewUserClient(srv.URL)
	_, err := c.VerifyAccessToken("expired")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrTokenExpired))
}

func TestUserClient_Validate_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewUserClient(srv.URL)
	_, err := c.VerifyAccessToken("x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAuthUnavailable))
}

func TestUserClient_Validate_Unreachable(t *testing.T) {
	t.Parallel()

	// несуществующий порт — connection refused
	c := NewUserClient("http://127.0.0.1:1")
	_, err := c.VerifyAccessToken("x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAuthUnavailable))
}

func TestUserClient_Validate_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewUserClient(srv.URL)
	// override таймаут на короткий, чтобы тест прошёл быстро
	c.httpClient.Timeout = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := c.VerifyAccessTokenCtx(ctx, "x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAuthUnavailable) || errors.Is(err, context.DeadlineExceeded))
}
