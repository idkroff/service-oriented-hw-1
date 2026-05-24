//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCrossServiceTokenValidation — ключевой интеграционный тест:
// проверяет, что marketplace-api валидирует токен через HTTP-вызов user-service.
// Сценарий: SELLER регистрируется через user-service → создаёт product через
// marketplace-api (с Bearer-токеном) → запись появилась в БД marketplace.
func TestCrossServiceTokenValidation(t *testing.T) {
	t.Cleanup(func() { cleanupDatabases(context.Background()) })

	email := "seller-" + uuid.NewString() + "@test"
	token := register(t, email, "secret123", "SELLER")
	require.NotEmpty(t, token)

	body := map[string]any{
		"name":     "Widget",
		"price":    19.99,
		"stock":    100,
		"category": "general",
	}
	resp := doJSON(t, http.MethodPost, marketplaceURL+"/products", token, body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "marketplace должен принять валидный токен")

	var product struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Price    float64 `json:"price"`
		Stock    int     `json:"stock"`
		SellerId string  `json:"seller_id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&product))
	assert.Equal(t, "Widget", product.Name)
	assert.Equal(t, 100, product.Stock)

	var count int
	err := marketplacePool.QueryRow(context.Background(),
		`SELECT count(*) FROM products WHERE name=$1`, "Widget").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "продукт должен сохраниться в БД marketplace")
}

// TestCrossService_InvalidToken — marketplace-api должен отклонить невалидный
// токен (благодаря HTTP-вызову user-service /auth/validate).
func TestCrossService_InvalidToken(t *testing.T) {
	t.Cleanup(func() { cleanupDatabases(context.Background()) })

	body := map[string]any{
		"name":     "Junk",
		"price":    1.0,
		"stock":    1,
		"category": "general",
	}
	resp := doJSON(t, http.MethodPost, marketplaceURL+"/products", "totally-bogus-jwt", body)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestCrossService_MissingToken — без Authorization header marketplace-api
// возвращает 401 на защищённый эндпоинт.
func TestCrossService_MissingToken(t *testing.T) {
	t.Cleanup(func() { cleanupDatabases(context.Background()) })

	body := map[string]any{"name": "X", "price": 1.0, "stock": 1, "category": "x"}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, marketplaceURL+"/products", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestCrossService_RoleBased — buyer (роль USER) не может создавать продукты,
// SELLER — может. Проверяет, что роль из user-service корректно проксируется в
// claims marketplace-api.
func TestCrossService_RoleBased(t *testing.T) {
	t.Cleanup(func() { cleanupDatabases(context.Background()) })

	buyerToken := register(t, "buyer-"+uuid.NewString()+"@test", "secret123", "USER")
	body := map[string]any{"name": "BuyerCannot", "price": 1.0, "stock": 1, "category": "x"}
	resp := doJSON(t, http.MethodPost, marketplaceURL+"/products", buyerToken, body)
	defer resp.Body.Close()

	// Buyer не имеет роли SELLER → 403.
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	raw, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(raw), "ACCESS_DENIED")
}
