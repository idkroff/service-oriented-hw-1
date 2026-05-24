//go:build e2e

// Package e2e — сквозной пользовательский сценарий: SELLER создаёт товар,
// USER оформляет заказ, проверяем статус через API и состояние в БД.
//
// Запуск:
//
//	docker compose -f docker-compose.yml -f docker-compose.test.yml up -d --wait
//	go test -tags=e2e ./test/e2e/...
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultUserURL        = "http://localhost:8000"
	defaultMarketplaceURL = "http://localhost:8080"
	defaultMarketplaceDSN = "postgres://postgres:postgres@localhost:5432/marketplace?sslmode=disable"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func waitHealthy(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("health timeout: %s", url)
}

func registerUser(t *testing.T, baseURL, email, password, role string) string {
	t.Helper()
	body := map[string]string{"email": email, "password": password, "role": role}
	raw, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/auth/register", "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("register status=%d body=%s", resp.StatusCode, string(raw))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out.AccessToken
}

func doJSON(t *testing.T, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, raw
}

func TestE2E_FullCheckoutScenario(t *testing.T) {
	userURL := env("E2E_USER_URL", defaultUserURL)
	marketplaceURL := env("E2E_MARKETPLACE_URL", defaultMarketplaceURL)
	marketplaceDSN := env("E2E_MARKETPLACE_DSN", defaultMarketplaceDSN)

	waitHealthy(t, userURL+"/health")
	waitHealthy(t, marketplaceURL+"/health")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, marketplaceDSN)
	require.NoError(t, err)
	defer pool.Close()

	// 1. Регистрируем SELLER + USER.
	suffix := uuid.NewString()
	sellerEmail := "seller-" + suffix + "@e2e"
	userEmail := "user-" + suffix + "@e2e"
	sellerToken := registerUser(t, userURL, sellerEmail, "secret123", "SELLER")
	userToken := registerUser(t, userURL, userEmail, "secret123", "USER")

	// 2. SELLER создаёт product (stock=10, price=100).
	createBody := map[string]any{
		"name":     "E2E Widget " + suffix,
		"price":    100.0,
		"stock":    10,
		"category": "general",
	}
	resp, raw := doJSON(t, http.MethodPost, marketplaceURL+"/products", sellerToken, createBody)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "create product: %s", string(raw))

	var product struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Price    float64 `json:"price"`
		Stock    int     `json:"stock"`
		SellerID string  `json:"seller_id"`
		Status   string  `json:"status"`
	}
	require.NoError(t, json.Unmarshal(raw, &product))
	assert.Equal(t, 10, product.Stock)
	assert.InDelta(t, 100.0, product.Price, 0.001)
	assert.Equal(t, "ACTIVE", product.Status)

	// 3. USER создаёт заказ на 3 шт.
	orderBody := map[string]any{
		"items": []map[string]any{
			{"product_id": product.ID, "quantity": 3},
		},
	}
	resp, raw = doJSON(t, http.MethodPost, marketplaceURL+"/orders", userToken, orderBody)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "create order: %s", string(raw))

	var order struct {
		ID          string  `json:"id"`
		UserID      string  `json:"user_id"`
		Status      string  `json:"status"`
		TotalAmount float64 `json:"total_amount"`
		Items       []struct {
			ProductID    string  `json:"product_id"`
			Quantity     int     `json:"quantity"`
			PriceAtOrder float64 `json:"price_at_order"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(raw, &order))
	assert.Equal(t, "CREATED", order.Status)
	assert.InDelta(t, 300.0, order.TotalAmount, 0.001)
	require.Len(t, order.Items, 1)
	assert.Equal(t, 3, order.Items[0].Quantity)

	// 4. GET order/{id} — статус и поля валидны.
	resp, raw = doJSON(t, http.MethodGet, marketplaceURL+"/orders/"+order.ID, userToken, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "get order: %s", string(raw))
	var fetched struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(raw, &fetched))
	assert.Equal(t, order.ID, fetched.ID)
	assert.Equal(t, "CREATED", fetched.Status)

	// 5. SQL-проверка состояния в БД.
	var stock int
	err = pool.QueryRow(ctx, `SELECT stock FROM products WHERE id=$1`, product.ID).Scan(&stock)
	require.NoError(t, err)
	assert.Equal(t, 7, stock, "stock должен уменьшиться на 3 (10 - 3)")

	var ordersCount, itemsCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM orders WHERE id=$1`, order.ID).Scan(&ordersCount)
	require.NoError(t, err)
	assert.Equal(t, 1, ordersCount)

	err = pool.QueryRow(ctx, `SELECT count(*) FROM order_items WHERE order_id=$1`, order.ID).Scan(&itemsCount)
	require.NoError(t, err)
	assert.Equal(t, 1, itemsCount)

	// 6. Cleanup БД, чтобы прогон не оставлял мусор для следующих тестов.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `TRUNCATE TABLE order_items, orders, products, promo_codes, user_operations RESTART IDENTITY CASCADE`)
		// users живут в user-service DB, см. README.
		fmt.Fprintln(io.Discard, "cleanup done")
	})
}
