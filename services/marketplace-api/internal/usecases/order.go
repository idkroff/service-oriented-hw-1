package usecases

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"marketplace-api/internal/domain"
	"marketplace-api/internal/repository"
)

type OrderRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error
}

type PromoRepository interface {
	Create(ctx context.Context, p *domain.PromoCode) (*domain.PromoCode, error)
}

type OrderUseCase struct {
	orderRepo        OrderRepository
	rateLimitMinutes int
}

func NewOrderUseCase(orderRepo OrderRepository, rateLimitMinutes int) *OrderUseCase {
	return &OrderUseCase{
		orderRepo:        orderRepo,
		rateLimitMinutes: rateLimitMinutes,
	}
}

func (uc *OrderUseCase) GetByID(ctx context.Context, id uuid.UUID, claims domain.UserClaims) (*domain.Order, error) {
	order, err := uc.orderRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if claims.Role != domain.RoleAdmin && order.UserID != claims.UserID {
		return nil, domain.ErrForbidden
	}
	return order, nil
}

func (uc *OrderUseCase) Create(ctx context.Context, userID uuid.UUID, items []domain.OrderItemRequest, promoCode *string) (*domain.Order, error) {
	var result *domain.Order

	err := uc.orderRepo.WithTx(ctx, func(tx pgx.Tx) error {
		limited, err := repository.RateLimitCheck(ctx, tx, userID, domain.OpCreateOrder, uc.rateLimitMinutes)
		if err != nil {
			return err
		}
		if limited {
			return domain.ErrOrderLimitExceeded
		}

		hasActive, err := repository.HasActiveOrder(ctx, tx, userID)
		if err != nil {
			return err
		}
		if hasActive {
			return domain.ErrActiveOrderExists
		}

		orderItems, total, err := uc.reserveItems(ctx, tx, items)
		if err != nil {
			return err
		}

		var promoID *uuid.UUID
		discount := 0.0
		if promoCode != nil && *promoCode != "" {
			promo, err := repository.LockPromo(ctx, tx, *promoCode)
			if err != nil || !promo.Active || promo.CurrentUses >= promo.MaxUses ||
				time.Now().Before(promo.ValidFrom) || time.Now().After(promo.ValidUntil) {
				return domain.ErrPromoInvalid
			}
			if total < promo.MinOrderAmount {
				return domain.ErrPromoMinAmount
			}
			discount = calcDiscount(promo, total)
			promoID = &promo.ID
			if err := repository.IncrementPromoUses(ctx, tx, promo.ID); err != nil {
				return err
			}
		}

		order, err := repository.InsertOrder(ctx, tx, domain.Order{
			UserID:         userID,
			Status:         domain.StatusCreated,
			Items:          orderItems,
			TotalAmount:    total,
			DiscountAmount: discount,
			PromoCodeID:    promoID,
		})
		if err != nil {
			return err
		}

		for i := range orderItems {
			orderItems[i].OrderID = order.ID
		}
		if err := repository.InsertOrderItems(ctx, tx, order.ID, orderItems); err != nil {
			return err
		}

		if err := repository.InsertUserOp(ctx, tx, userID, domain.OpCreateOrder); err != nil {
			return err
		}

		result = &order
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (uc *OrderUseCase) Update(ctx context.Context, orderID uuid.UUID, claims domain.UserClaims, items []domain.OrderItemRequest) (*domain.Order, error) {
	var result *domain.Order

	err := uc.orderRepo.WithTx(ctx, func(tx pgx.Tx) error {
		order, err := repository.GetOrderForUpdate(ctx, tx, orderID)
		if err != nil {
			return err
		}

		if claims.Role != domain.RoleAdmin && order.UserID != claims.UserID {
			return domain.ErrForbidden
		}

		if order.Status != domain.StatusCreated {
			return domain.ErrInvalidTransition
		}

		limited, err := repository.RateLimitCheck(ctx, tx, claims.UserID, domain.OpUpdateOrder, uc.rateLimitMinutes)
		if err != nil {
			return err
		}
		if limited {
			return domain.ErrOrderLimitExceeded
		}

		oldItems, err := repository.GetOrderItems(ctx, tx, orderID)
		if err != nil {
			return err
		}
		for _, item := range oldItems {
			if err := repository.IncreaseStock(ctx, tx, item.ProductID, item.Quantity); err != nil {
				return err
			}
		}

		newItems, total, err := uc.reserveItems(ctx, tx, items)
		if err != nil {
			return err
		}

		discount := 0.0
		promoID := order.PromoCodeID
		if order.PromoCodeID != nil {
			promo, err := repository.LockPromoByID(ctx, tx, *order.PromoCodeID)
			if err != nil {
				if err := repository.DecrementPromoUses(ctx, tx, *order.PromoCodeID); err != nil {
					return err
				}
				promoID = nil
			} else {
				if total < promo.MinOrderAmount {
					if err := repository.DecrementPromoUses(ctx, tx, promo.ID); err != nil {
						return err
					}
					promoID = nil
				} else {
					discount = calcDiscount(promo, total)
				}
			}
		}
		if err := repository.DeleteOrderItems(ctx, tx, orderID); err != nil {
			return err
		}
		for i := range newItems {
			newItems[i].OrderID = orderID
		}
		if err := repository.InsertOrderItems(ctx, tx, orderID, newItems); err != nil {
			return err
		}

		order.TotalAmount = total
		order.DiscountAmount = discount
		order.PromoCodeID = promoID
		order.Items = newItems

		if err := repository.UpdateOrderTx(ctx, tx, order); err != nil {
			return err
		}

		if err := repository.InsertUserOp(ctx, tx, claims.UserID, domain.OpUpdateOrder); err != nil {
			return err
		}

		result = order
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (uc *OrderUseCase) Cancel(ctx context.Context, orderID uuid.UUID, claims domain.UserClaims) (*domain.Order, error) {
	var result *domain.Order

	err := uc.orderRepo.WithTx(ctx, func(tx pgx.Tx) error {
		order, err := repository.GetOrderForUpdate(ctx, tx, orderID)
		if err != nil {
			return err
		}

		if claims.Role != domain.RoleAdmin && order.UserID != claims.UserID {
			return domain.ErrForbidden
		}

		if order.Status != domain.StatusCreated && order.Status != domain.StatusPaymentPending {
			return domain.ErrInvalidTransition
		}

		items, err := repository.GetOrderItems(ctx, tx, orderID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := repository.IncreaseStock(ctx, tx, item.ProductID, item.Quantity); err != nil {
				return err
			}
		}

		if order.PromoCodeID != nil {
			if err := repository.DecrementPromoUses(ctx, tx, *order.PromoCodeID); err != nil {
				return err
			}
		}

		if err := repository.CancelOrderTx(ctx, tx, order); err != nil {
			return err
		}

		order.Items = items
		result = order
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (uc *OrderUseCase) reserveItems(ctx context.Context, tx pgx.Tx, items []domain.OrderItemRequest) ([]domain.OrderItem, float64, error) {
	var orderItems []domain.OrderItem
	total := 0.0

	for _, req := range items {
		product, err := repository.LockProduct(ctx, tx, req.ProductID)
		if err != nil {
			return nil, 0, err
		}
		if product.Status != domain.ProductStatusActive {
			return nil, 0, domain.ErrProductInactive
		}
		if product.Stock < req.Quantity {
			return nil, 0, domain.ErrInsufficientStock
		}

		if err := repository.DecreaseStock(ctx, tx, req.ProductID, req.Quantity); err != nil {
			return nil, 0, err
		}

		total += product.Price * float64(req.Quantity)
		orderItems = append(orderItems, domain.OrderItem{
			ProductID:    req.ProductID,
			Quantity:     req.Quantity,
			PriceAtOrder: product.Price,
		})
	}

	return orderItems, total, nil
}

func calcDiscount(promo *domain.PromoCode, total float64) float64 {
	switch promo.DiscountType {
	case domain.DiscountPercentage:
		d := total * promo.DiscountValue / 100
		max := total * 0.7
		if d > max {
			d = max
		}
		return d
	case domain.DiscountFixedAmount:
		d := promo.DiscountValue
		if d > total {
			d = total
		}
		return d
	}
	return 0
}

