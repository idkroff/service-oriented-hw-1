package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"marketplace-api/internal/domain"
)

type PromoRepo struct {
	pool *pgxpool.Pool
}

func NewPromoRepo(pool *pgxpool.Pool) *PromoRepo {
	return &PromoRepo{pool: pool}
}

func (r *PromoRepo) Create(ctx context.Context, p *domain.PromoCode) (*domain.PromoCode, error) {
	out := &domain.PromoCode{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO promo_codes (code, discount_type, discount_value, min_order_amount, max_uses, valid_from, valid_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, code, discount_type, discount_value, min_order_amount, max_uses, current_uses, valid_from, valid_until, active`,
		p.Code, string(p.DiscountType), p.DiscountValue, p.MinOrderAmount,
		p.MaxUses, p.ValidFrom, p.ValidUntil,
	).Scan(&out.ID, &out.Code, &out.DiscountType, &out.DiscountValue,
		&out.MinOrderAmount, &out.MaxUses, &out.CurrentUses,
		&out.ValidFrom, &out.ValidUntil, &out.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return out, err
}
