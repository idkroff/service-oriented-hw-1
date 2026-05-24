package usecases

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"marketplace-api/internal/domain"
)

func TestCalcDiscount_Percentage(t *testing.T) {
	t.Parallel()
	promo := &domain.PromoCode{
		DiscountType:  domain.DiscountPercentage,
		DiscountValue: 10,
	}
	got := calcDiscount(promo, 100)
	assert.InDelta(t, 10.0, got, 0.001)
}

func TestCalcDiscount_PercentageCapped(t *testing.T) {
	t.Parallel()
	// 80% от 100 — должно быть обрезано до 70% (max).
	promo := &domain.PromoCode{
		DiscountType:  domain.DiscountPercentage,
		DiscountValue: 80,
	}
	got := calcDiscount(promo, 100)
	assert.InDelta(t, 70.0, got, 0.001)
}

func TestCalcDiscount_Fixed(t *testing.T) {
	t.Parallel()
	promo := &domain.PromoCode{
		DiscountType:  domain.DiscountFixedAmount,
		DiscountValue: 20,
	}
	got := calcDiscount(promo, 100)
	assert.InDelta(t, 20.0, got, 0.001)
}

func TestCalcDiscount_FixedClampedToTotal(t *testing.T) {
	t.Parallel()
	promo := &domain.PromoCode{
		DiscountType:  domain.DiscountFixedAmount,
		DiscountValue: 200,
	}
	got := calcDiscount(promo, 50)
	assert.InDelta(t, 50.0, got, 0.001)
}

func TestCalcDiscount_UnknownType(t *testing.T) {
	t.Parallel()
	promo := &domain.PromoCode{
		DiscountType:  "BOGUS",
		DiscountValue: 10,
	}
	assert.Zero(t, calcDiscount(promo, 100))
}
