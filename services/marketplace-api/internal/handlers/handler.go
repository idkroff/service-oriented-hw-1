package handlers

import (
	"context"

	"github.com/google/uuid"

	"marketplace-api/internal/domain"
	"marketplace-api/internal/generated"
	"marketplace-api/internal/middleware"
	"marketplace-api/internal/usecases"
)

type Handler struct {
	productUC *usecases.ProductUseCase
	orderUC   *usecases.OrderUseCase
	promoUC   *usecases.PromoUseCase
}

func New(
	productUC *usecases.ProductUseCase,
	orderUC *usecases.OrderUseCase,
	promoUC *usecases.PromoUseCase,
) *Handler {
	return &Handler{
		productUC: productUC,
		orderUC:   orderUC,
		promoUC:   promoUC,
	}
}

func userFromCtx(ctx context.Context) *domain.UserClaims {
	return middleware.UserFromCtx(ctx)
}

func errResp(code, message string) generated.Error {
	return generated.Error{ErrorCode: code, Message: message}
}

func toProductResponse(p *domain.Product) generated.ProductResponse {
	resp := generated.ProductResponse{
		Id:          p.ID,
		Name:        p.Name,
		Price:       p.Price,
		Stock:       p.Stock,
		Category:    p.Category,
		Status:      generated.ProductStatus(p.Status),
		SellerId:    p.SellerID,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	if p.Description != nil {
		resp.Description = p.Description
	}
	return resp
}

func toOrderResponse(o *domain.Order) generated.OrderResponse {
	items := make([]generated.OrderItemResponse, len(o.Items))
	for i, item := range o.Items {
		items[i] = generated.OrderItemResponse{
			Id:           item.ID,
			ProductId:    item.ProductID,
			Quantity:     item.Quantity,
			PriceAtOrder: item.PriceAtOrder,
		}
	}
	resp := generated.OrderResponse{
		Id:             o.ID,
		UserId:         o.UserID,
		Status:         generated.OrderStatus(o.Status),
		Items:          items,
		TotalAmount:    o.TotalAmount,
		DiscountAmount: o.DiscountAmount,
		CreatedAt:      o.CreatedAt,
		UpdatedAt:      o.UpdatedAt,
	}
	if o.PromoCodeID != nil {
		id := uuid.UUID(*o.PromoCodeID)
		resp.PromoCodeId = &id
	}
	return resp
}

func toOrderItems(items []generated.OrderItemRequest) []domain.OrderItemRequest {
	result := make([]domain.OrderItemRequest, len(items))
	for i, item := range items {
		result[i] = domain.OrderItemRequest{
			ProductID: uuid.UUID(item.ProductId),
			Quantity:  item.Quantity,
		}
	}
	return result
}

func mapDomainError(err error) (string, string) {
	switch err {
	case domain.ErrNotFound:
		return "NOT_FOUND", "resource not found"
	case domain.ErrForbidden:
		return "ACCESS_DENIED", "access denied"
	case domain.ErrConflict:
		return "CONFLICT", "conflict"
	case domain.ErrUnauthorized, domain.ErrTokenInvalid:
		return "TOKEN_INVALID", "invalid token"
	case domain.ErrTokenExpired:
		return "TOKEN_EXPIRED", "token expired"
	case domain.ErrAuthUnavailable:
		return "AUTH_UNAVAILABLE", "auth service unavailable"
	case domain.ErrProductInactive:
		return "PRODUCT_INACTIVE", "product is not available"
	case domain.ErrInsufficientStock:
		return "INSUFFICIENT_STOCK", "insufficient stock for one or more products"
	case domain.ErrActiveOrderExists:
		return "ORDER_HAS_ACTIVE", "user already has an active order"
	case domain.ErrOrderLimitExceeded:
		return "ORDER_LIMIT_EXCEEDED", "order rate limit exceeded, try again later"
	case domain.ErrPromoInvalid:
		return "PROMO_CODE_INVALID", "promo code is invalid or expired"
	case domain.ErrPromoMinAmount:
		return "PROMO_CODE_MIN_AMOUNT", "order total is below promo minimum amount"
	case domain.ErrInvalidTransition:
		return "INVALID_STATE_TRANSITION", "invalid order state transition"
	default:
		return "INTERNAL_ERROR", "internal server error"
	}
}

// compile-time interface check
var _ generated.StrictServerInterface = (*Handler)(nil)
