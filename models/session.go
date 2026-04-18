package models

import (
	"time"
)

type Session struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	UserEmail    string    `json:"user_email" gorm:"column:user_email"`
	RefreshToken string    `json:"refresh_token" gorm:"column:refresh_token"`
	IsRevoked    bool      `json:"is_revoked" gorm:"column:is_revoked"`
	ExpiresAt    time.Time `json:"expires_at" gorm:"column:expires_at"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    time.Time
}

func NewSession() *Session {
	return &Session{}
}

// func (s *Session) CreateSession(db *gorm.DB) error {
// 	return db.Create(&s).Error
// }
//
// func (s *Session) GetSession(db *gorm.DB) error {
// 	return db.First(&s, "id = ?", s.ID).Error
// }
//
// func (s *Session) RevokeSession(db *gorm.DB) error {
// 	return db.Table("sessions").Where("id = ?", s.ID).Update("is_revoked", true).Error
// }
//
// func (s *Session) DeleteSession(db *gorm.DB) error {
// 	return db.Delete(&s).Error
// }
