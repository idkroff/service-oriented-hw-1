package handlers

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"marketplace-api/internal/domain"
)

func TestMapDomainError_KnownErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err      error
		wantCode string
	}{
		{domain.ErrNotFound, "NOT_FOUND"},
		{domain.ErrForbidden, "ACCESS_DENIED"},
		{domain.ErrConflict, "CONFLICT"},
		{domain.ErrTokenInvalid, "TOKEN_INVALID"},
		{domain.ErrUnauthorized, "TOKEN_INVALID"},
		{domain.ErrTokenExpired, "TOKEN_EXPIRED"},
		{domain.ErrAuthUnavailable, "AUTH_UNAVAILABLE"},
		{domain.ErrProductInactive, "PRODUCT_INACTIVE"},
		{domain.ErrInsufficientStock, "INSUFFICIENT_STOCK"},
		{domain.ErrActiveOrderExists, "ORDER_HAS_ACTIVE"},
		{domain.ErrOrderLimitExceeded, "ORDER_LIMIT_EXCEEDED"},
		{domain.ErrPromoInvalid, "PROMO_CODE_INVALID"},
		{domain.ErrPromoMinAmount, "PROMO_CODE_MIN_AMOUNT"},
		{domain.ErrInvalidTransition, "INVALID_STATE_TRANSITION"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.wantCode, func(t *testing.T) {
			t.Parallel()
			code, msg := mapDomainError(c.err)
			assert.Equal(t, c.wantCode, code)
			assert.NotEmpty(t, msg)
		})
	}
}

func TestMapDomainError_Unknown(t *testing.T) {
	t.Parallel()
	code, msg := mapDomainError(errors.New("some random error"))
	assert.Equal(t, "INTERNAL_ERROR", code)
	assert.Equal(t, "internal server error", msg)
}
