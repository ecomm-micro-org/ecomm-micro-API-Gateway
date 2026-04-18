// Package handler provides HandlerFunc
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/risbern21/api_gateway/internal/token"
	"github.com/risbern21/api_gateway/internal/validate"
	"github.com/risbern21/api_gateway/models"
	"github.com/risbern21/api_gateway/store"
	"github.com/risbern21/api_gateway/util"
	"gorm.io/gorm"
)

type Storer interface {
	AddUser(u *models.User) error
	GetUserByEmail(u *models.User, email string) error
	CreateSession(s *models.Session) error
	GetSession(s *models.Session) error
	RevokeSession(s *models.Session) error
	DeleteSession(s *models.Session) error
}

type Handler struct {
	tokenMaker *token.JWTMaker
	s          Storer
}

func NewHandler(secretKey string) *Handler {
	return &Handler{
		tokenMaker: token.NewJWTMaker(secretKey),
		s:          store.NewPGStore(),
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	u := &SigninRequest{}
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "error creating user", http.StatusBadRequest)
		return
	}

	validator := validate.New()
	if err := validator.Validator.Struct(u); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	password, err := util.HashPassword(u.Password)
	if err != nil {
		http.Error(w, "error creating user", http.StatusInternalServerError)
		return
	}

	m := models.NewUser()
	m.Username = u.Username
	m.Email = u.Email
	m.Password = password
	m.Role = models.Role(u.Role)
	m.Phone = u.Phone
	m.Address = u.Address

	if err := h.s.AddUser(m); err != nil {
		http.Error(w, "error creating user", http.StatusInternalServerError)
		return
	}

	res := &SigninResponse{}
	res.ID = m.ID
	res.Username = u.Username
	res.Email = m.Email
	res.Address = m.Address
	res.Role = string(m.Role)
	res.Phone = m.Phone

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(&res)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	req := &LoginReq{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "error invalid request body", http.StatusBadRequest)
		return
	}

	if err := validate.New().Validator.Struct(req); err != nil {
		http.Error(w, "error invalid request body", http.StatusBadRequest)
		return
	}

	u := models.NewUser()
	err := h.s.GetUserByEmail(u, req.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "error user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "error unable to login", http.StatusInternalServerError)
		return
	}

	if err := util.CheckPassword(req.Password, u.Password); err != nil {
		http.Error(w, "error invalid password", http.StatusBadRequest)
		return
	}

	//generate JWT token and return
	accessToken, accessTokenClaims, err := h.tokenMaker.CreateToken(u.ID, u.Email, u.Role.String(), 15*time.Minute)
	if err != nil {
		http.Error(w, "error creating token", http.StatusInternalServerError)
		return
	}

	refreshToken, refreshTokenClaims, err := h.tokenMaker.CreateToken(u.ID, u.Email, u.Role.String(), 24*time.Hour)
	if err != nil {
		http.Error(w, "error creating token", http.StatusInternalServerError)
		return
	}

	s := models.NewSession()
	s.ID = refreshTokenClaims.RegisteredClaims.ID
	s.UserEmail = u.Email
	s.RefreshToken = refreshToken
	s.IsRevoked = false
	s.ExpiresAt = refreshTokenClaims.ExpiresAt.Time

	if err := h.s.CreateSession(s); err != nil {
		http.Error(w, "error unable to create session", http.StatusInternalServerError)
		return
	}

	res := &LoginRes{
		SessionID:             s.ID,
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessTokenClaims.ExpiresAt.Time,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: refreshTokenClaims.ExpiresAt.Time,
		User:                  *u,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	id := query["id"][0]
	if id == "" {
		http.Error(w, "error no session id", http.StatusBadRequest)
		return
	}

	s := models.NewSession()
	s.ID = id

	if err := h.s.GetSession(s); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "error session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "unable to get session", http.StatusInternalServerError)
		return
	}

	if err := h.s.DeleteSession(s); err != nil {
		http.Error(w, "unable to delete session", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RenewAccessToken(w http.ResponseWriter, r *http.Request) {
	req := &RenewAccessTokenReq{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "error invalid request body", http.StatusBadRequest)
		return
	}

	if err := validate.New().Validator.Struct(req); err != nil {
		http.Error(w, "error invalid request body", http.StatusBadRequest)
		return
	}

	refreshTokenClaims, err := h.tokenMaker.VerifyToken(req.RefreshToken)
	if err != nil {
		http.Error(w, "error unable to verifying token", http.StatusUnauthorized)
		return
	}

	s := models.NewSession()
	s.ID = refreshTokenClaims.RegisteredClaims.ID
	if err := h.s.GetSession(s); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "error session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "error unable to fetch session", http.StatusInternalServerError)
		return
	}

	if s.IsRevoked {
		http.Error(w, "session is revoked", http.StatusUnauthorized)
		return
	}

	if s.UserEmail != refreshTokenClaims.Email {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	accessToken, accessTokenClaims, err := h.tokenMaker.CreateToken(refreshTokenClaims.ID, refreshTokenClaims.Email, refreshTokenClaims.Role, 15*time.Minute)
	if err != nil {
		http.Error(w, "error creating access token", http.StatusInternalServerError)
		return
	}

	res := &RenewAccessTokenRes{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessTokenClaims.ExpiresAt.Time,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	id := query["id"][0]
	if id == "" {
		http.Error(w, "error no session id", http.StatusBadRequest)
		return
	}

	s := models.NewSession()
	s.ID = id
	if err := h.s.GetSession(s); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "error unable to find session", http.StatusNotFound)
			return
		}
		http.Error(w, "error unable to revoke session", http.StatusInternalServerError)
		return
	}

	if err := h.s.RevokeSession(s); err != nil {
		http.Error(w, "unable to revoke session", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
