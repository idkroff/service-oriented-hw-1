package usecases

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"marketplace-api/internal/domain"
)

type UserRepo interface {
	Create(ctx context.Context, email, passwordHash string, role domain.Role) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

type RefreshTokenRepo interface {
	Create(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	Delete(ctx context.Context, tokenHash string) error
}

type AuthUseCase struct {
	userRepo    UserRepo
	rtRepo      RefreshTokenRepo
	jwtSecret   []byte
	accessTTL   time.Duration
	refreshTTL  time.Duration
}

func NewAuthUseCase(userRepo UserRepo, rtRepo RefreshTokenRepo, jwtSecret string, accessTTLMin, refreshTTLDays int) *AuthUseCase {
	return &AuthUseCase{
		userRepo:   userRepo,
		rtRepo:     rtRepo,
		jwtSecret:  []byte(jwtSecret),
		accessTTL:  time.Duration(accessTTLMin) * time.Minute,
		refreshTTL: time.Duration(refreshTTLDays) * 24 * time.Hour,
	}
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func (uc *AuthUseCase) Register(ctx context.Context, email, password string, role domain.Role) (*TokenPair, error) {
	if _, err := uc.userRepo.GetByEmail(ctx, email); err == nil {
		return nil, domain.ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user, err := uc.userRepo.Create(ctx, email, string(hash), role)
	if err != nil {
		return nil, err
	}

	return uc.issueTokens(ctx, user)
}

func (uc *AuthUseCase) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := uc.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	return uc.issueTokens(ctx, user)
}

func (uc *AuthUseCase) Refresh(ctx context.Context, rawToken string) (*TokenPair, error) {
	hash := tokenHash(rawToken)
	rt, err := uc.rtRepo.GetByHash(ctx, hash)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	if time.Now().After(rt.ExpiresAt) {
		_ = uc.rtRepo.Delete(ctx, hash)
		return nil, domain.ErrTokenExpired
	}

	user, err := uc.userRepo.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	_ = uc.rtRepo.Delete(ctx, hash)
	return uc.issueTokens(ctx, user)
}

func (uc *AuthUseCase) VerifyAccessToken(tokenStr string) (*domain.UserClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, domain.ErrTokenInvalid
		}
		return uc.jwtSecret, nil
	})
	if err != nil {
		if err.Error() == "token is expired" {
			return nil, domain.ErrTokenExpired
		}
		return nil, domain.ErrTokenInvalid
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, domain.ErrTokenInvalid
	}

	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	userID, err := uuid.Parse(sub)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	return &domain.UserClaims{UserID: userID, Role: domain.Role(role)}, nil
}

func (uc *AuthUseCase) issueTokens(ctx context.Context, user *domain.User) (*TokenPair, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":  user.ID.String(),
		"role": string(user.Role),
		"exp":  now.Add(uc.accessTTL).Unix(),
		"iat":  now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString(uc.jwtSecret)
	if err != nil {
		return nil, err
	}

	rawRefresh := fmt.Sprintf("%s.%d.%s", user.ID, now.UnixNano(), uuid.New())
	hash := tokenHash(rawRefresh)
	expiresAt := now.Add(uc.refreshTTL)

	if err := uc.rtRepo.Create(ctx, user.ID, hash, expiresAt); err != nil {
		return nil, err
	}

	return &TokenPair{AccessToken: accessToken, RefreshToken: rawRefresh}, nil
}

func tokenHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}
