package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"marketplace-api/internal/domain"
)

type OrderRepo struct {
	pool *pgxpool.Pool
}

func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo {
	return &OrderRepo{pool: pool}
}

func (r *OrderRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	o := &domain.Order{}
	err := r.pool.QueryRow(
		ctx,
		`SELECT id, user_id, status, promo_code_id, total_amount, discount_amount, created_at, updated_at
		FROM orders WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.UserID, &o.Status, &o.PromoCodeID, &o.TotalAmount, &o.DiscountAmount, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	items, err := r.getItems(ctx, id)
	if err != nil {
		return nil, err
	}
	o.Items = items
	return o, nil
}

func (r *OrderRepo) getItems(ctx context.Context, orderID uuid.UUID) ([]domain.OrderItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, order_id, product_id, quantity, price_at_order FROM order_items WHERE order_id = $1`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.OrderItem
	for rows.Next() {
		item := domain.OrderItem{}
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.Quantity, &item.PriceAtOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []domain.OrderItem{}
	}
	return items, nil
}

func (r *OrderRepo) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func HasActiveOrder(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (bool, error) {
	var count int
	err := tx.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM orders WHERE user_id=$1 AND status IN ('CREATED','PAYMENT_PENDING')`,
		userID,
	).Scan(&count)
	return count > 0, err
}

func RateLimitCheck(ctx context.Context, tx pgx.Tx, userID uuid.UUID, opType domain.OperationType, limitMinutes int) (bool, error) {
	var count int
	cutoff := time.Now().Add(-time.Duration(limitMinutes) * time.Minute)
	err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_operations WHERE user_id=$1 AND operation_type=$2 AND created_at > $3`,
		userID, string(opType), cutoff,
	).Scan(&count)
	return count > 0, err
}

func LockProduct(ctx context.Context, tx pgx.Tx, productID uuid.UUID) (*domain.Product, error) {
	p := &domain.Product{}
	err := tx.QueryRow(
		ctx,
		`SELECT id, name, description, price, stock, category, status, seller_id, created_at, updated_at
		FROM products WHERE id=$1 FOR UPDATE`,
		productID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock,
		&p.Category, &p.Status, &p.SellerID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return p, err
}

func DecreaseStock(ctx context.Context, tx pgx.Tx, productID uuid.UUID, qty int) error {
	_, err := tx.Exec(ctx,
		`UPDATE products SET stock=stock-$1, updated_at=now() WHERE id=$2`, qty, productID,
	)
	return err
}

func IncreaseStock(ctx context.Context, tx pgx.Tx, productID uuid.UUID, qty int) error {
	_, err := tx.Exec(ctx,
		`UPDATE products SET stock=stock+$1, updated_at=now() WHERE id=$2`, qty, productID,
	)
	return err
}

func LockPromo(ctx context.Context, tx pgx.Tx, code string) (*domain.PromoCode, error) {
	p := &domain.PromoCode{}
	err := tx.QueryRow(ctx,
		`SELECT id, code, discount_type, discount_value, min_order_amount, max_uses, current_uses, valid_from, valid_until, active
		FROM promo_codes WHERE code=$1 FOR UPDATE`, code,
	).Scan(&p.ID, &p.Code, &p.DiscountType, &p.DiscountValue,
		&p.MinOrderAmount, &p.MaxUses, &p.CurrentUses, &p.ValidFrom, &p.ValidUntil, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return p, err
}

func LockPromoByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*domain.PromoCode, error) {
	p := &domain.PromoCode{}
	err := tx.QueryRow(ctx,
		`SELECT id, code, discount_type, discount_value, min_order_amount, max_uses, current_uses, valid_from, valid_until, active
		FROM promo_codes WHERE id=$1 FOR UPDATE`, id,
	).Scan(&p.ID, &p.Code, &p.DiscountType, &p.DiscountValue,
		&p.MinOrderAmount, &p.MaxUses, &p.CurrentUses, &p.ValidFrom, &p.ValidUntil, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return p, err
}

func IncrementPromoUses(ctx context.Context, tx pgx.Tx, promoID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE promo_codes SET current_uses=current_uses+1 WHERE id=$1`, promoID)
	return err
}

func DecrementPromoUses(ctx context.Context, tx pgx.Tx, promoID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE promo_codes SET current_uses=GREATEST(current_uses-1,0) WHERE id=$1`, promoID)
	return err
}

func InsertOrder(ctx context.Context, tx pgx.Tx, o domain.Order) (domain.Order, error) {
	err := tx.QueryRow(ctx,
		`INSERT INTO orders (user_id, status, promo_code_id, total_amount, discount_amount)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at`,
		o.UserID, string(o.Status), o.PromoCodeID, o.TotalAmount, o.DiscountAmount,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

func InsertOrderItems(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, items []domain.OrderItem) error {
	for i := range items {
		err := tx.QueryRow(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, price_at_order) VALUES ($1,$2,$3,$4) RETURNING id`,
			orderID, items[i].ProductID, items[i].Quantity, items[i].PriceAtOrder,
		).Scan(&items[i].ID)
		if err != nil {
			return err
		}
	}
	return nil
}

func InsertUserOp(ctx context.Context, tx pgx.Tx, userID uuid.UUID, opType domain.OperationType) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO user_operations (user_id, operation_type) VALUES ($1, $2)`,
		userID, string(opType),
	)
	return err
}

func GetOrderForUpdate(ctx context.Context, tx pgx.Tx, orderID uuid.UUID) (*domain.Order, error) {
	o := &domain.Order{}
	err := tx.QueryRow(ctx,
		`SELECT id, user_id, status, promo_code_id, total_amount, discount_amount, created_at, updated_at
		FROM orders WHERE id=$1 FOR UPDATE`, orderID,
	).Scan(&o.ID, &o.UserID, &o.Status, &o.PromoCodeID, &o.TotalAmount, &o.DiscountAmount, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return o, err
}

func GetOrderItems(ctx context.Context, tx pgx.Tx, orderID uuid.UUID) ([]domain.OrderItem, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, order_id, product_id, quantity, price_at_order FROM order_items WHERE order_id=$1`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.OrderItem
	for rows.Next() {
		item := domain.OrderItem{}
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.Quantity, &item.PriceAtOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []domain.OrderItem{}
	}
	return items, nil
}

func DeleteOrderItems(ctx context.Context, tx pgx.Tx, orderID uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM order_items WHERE order_id=$1`, orderID)
	return err
}

func UpdateOrderTx(ctx context.Context, tx pgx.Tx, o *domain.Order) error {
	return tx.QueryRow(ctx,
		`UPDATE orders SET status=$1, promo_code_id=$2, total_amount=$3, discount_amount=$4, updated_at=now()
		WHERE id=$5 RETURNING updated_at`,
		string(o.Status), o.PromoCodeID, o.TotalAmount, o.DiscountAmount, o.ID,
	).Scan(&o.UpdatedAt)
}

func CancelOrderTx(ctx context.Context, tx pgx.Tx, o *domain.Order) error {
	o.Status = domain.StatusCanceled
	return tx.QueryRow(ctx,
		`UPDATE orders SET status='CANCELED', updated_at=now() WHERE id=$1 RETURNING updated_at`, o.ID,
	).Scan(&o.UpdatedAt)
}
