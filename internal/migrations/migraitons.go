package migrations

import (
	"log"

	"github.com/risbern21/api_gateway/internal/database"
	"github.com/risbern21/api_gateway/models"
)

func AutoMigrate() {
	if err := database.Client().AutoMigrate(&models.User{}, &models.Session{}); err != nil {
		log.Fatalf("unable to migrate %v", err)
	}
}
