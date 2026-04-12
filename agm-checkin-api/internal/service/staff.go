package service

import (
	"errors"

	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
)

var (
	ErrStaffNotFound  = errors.New("staff token not found")
	ErrInvalidRole    = errors.New("invalid role; must be 'registration' or 'admin'")
	ErrCannotSelfEdit = errors.New("cannot modify your own token")
)

type StaffService struct {
	db *gorm.DB
}

func NewStaffService(database *gorm.DB) *StaffService {
	return &StaffService{db: database}
}

func (s *StaffService) GetByID(id string) (*db.StaffToken, error) {
	var token db.StaffToken
	if err := s.db.First(&token, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *StaffService) List() ([]db.StaffToken, error) {
	var tokens []db.StaffToken
	if err := s.db.Order("created_at asc").Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

func (s *StaffService) UpdateRole(id, role, requestorID string) (*db.StaffToken, error) {
	if id == requestorID {
		return nil, ErrCannotSelfEdit
	}
	if role != "registration" && role != "admin" {
		return nil, ErrInvalidRole
	}
	var token db.StaffToken
	if err := s.db.First(&token, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStaffNotFound
		}
		return nil, err
	}
	token.Role = role
	if err := s.db.Save(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *StaffService) Revoke(id, requestorID string) error {
	if id == requestorID {
		return ErrCannotSelfEdit
	}
	result := s.db.Delete(&db.StaffToken{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrStaffNotFound
	}
	return nil
}
