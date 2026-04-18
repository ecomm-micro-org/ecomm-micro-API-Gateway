package handler

import (
	"time"

	"github.com/google/uuid"
	"github.com/risbern21/api_gateway/models"
)

type SigninRequest struct {
	Username string `json:"username" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
	Role     string `json:"role" validate:"required"`
	Phone    string `json:"phone"  validate:"required"`
	Address  string `json:"address"  validate:"required"`
}

type SigninResponse struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	Phone    string    `json:"phone"`
	Address  string    `json:"address"`
}

type LoginReq struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type LoginRes struct {
	SessionID             string      `json:"session_id"`
	AccessToken           string      `json:"access_token"`
	AccessTokenExpiresAt  time.Time   `json:"access_token_expires_at"`
	RefreshToken          string      `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time   `json:"refresh_token_expires_at"`
	User                  models.User `json:"user"`
}

type RenewAccessTokenReq struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type RenewAccessTokenRes struct {
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
}
