package service

import (
	"time"

	"johndpete316/agm-checkin-api/internal/db"

	"gorm.io/gorm"
)

type CompetitorService struct {
	db *gorm.DB
}

func NewCompetitorService(database *gorm.DB) *CompetitorService {
	return &CompetitorService{db: database}
}

func (s *CompetitorService) GetAll(search string) ([]db.Competitor, error) {
	var competitors []db.Competitor
	query := s.db
	if search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}
	result := query.Find(&competitors)
	return competitors, result.Error
}

func (s *CompetitorService) GetByID(id string) (*db.Competitor, error) {
	var competitor db.Competitor
	result := s.db.First(&competitor, "id = ?", id)
	return &competitor, result.Error
}

func (s *CompetitorService) Create(competitor *db.Competitor) error {
	return s.db.Create(competitor).Error
}

func (s *CompetitorService) CheckIn(id string) (*db.Competitor, error) {
	var competitor db.Competitor
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&competitor, "id = ?", id).Error; err != nil {
			return err
		}
		now := time.Now()
		competitor.IsCheckedIn = true
		competitor.CheckInDateTime = &now
		return tx.Save(&competitor).Error
	})
	return &competitor, err
}

func (s *CompetitorService) Delete(id string) error {
	return s.db.Delete(&db.Competitor{}, "id = ?", id).Error
}
