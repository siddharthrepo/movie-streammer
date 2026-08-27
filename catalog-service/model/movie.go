package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
)

type Movie struct {
	ID          string `gorm:"type:char(36);primaryKey"`
	Title       string `gorm:"type:varchar(255);not null;index"`
	Description string `gorm:"type:text"`
	DurationMs  *int64 `gorm:"column:duration_ms"`
	Status      string `gorm:"type:varchar(32);not null;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Movie) TableName() string {
	return "movies"
}

func (m *Movie) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.Status == "" {
		m.Status = global.StatusDraft
	}
	return nil
}

func (m *Movie) ToResponse() global.MovieResponse {
	return global.MovieResponse{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		DurationMs:  m.DurationMs,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   m.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
