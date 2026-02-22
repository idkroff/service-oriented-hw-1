package usecases

import (
	"context"

	"marketplace-api/internal/domain"
)

type PromoUseCase struct {
	repo PromoRepository
}

func NewPromoUseCase(repo PromoRepository) *PromoUseCase {
	return &PromoUseCase{repo: repo}
}

func (uc *PromoUseCase) Create(ctx context.Context, claims domain.UserClaims, p *domain.PromoCode) (*domain.PromoCode, error) {
	if claims.Role != domain.RoleSeller && claims.Role != domain.RoleAdmin {
		return nil, domain.ErrForbidden
	}
	return uc.repo.Create(ctx, p)
}
