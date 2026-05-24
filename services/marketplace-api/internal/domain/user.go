package domain

import "github.com/google/uuid"

// Role — роль пользователя, проксируется из user-service в JWT-claims.
type Role string

const (
	RoleUser   Role = "USER"
	RoleSeller Role = "SELLER"
	RoleAdmin  Role = "ADMIN"
)

// UserClaims — то, что user-service возвращает по /auth/validate.
type UserClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   Role      `json:"role"`
}
