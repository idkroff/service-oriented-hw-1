package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"user-service/internal/config"
	appdb "user-service/internal/db"
	"user-service/internal/handlers"
	"user-service/internal/middleware"
	"user-service/internal/repository"
	"user-service/internal/usecases"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	pool, err := appdb.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	userRepo := repository.NewUserRepo(pool)
	rtRepo := repository.NewRefreshTokenRepo(pool)

	authUC := usecases.NewAuthUseCase(userRepo, rtRepo, cfg.JWTSecret, cfg.AccessTokenTTLMinutes, cfg.RefreshTokenTTLDays)
	h := handlers.New(authUC)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	r := chi.NewRouter()
	r.Use(middleware.Logging(logger))
	r.Use(middleware.Metrics())

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)

	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/refresh", h.Refresh)
	r.Post("/auth/validate", h.Validate)

	addr := ":" + cfg.Port
	log.Printf("user-service listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}
