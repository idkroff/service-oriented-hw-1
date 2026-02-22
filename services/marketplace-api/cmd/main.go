package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	appdb "marketplace-api/internal/db"
	"marketplace-api/internal/config"
	"marketplace-api/internal/generated"
	"marketplace-api/internal/handlers"
	"marketplace-api/internal/middleware"
	"marketplace-api/internal/repository"
	"marketplace-api/internal/usecases"
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
	productRepo := repository.NewProductRepo(pool)
	orderRepo := repository.NewOrderRepo(pool)
	promoRepo := repository.NewPromoRepo(pool)

	authUC := usecases.NewAuthUseCase(userRepo, rtRepo, cfg.JWTSecret, cfg.AccessTokenTTLMinutes, cfg.RefreshTokenTTLDays)
	productUC := usecases.NewProductUseCase(productRepo)
	orderUC := usecases.NewOrderUseCase(orderRepo, cfg.OrderRateLimitMinutes)
	promoUC := usecases.NewPromoUseCase(promoRepo)

	h := handlers.New(authUC, productUC, orderUC, promoUC)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	swagger, err := generated.GetSwagger()
	if err != nil {
		log.Fatalf("swagger: %v", err)
	}
	validationMiddleware := nethttpmiddleware.OapiRequestValidatorWithOptions(swagger, &nethttpmiddleware.Options{
		ErrorHandler: func(w http.ResponseWriter, message string, statusCode int) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error_code": "VALIDATION_ERROR",
				"message":    message,
			})
		},
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
	})

	strictHandler := generated.NewStrictHandler(h, nil)
	wrapper := &generated.ServerInterfaceWrapper{
		Handler: strictHandler,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error_code": "VALIDATION_ERROR",
				"message":    err.Error(),
			})
		},
	}

	r := chi.NewRouter()
	r.Use(middleware.Logging(logger))

	// /health не в OpenAPI-спеке, регистрируем до группы с валидацией
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "marketplace-api"})
	})

	r.Group(func(r chi.Router) {
		r.Use(validationMiddleware)

		// no auth required
		r.Post("/auth/login", wrapper.Login)
		r.Post("/auth/refresh", wrapper.RefreshToken)
		r.Post("/auth/register", wrapper.Register)

		// optional auth — personalization without enforcement
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthOptional(authUC))
			r.Get("/products", wrapper.ListProducts)
			r.Get("/products/{id}", wrapper.GetProduct)
		})

		// auth required
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(authUC))
			r.Post("/products", wrapper.CreateProduct)
			r.Put("/products/{id}", wrapper.UpdateProduct)
			r.Delete("/products/{id}", wrapper.DeleteProduct)
			r.Post("/orders", wrapper.CreateOrder)
			r.Get("/orders/{id}", wrapper.GetOrder)
			r.Put("/orders/{id}", wrapper.UpdateOrder)
			r.Post("/orders/{id}/cancel", wrapper.CancelOrder)
			r.Post("/promo-codes", wrapper.CreatePromoCode)
		})
	})

	addr := ":" + cfg.Port
	log.Printf("marketplace-api listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}
