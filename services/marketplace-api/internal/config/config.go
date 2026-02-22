package config

import (
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL            string
	JWTSecret              string
	AccessTokenTTLMinutes  int
	RefreshTokenTTLDays    int
	OrderRateLimitMinutes  int
	Port                   string
}

func Load() Config {
	return Config{
		DatabaseURL:           getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/marketplace?sslmode=disable"),
		JWTSecret:             getEnv("JWT_SECRET", "dev-secret-key"),
		AccessTokenTTLMinutes: getEnvInt("ACCESS_TOKEN_TTL_MINUTES", 30),
		RefreshTokenTTLDays:   getEnvInt("REFRESH_TOKEN_TTL_DAYS", 7),
		OrderRateLimitMinutes: getEnvInt("ORDER_RATE_LIMIT_MINUTES", 1),
		Port:                  getEnv("PORT", "8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
