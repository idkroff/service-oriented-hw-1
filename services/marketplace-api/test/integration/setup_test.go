//go:build integration

// Package integration содержит интеграционные тесты, требующие живого
// docker compose (marketplace-api + user-service + postgres).
//
// Запуск:
//
//	docker compose -f docker-compose.yml -f docker-compose.test.yml up -d --wait
//	go test -tags=integration ./test/integration/...
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultUserURL        = "http://localhost:8000"
	defaultMarketplaceURL = "http://localhost:8080"
	defaultMarketplaceDSN = "postgres://postgres:postgres@localhost:5432/marketplace?sslmode=disable"
	defaultUserDSN        = "postgres://postgres:postgres@localhost:5432/user_service?sslmode=disable"
)

// env возвращает переменную окружения или значение по умолчанию.
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var (
	userURL        string
	marketplaceURL string
	marketplaceDSN string
	userDSN        string
)

// marketplacePool и userPool — независимые пулы для прямых SQL-проверок и cleanup.
var (
	marketplacePool *pgxpool.Pool
	userPool        *pgxpool.Pool
)

func TestMain(m *testing.M) {
	userURL = env("INTEGRATION_USER_URL", defaultUserURL)
	marketplaceURL = env("INTEGRATION_MARKETPLACE_URL", defaultMarketplaceURL)
	marketplaceDSN = env("INTEGRATION_MARKETPLACE_DSN", defaultMarketplaceDSN)
	userDSN = env("INTEGRATION_USER_DSN", defaultUserDSN)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := waitHealthy(ctx, userURL+"/health"); err != nil {
		log.Fatalf("user-service не отвечает: %v\nЗапусти docker compose up -d --wait перед тестами.", err)
	}
	if err := waitHealthy(ctx, marketplaceURL+"/health"); err != nil {
		log.Fatalf("marketplace-api не отвечает: %v", err)
	}

	var err error
	marketplacePool, err = pgxpool.New(ctx, marketplaceDSN)
	if err != nil {
		log.Fatalf("connect marketplace db: %v", err)
	}
	userPool, err = pgxpool.New(ctx, userDSN)
	if err != nil {
		log.Fatalf("connect user db: %v", err)
	}

	cleanupDatabases(ctx)

	code := m.Run()

	marketplacePool.Close()
	userPool.Close()
	os.Exit(code)
}

func waitHealthy(ctx context.Context, url string) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(60 * time.Second)
	}
	for {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("health check timed out: %s", url)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// cleanupDatabases полностью очищает изменяемые таблицы — обеспечивает
// независимость тестов.
func cleanupDatabases(ctx context.Context) {
	// Очерёдность важна из-за FK.
	for _, q := range []string{
		"TRUNCATE TABLE order_items, orders, products, promo_codes, user_operations RESTART IDENTITY CASCADE",
	} {
		if _, err := marketplacePool.Exec(ctx, q); err != nil {
			log.Fatalf("marketplace cleanup %q: %v", q, err)
		}
	}
	for _, q := range []string{
		"TRUNCATE TABLE refresh_tokens, users RESTART IDENTITY CASCADE",
	} {
		if _, err := userPool.Exec(ctx, q); err != nil {
			log.Fatalf("user cleanup %q: %v", q, err)
		}
	}
}

// register — utility для тестов: создаёт пользователя в user-service.
func register(t *testing.T, email, password, role string) (accessToken string) {
	t.Helper()
	body := map[string]string{"email": email, "password": password, "role": role}
	raw, _ := json.Marshal(body)
	resp, err := http.Post(userURL+"/auth/register", "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("register %s: %v", email, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("register %s status=%d body=%s", email, resp.StatusCode, string(raw))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("register decode: %v", err)
	}
	return out.AccessToken
}

// doJSON — utility для тестов: POST/PUT JSON с авторизацией.
func doJSON(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do req: %v", err)
	}
	return resp
}
