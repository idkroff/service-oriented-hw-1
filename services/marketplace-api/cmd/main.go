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
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"marketplace-api/internal/clients"
	"marketplace-api/internal/config"
	appdb "marketplace-api/internal/db"
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

	productRepo := repository.NewProductRepo(pool)
	orderRepo := repository.NewOrderRepo(pool)
	promoRepo := repository.NewPromoRepo(pool)

	productUC := usecases.NewProductUseCase(productRepo)
	orderUC := usecases.NewOrderUseCase(orderRepo, cfg.OrderRateLimitMinutes)
	promoUC := usecases.NewPromoUseCase(promoRepo)

	userClient := clients.NewUserClient(cfg.UserServiceURL)

	h := handlers.New(productUC, orderUC, promoUC)

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
	r.Use(middleware.Metrics())

	// /metrics и /health регистрируем ДО группы с OapiRequestValidator —
	// они вне OpenAPI-спеки и не должны проходить валидацию (иначе 4xx).
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "marketplace-api"})
	})

	r.Group(func(r chi.Router) {
		r.Use(validationMiddleware)

		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthOptional(userClient))
			r.Get("/products", wrapper.ListProducts)
			r.Get("/products/{id}", wrapper.GetProduct)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(userClient))
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
