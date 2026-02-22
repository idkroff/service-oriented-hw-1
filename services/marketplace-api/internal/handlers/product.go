package handlers

import (
	"context"

	"github.com/google/uuid"

	"marketplace-api/internal/domain"
	"marketplace-api/internal/generated"
)

func (h *Handler) ListProducts(ctx context.Context, req generated.ListProductsRequestObject) (generated.ListProductsResponseObject, error) {
	p := req.Params

	filter := domain.ProductFilter{
		Page: 0,
		Size: 20,
	}

	if p.Page != nil {
		filter.Page = *p.Page
	}
	if p.Size != nil {
		filter.Size = *p.Size
	}
	if p.Category != nil {
		filter.Category = p.Category
	}
	if p.Status != nil {
		s := domain.ProductStatus(*p.Status)
		filter.Status = &s
	}
	if p.SellerId != nil {
		id := uuid.UUID(*p.SellerId)
		filter.SellerID = &id
	}

	list, err := h.productUC.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]generated.ProductResponse, len(list.Items))
	for i, p := range list.Items {
		p := p
		items[i] = toProductResponse(&p)
	}

	return generated.ListProducts200JSONResponse(generated.ProductListResponse{
		Items:         items,
		TotalElements: list.TotalElements,
		Page:          filter.Page,
		Size:          filter.Size,
	}), nil
}

func (h *Handler) CreateProduct(ctx context.Context, req generated.CreateProductRequestObject) (generated.CreateProductResponseObject, error) {
	claims := userFromCtx(ctx)
	if claims == nil || (claims.Role != domain.RoleSeller && claims.Role != domain.RoleAdmin) {
		return generated.CreateProduct403JSONResponse(errResp("ACCESS_DENIED", "only SELLER or ADMIN can create products")), nil
	}

	p := &domain.Product{
		Name:     req.Body.Name,
		Price:    req.Body.Price,
		Stock:    req.Body.Stock,
		Category: req.Body.Category,
	}
	if req.Body.Description != nil {
		p.Description = req.Body.Description
	}
	if req.Body.Status != nil {
		p.Status = domain.ProductStatus(*req.Body.Status)
	}

	created, err := h.productUC.Create(ctx, claims.UserID, p)
	if err != nil {
		return generated.CreateProduct400JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp("INTERNAL_ERROR", err.Error()))}, nil
	}

	return generated.CreateProduct201JSONResponse(toProductResponse(created)), nil
}

func (h *Handler) GetProduct(ctx context.Context, req generated.GetProductRequestObject) (generated.GetProductResponseObject, error) {
	product, err := h.productUC.GetByID(ctx, uuid.UUID(req.Id))
	if err != nil {
		code, message := mapDomainError(err)
		return generated.GetProduct404JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
	}
	return generated.GetProduct200JSONResponse(toProductResponse(product)), nil
}

func (h *Handler) UpdateProduct(ctx context.Context, req generated.UpdateProductRequestObject) (generated.UpdateProductResponseObject, error) {
	claims := userFromCtx(ctx)
	if claims == nil {
		return generated.UpdateProduct403JSONResponse(errResp("ACCESS_DENIED", "authentication required")), nil
	}

	upd := &domain.Product{}
	if req.Body.Name != nil {
		upd.Name = *req.Body.Name
	}
	if req.Body.Description != nil {
		upd.Description = req.Body.Description
	}
	if req.Body.Price != nil {
		upd.Price = *req.Body.Price
	}
	if req.Body.Stock != nil {
		upd.Stock = *req.Body.Stock
	}
	if req.Body.Category != nil {
		upd.Category = *req.Body.Category
	}
	if req.Body.Status != nil {
		upd.Status = domain.ProductStatus(*req.Body.Status)
	}

	product, err := h.productUC.Update(ctx, uuid.UUID(req.Id), *claims, upd)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrNotFound:
			return generated.UpdateProduct404JSONResponse(errResp(code, message)), nil
		case domain.ErrForbidden:
			return generated.UpdateProduct403JSONResponse(errResp(code, message)), nil
		default:
			return generated.UpdateProduct400JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		}
	}

	return generated.UpdateProduct200JSONResponse(toProductResponse(product)), nil
}

func (h *Handler) DeleteProduct(ctx context.Context, req generated.DeleteProductRequestObject) (generated.DeleteProductResponseObject, error) {
	claims := userFromCtx(ctx)
	if claims == nil {
		return generated.DeleteProduct403JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp("ACCESS_DENIED", "authentication required"))}, nil
	}

	err := h.productUC.Delete(ctx, uuid.UUID(req.Id), *claims)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrNotFound:
			return generated.DeleteProduct404JSONResponse(errResp(code, message)), nil
		default:
			return generated.DeleteProduct403JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		}
	}

	return generated.DeleteProduct204Response{}, nil
}
