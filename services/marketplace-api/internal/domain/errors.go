package domain

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrForbidden          = errors.New("forbidden")
	ErrConflict           = errors.New("conflict")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrTokenInvalid       = errors.New("token invalid")
	ErrTokenExpired       = errors.New("token expired")
	ErrAuthUnavailable    = errors.New("auth service unavailable")
	ErrProductInactive    = errors.New("product is not active")
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrActiveOrderExists  = errors.New("user already has an active order")
	ErrOrderLimitExceeded = errors.New("order rate limit exceeded")
	ErrPromoInvalid       = errors.New("promo code is invalid or expired")
	ErrPromoMinAmount     = errors.New("order total is below promo minimum amount")
)
