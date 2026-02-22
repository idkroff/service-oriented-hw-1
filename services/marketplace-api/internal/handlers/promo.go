package handlers

import (
	"context"

	"marketplace-api/internal/domain"
	"marketplace-api/internal/generated"
)

func (h *Handler) CreatePromoCode(ctx context.Context, req generated.CreatePromoCodeRequestObject) (generated.CreatePromoCodeResponseObject, error) {
	claims := userFromCtx(ctx)
	if claims == nil || (claims.Role != domain.RoleSeller && claims.Role != domain.RoleAdmin) {
		return generated.CreatePromoCode403JSONResponse(errResp("ACCESS_DENIED", "only SELLER or ADMIN can create promo codes")), nil
	}

	p := &domain.PromoCode{
		Code:           req.Body.Code,
		DiscountType:   domain.DiscountType(req.Body.DiscountType),
		DiscountValue:  req.Body.DiscountValue,
		MinOrderAmount: req.Body.MinOrderAmount,
		MaxUses:        req.Body.MaxUses,
		ValidFrom:      req.Body.ValidFrom,
		ValidUntil:     req.Body.ValidUntil,
	}

	promo, err := h.promoUC.Create(ctx, *claims, p)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrForbidden:
			return generated.CreatePromoCode403JSONResponse(errResp(code, message)), nil
		default:
			return generated.CreatePromoCode400JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		}
	}

	return generated.CreatePromoCode201JSONResponse(generated.PromoCodeResponse{
		Id:             promo.ID,
		Code:           promo.Code,
		DiscountType:   generated.DiscountType(promo.DiscountType),
		DiscountValue:  promo.DiscountValue,
		MinOrderAmount: promo.MinOrderAmount,
		MaxUses:        promo.MaxUses,
		CurrentUses:    promo.CurrentUses,
		ValidFrom:      promo.ValidFrom,
		ValidUntil:     promo.ValidUntil,
		Active:         promo.Active,
	}), nil
}
