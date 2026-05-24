package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"user-service/internal/domain"
	"user-service/internal/usecases"
)

type Handler struct {
	authUC *usecases.AuthUseCase
}

func New(authUC *usecases.AuthUseCase) *Handler {
	return &Handler{authUC: authUC}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "email and password required")
		return
	}
	if len(req.Password) < 6 || len(req.Password) > 72 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "password length must be 6..72")
		return
	}

	role := domain.RoleUser
	if req.Role != "" {
		role = domain.Role(req.Role)
	}

	tokens, err := h.authUC.Register(r.Context(), req.Email, req.Password, role)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrEmailTaken):
			writeError(w, http.StatusConflict, "EMAIL_TAKEN", "email already registered")
		case errors.Is(err, domain.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid input")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    "Bearer",
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	tokens, err := h.authUC.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    "Bearer",
	})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	tokens, err := h.authUC.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrTokenExpired):
			writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "refresh token expired")
		default:
			writeError(w, http.StatusUnauthorized, "TOKEN_INVALID", "invalid refresh token")
		}
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    "Bearer",
	})
}

// Validate проверяет access-токен и возвращает claims.
// Используется только marketplace-api при авторизации запросов.
func (h *Handler) Validate(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "TOKEN_MISSING", "authorization header missing")
		return
	}
	token := strings.TrimPrefix(header, "Bearer ")
	claims, err := h.authUC.VerifyAccessToken(token)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrTokenExpired):
			writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "access token expired")
		default:
			writeError(w, http.StatusUnauthorized, "TOKEN_INVALID", "invalid access token")
		}
		return
	}
	writeJSON(w, http.StatusOK, claims)
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "user-service",
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{
		"error_code": code,
		"message":    message,
	})
}
