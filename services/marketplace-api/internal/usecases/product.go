package usecases

import (
	"context"

	"github.com/google/uuid"

	"marketplace-api/internal/domain"
)

type ProductRepository interface {
	Create(ctx context.Context, p *domain.Product) (*domain.Product, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
	Update(ctx context.Context, p *domain.Product) (*domain.Product, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, f domain.ProductFilter) (*domain.ProductList, error)
}

type ProductUseCase struct {
	repo ProductRepository
}

func NewProductUseCase(repo ProductRepository) *ProductUseCase {
	return &ProductUseCase{repo: repo}
}

func (uc *ProductUseCase) Create(ctx context.Context, sellerID uuid.UUID, p *domain.Product) (*domain.Product, error) {
	p.SellerID = sellerID
	if p.Status == "" {
		p.Status = domain.ProductStatusActive
	}
	return uc.repo.Create(ctx, p)
}

func (uc *ProductUseCase) GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *ProductUseCase) Update(ctx context.Context, id uuid.UUID, claims domain.UserClaims, upd *domain.Product) (*domain.Product, error) {
	existing, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if claims.Role == domain.RoleSeller && existing.SellerID != claims.UserID {
		return nil, domain.ErrForbidden
	}

	if upd.Name != "" {
		existing.Name = upd.Name
	}
	if upd.Description != nil {
		existing.Description = upd.Description
	}
	if upd.Price > 0 {
		existing.Price = upd.Price
	}
	if upd.Stock >= 0 && upd.Stock != existing.Stock {
		existing.Stock = upd.Stock
	}
	if upd.Category != "" {
		existing.Category = upd.Category
	}
	if upd.Status != "" {
		existing.Status = upd.Status
	}

	return uc.repo.Update(ctx, existing)
}

func (uc *ProductUseCase) Delete(ctx context.Context, id uuid.UUID, claims domain.UserClaims) error {
	existing, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if claims.Role == domain.RoleSeller && existing.SellerID != claims.UserID {
		return domain.ErrForbidden
	}

	return uc.repo.Delete(ctx, id)
}

func (uc *ProductUseCase) List(ctx context.Context, f domain.ProductFilter) (*domain.ProductList, error) {
	return uc.repo.List(ctx, f)
}
