package usecases

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"user-service/internal/domain"
)

// in-memory моки репозиториев

type userMemRepo struct {
	mu    sync.Mutex
	users []domain.User
}

func (r *userMemRepo) Create(_ context.Context, email, hash string, role domain.Role) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u := domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		Role:         role,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	r.users = append(r.users, u)
	return &u, nil
}

func (r *userMemRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.users {
		if r.users[i].Email == email {
			u := r.users[i]
			return &u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *userMemRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.users {
		if r.users[i].ID == id {
			u := r.users[i]
			return &u, nil
		}
	}
	return nil, domain.ErrNotFound
}

type rtMemRepo struct {
	mu     sync.Mutex
	tokens map[string]domain.RefreshToken
}

func (r *rtMemRepo) Create(_ context.Context, userID uuid.UUID, hash string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokens == nil {
		r.tokens = map[string]domain.RefreshToken{}
	}
	r.tokens[hash] = domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return nil
}

func (r *rtMemRepo) GetByHash(_ context.Context, hash string) (*domain.RefreshToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.tokens[hash]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &rt, nil
}

func (r *rtMemRepo) Delete(_ context.Context, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tokens, hash)
	return nil
}

// --- tests ---

func newUC(t *testing.T) (*AuthUseCase, *userMemRepo, *rtMemRepo) {
	t.Helper()
	users := &userMemRepo{}
	rts := &rtMemRepo{}
	uc := NewAuthUseCase(users, rts, "test-secret", 30, 7)
	uc.SetBcryptCost(bcrypt.MinCost)
	return uc, users, rts
}

func TestRegister_AndLogin(t *testing.T) {
	t.Parallel()
	uc, users, _ := newUC(t)
	ctx := context.Background()

	tokens, err := uc.Register(ctx, "a@x.com", "secret123", domain.RoleUser)
	require.NoError(t, err)
	require.NotEmpty(t, tokens.AccessToken)
	require.NotEmpty(t, tokens.RefreshToken)
	assert.Len(t, users.users, 1)

	tokens2, err := uc.Login(ctx, "a@x.com", "secret123")
	require.NoError(t, err)
	require.NotEmpty(t, tokens2.AccessToken)
}

func TestRegister_EmailTaken(t *testing.T) {
	t.Parallel()
	uc, _, _ := newUC(t)
	ctx := context.Background()
	_, err := uc.Register(ctx, "dup@x.com", "secret123", domain.RoleUser)
	require.NoError(t, err)

	_, err = uc.Register(ctx, "dup@x.com", "secret123", domain.RoleUser)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrEmailTaken))
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	uc, _, _ := newUC(t)
	ctx := context.Background()
	_, err := uc.Register(ctx, "b@x.com", "right-pass", domain.RoleUser)
	require.NoError(t, err)

	_, err = uc.Login(ctx, "b@x.com", "wrong-pass")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidCredentials))
}

func TestLogin_UnknownEmail(t *testing.T) {
	t.Parallel()
	uc, _, _ := newUC(t)
	_, err := uc.Login(context.Background(), "ghost@x.com", "pass")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidCredentials))
}

func TestVerifyAccessToken_Valid(t *testing.T) {
	t.Parallel()
	uc, _, _ := newUC(t)
	tokens, err := uc.Register(context.Background(), "c@x.com", "secret123", domain.RoleSeller)
	require.NoError(t, err)

	claims, err := uc.VerifyAccessToken(tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, domain.RoleSeller, claims.Role)
}

func TestVerifyAccessToken_BadSecret(t *testing.T) {
	t.Parallel()
	uc, _, _ := newUC(t)
	tokens, err := uc.Register(context.Background(), "d@x.com", "secret123", domain.RoleUser)
	require.NoError(t, err)

	other := NewAuthUseCase(&userMemRepo{}, &rtMemRepo{}, "other-secret", 30, 7)
	_, err = other.VerifyAccessToken(tokens.AccessToken)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrTokenInvalid))
}

func TestVerifyAccessToken_Garbage(t *testing.T) {
	t.Parallel()
	uc, _, _ := newUC(t)
	_, err := uc.VerifyAccessToken("not-a-jwt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrTokenInvalid))
}

func TestRefresh_ExpiredToken(t *testing.T) {
	t.Parallel()
	uc, _, rts := newUC(t)
	ctx := context.Background()
	tokens, err := uc.Register(ctx, "e@x.com", "secret123", domain.RoleUser)
	require.NoError(t, err)

	// «портим» срок жизни refresh-токена
	rts.mu.Lock()
	for k, v := range rts.tokens {
		v.ExpiresAt = time.Now().Add(-1 * time.Hour)
		rts.tokens[k] = v
	}
	rts.mu.Unlock()

	_, err = uc.Refresh(ctx, tokens.RefreshToken)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrTokenExpired))
}

func TestRefresh_UnknownToken(t *testing.T) {
	t.Parallel()
	uc, _, _ := newUC(t)
	_, err := uc.Refresh(context.Background(), "no-such-token")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrTokenInvalid))
}

func TestRefresh_RotatesToken(t *testing.T) {
	t.Parallel()
	uc, _, _ := newUC(t)
	ctx := context.Background()
	first, err := uc.Register(ctx, "f@x.com", "secret123", domain.RoleUser)
	require.NoError(t, err)

	second, err := uc.Refresh(ctx, first.RefreshToken)
	require.NoError(t, err)
	assert.NotEqual(t, first.RefreshToken, second.RefreshToken)

	// старый токен уже нельзя использовать
	_, err = uc.Refresh(ctx, first.RefreshToken)
	require.Error(t, err)
}

func TestTokenHash_Stable(t *testing.T) {
	t.Parallel()
	a := TokenHash("hello")
	b := TokenHash("hello")
	assert.Equal(t, a, b)
	assert.NotEqual(t, a, TokenHash("world"))
}
