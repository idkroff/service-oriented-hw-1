package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"marketplace-api/internal/domain"
)

type ProductRepo struct {
	pool *pgxpool.Pool
}

func NewProductRepo(pool *pgxpool.Pool) *ProductRepo {
	return &ProductRepo{pool: pool}
}

func scanProduct(row pgx.Row) (*domain.Product, error) {
	p := &domain.Product{}
	err := row.Scan(
		&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock,
		&p.Category, &p.Status, &p.SellerID, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return p, err
}

const productColumns = `id, name, description, price, stock, category, status, seller_id, created_at, updated_at`

func (r *ProductRepo) Create(ctx context.Context, p *domain.Product) (*domain.Product, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO products (name, description, price, stock, category, status, seller_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+productColumns,
		p.Name, p.Description, p.Price, p.Stock, p.Category, string(p.Status), p.SellerID,
	)
	return scanProduct(row)
}

func (r *ProductRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+productColumns+` FROM products WHERE id = $1`, id,
	)
	return scanProduct(row)
}

func (r *ProductRepo) Update(ctx context.Context, p *domain.Product) (*domain.Product, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE products
		SET name=$1, description=$2, price=$3, stock=$4, category=$5, status=$6, updated_at=now()
		WHERE id=$7
		RETURNING `+productColumns,
		p.Name, p.Description, p.Price, p.Stock, p.Category, string(p.Status), p.ID,
	)
	return scanProduct(row)
}

func (r *ProductRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE products SET status='ARCHIVED', updated_at=now() WHERE id=$1`, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ProductRepo) List(ctx context.Context, f domain.ProductFilter) (*domain.ProductList, error) {
	where := []string{}
	args := []any{}
	idx := 1

	if f.Category != nil {
		where = append(where, fmt.Sprintf("category = $%d", idx))
		args = append(args, *f.Category)
		idx++
	}
	if f.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, string(*f.Status))
		idx++
	}
	if f.SellerID != nil {
		where = append(where, fmt.Sprintf("seller_id = $%d", idx))
		args = append(args, *f.SellerID)
		idx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	err := r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM products %s`, whereClause),
		args...,
	).Scan(&total)
	if err != nil {
		return nil, err
	}

	offset := f.Page * f.Size
	args = append(args, f.Size, offset)
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM products %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
			productColumns, whereClause, idx, idx+1),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.Product
	for rows.Next() {
		p := domain.Product{}
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock,
			&p.Category, &p.Status, &p.SellerID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	if items == nil {
		items = []domain.Product{}
	}
	return &domain.ProductList{Items: items, TotalElements: total}, nil
}
