package store

import (
	"github.com/risbern21/api_gateway/internal/database"
	"github.com/risbern21/api_gateway/models"
	"gorm.io/gorm"
)

type PGStore struct {
	db *gorm.DB
}

func NewPGStore() *PGStore {
	return &PGStore{
		db: database.Client(),
	}
}

func (p *PGStore) AddUser(u *models.User) error {
	return p.db.Save(&u).Error
}

func (p *PGStore) GetUserByEmail(u *models.User, email string) error {
	return p.db.Table("users").Where("email= ?", email).First(&u).Error
}

func (p *PGStore) CreateSession(s *models.Session) error {
	return p.db.Create(&s).Error
}

func (p *PGStore) GetSession(s *models.Session) error {
	return p.db.First(&s, "id = ?", s.ID).Error
}

func (p *PGStore) RevokeSession(s *models.Session) error {
	return p.db.Table("sessions").Where("id = ?", s.ID).Update("is_revoked", true).Error
}

func (p *PGStore) DeleteSession(s *models.Session) error {
	return p.db.Delete(&s).Error
}
