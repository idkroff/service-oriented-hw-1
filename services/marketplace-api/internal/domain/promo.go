package domain

import (
	"time"

	"github.com/google/uuid"
)

type DiscountType string

const (
	DiscountPercentage  DiscountType = "PERCENTAGE"
	DiscountFixedAmount DiscountType = "FIXED_AMOUNT"
)

type PromoCode struct {
	ID              uuid.UUID
	Code            string
	DiscountType    DiscountType
	DiscountValue   float64
	MinOrderAmount  float64
	MaxUses         int
	CurrentUses     int
	ValidFrom       time.Time
	ValidUntil      time.Time
	Active          bool
}
