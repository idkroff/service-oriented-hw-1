-- +goose Up
CREATE INDEX idx_products_status ON products(status);
CREATE INDEX idx_products_category ON products(category);
CREATE INDEX idx_products_seller_id ON products(seller_id);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_user_ops_user_type_time ON user_operations(user_id, operation_type, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_products_status, idx_products_category, idx_products_seller_id,
    idx_orders_user_id, idx_orders_status, idx_user_ops_user_type_time;
