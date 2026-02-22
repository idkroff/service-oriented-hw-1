package handlers

import (
	"context"

	"marketplace-api/internal/domain"
	"marketplace-api/internal/generated"
)

func (h *Handler) Register(ctx context.Context, req generated.RegisterRequestObject) (generated.RegisterResponseObject, error) {
	role := domain.RoleUser
	if req.Body.Role != nil {
		role = domain.Role(*req.Body.Role)
	}

	tokens, err := h.authUC.Register(ctx, string(req.Body.Email), req.Body.Password, role)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrEmailTaken:
			return generated.Register409JSONResponse(errResp(code, message)), nil
		default:
			return generated.Register400JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		}
	}

	return generated.Register201JSONResponse(generated.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    "Bearer",
	}), nil
}

func (h *Handler) Login(ctx context.Context, req generated.LoginRequestObject) (generated.LoginResponseObject, error) {
	tokens, err := h.authUC.Login(ctx, string(req.Body.Email), req.Body.Password)
	if err != nil {
		code, message := mapDomainError(err)
		switch err {
		case domain.ErrInvalidCredentials:
			return generated.Login401JSONResponse(errResp(code, message)), nil
		default:
			return generated.Login400JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
		}
	}

	return generated.Login200JSONResponse(generated.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    "Bearer",
	}), nil
}

func (h *Handler) RefreshToken(ctx context.Context, req generated.RefreshTokenRequestObject) (generated.RefreshTokenResponseObject, error) {
	tokens, err := h.authUC.Refresh(ctx, req.Body.RefreshToken)
	if err != nil {
		code, message := mapDomainError(err)
		return generated.RefreshToken401JSONResponse{ErrorJSONResponse: generated.ErrorJSONResponse(errResp(code, message))}, nil
	}

	return generated.RefreshToken200JSONResponse(generated.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    "Bearer",
	}), nil
}
