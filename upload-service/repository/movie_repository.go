package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/upload-service/global"
)

var ErrNotFound = errors.New("movie not found")

type MovieRepository interface {
	Create(ctx context.Context, m *global.Movie) error
	GetByID(ctx context.Context, id string) (*global.Movie, error)
	UpdateStatus(ctx context.Context, id, status string) error
}

type gormMovieRepository struct {
	db *gorm.DB
}

func NewMovieRepository(db *gorm.DB) MovieRepository {
	return &gormMovieRepository{db: db}
}

func (r *gormMovieRepository) Create(ctx context.Context, m *global.Movie) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *gormMovieRepository) GetByID(ctx context.Context, id string) (*global.Movie, error) {
	var m global.Movie
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *gormMovieRepository) UpdateStatus(ctx context.Context, id, status string) error {
	res := r.db.WithContext(ctx).Model(&global.Movie{}).Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
