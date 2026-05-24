package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	StatusCreated        OrderStatus = "CREATED"
	StatusPaymentPending OrderStatus = "PAYMENT_PENDING"
	StatusPaid           OrderStatus = "PAID"
	StatusShipped        OrderStatus = "SHIPPED"
	StatusCompleted      OrderStatus = "COMPLETED"
	StatusCanceled       OrderStatus = "CANCELED"
)

var validTransitions = map[OrderStatus][]OrderStatus{
	StatusCreated:        {StatusPaymentPending, StatusCanceled},
	StatusPaymentPending: {StatusPaid, StatusCanceled},
	StatusPaid:           {StatusShipped},
	StatusShipped:        {StatusCompleted},
}

func (s OrderStatus) CanTransitionTo(next OrderStatus) bool {
	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == next {
			return true
		}
	}
	return false
}

func (s OrderStatus) String() string { return string(s) }

type OrderItem struct {
	ID           uuid.UUID
	OrderID      uuid.UUID
	ProductID    uuid.UUID
	Quantity     int
	PriceAtOrder float64
}

type Order struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Status         OrderStatus
	Items          []OrderItem
	TotalAmount    float64
	DiscountAmount float64
	PromoCodeID    *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type OrderItemRequest struct {
	ProductID uuid.UUID
	Quantity  int
}

type OperationType string

const (
	OpCreateOrder OperationType = "CREATE_ORDER"
	OpUpdateOrder OperationType = "UPDATE_ORDER"
)

var ErrInvalidTransition = fmt.Errorf("invalid order status transition")
