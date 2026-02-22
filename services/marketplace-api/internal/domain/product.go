package domain

import (
	"time"

	"github.com/google/uuid"
)

type ProductStatus string

const (
	ProductStatusActive   ProductStatus = "ACTIVE"
	ProductStatusInactive ProductStatus = "INACTIVE"
	ProductStatusArchived ProductStatus = "ARCHIVED"
)

type Product struct {
	ID          uuid.UUID
	Name        string
	Description *string
	Price       float64
	Stock       int
	Category    string
	Status      ProductStatus
	SellerID    uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ProductFilter struct {
	Category *string
	Status   *ProductStatus
	SellerID *uuid.UUID
	Page     int
	Size     int
}

type ProductList struct {
	Items         []Product
	TotalElements int
}
