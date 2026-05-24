package handlers

import (
	"context"

	"github.com/google/uuid"

	"marketplace-api/internal/domain"
	"marketplace-api/internal/generated"
)

func (h *Handler) CreateOrder(ctx context.Context, req generated.CreateOrderRequestObject) (generated.CreateOrderResponseObject, error) {
	claims := userFromCtx(ctx)
	if claims == nil {
		return generated.CreateOrder400JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp("TOKEN_MISSING", "authentication required"))}, nil
	}

	items := toOrderItems(req.Body.Items)

	order, err := h.orderUC.Create(ctx, claims.UserID, items, req.Body.PromoCode)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrOrderLimitExceeded:
			return generated.CreateOrder429JSONResponse(errResp(code, message)), nil
		case domain.ErrActiveOrderExists, domain.ErrInsufficientStock, domain.ErrPromoInvalid, domain.ErrPromoMinAmount:
			return generated.CreateOrder409JSONResponse(errResp(code, message)), nil
		default:
			return generated.CreateOrder400JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		}
	}

	return generated.CreateOrder201JSONResponse(toOrderResponse(order)), nil
}

func (h *Handler) GetOrder(ctx context.Context, req generated.GetOrderRequestObject) (generated.GetOrderResponseObject, error) {
	claims := userFromCtx(ctx)
	if claims == nil {
		return generated.GetOrder403JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp("ACCESS_DENIED", "authentication required"))}, nil
	}

	order, err := h.orderUC.GetByID(ctx, uuid.UUID(req.Id), *claims)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrForbidden:
			return generated.GetOrder403JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		default:
			return generated.GetOrder404JSONResponse(errResp(code, message)), nil
		}
	}

	return generated.GetOrder200JSONResponse(toOrderResponse(order)), nil
}

func (h *Handler) UpdateOrder(ctx context.Context, req generated.UpdateOrderRequestObject) (generated.UpdateOrderResponseObject, error) {
	claims := userFromCtx(ctx)
	if claims == nil {
		return generated.UpdateOrder403JSONResponse(errResp("ACCESS_DENIED", "authentication required")), nil
	}

	items := toOrderItems(req.Body.Items)

	order, err := h.orderUC.Update(ctx, uuid.UUID(req.Id), *claims, items)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrOrderLimitExceeded:
			return generated.UpdateOrder429JSONResponse(errResp(code, message)), nil
		case domain.ErrInvalidTransition, domain.ErrInsufficientStock, domain.ErrPromoInvalid, domain.ErrPromoMinAmount:
			return generated.UpdateOrder409JSONResponse(errResp(code, message)), nil
		case domain.ErrForbidden:
			return generated.UpdateOrder403JSONResponse(errResp(code, message)), nil
		default:
			return generated.UpdateOrder400JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		}
	}

	return generated.UpdateOrder200JSONResponse(toOrderResponse(order)), nil
}

func (h *Handler) CancelOrder(ctx context.Context, req generated.CancelOrderRequestObject) (generated.CancelOrderResponseObject, error) {
	claims := userFromCtx(ctx)
	if claims == nil {
		return generated.CancelOrder403JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp("ACCESS_DENIED", "authentication required"))}, nil
	}

	order, err := h.orderUC.Cancel(ctx, uuid.UUID(req.Id), *claims)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrForbidden:
			return generated.CancelOrder403JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		case domain.ErrInvalidTransition:
			return generated.CancelOrder409JSONResponse(errResp(code, message)), nil
		default:
			return generated.CancelOrder404JSONResponse(errResp(code, message)), nil
		}
	}

	return generated.CancelOrder200JSONResponse(toOrderResponse(order)), nil
}
