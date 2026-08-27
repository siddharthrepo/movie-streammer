package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/catalog-service/model"
)

var ErrNotFound = errors.New("not found")

type MovieRepository interface {
	Create(ctx context.Context, m *model.Movie) error
	GetByID(ctx context.Context, id string) (*model.Movie, error)
	List(ctx context.Context, offset, limit int) ([]model.Movie, int64, error)
	Update(ctx context.Context, m *model.Movie) error
	Delete(ctx context.Context, id string) error
}

type movieRepository struct {
	db *gorm.DB
}

func NewMovieRepository(db *gorm.DB) MovieRepository {
	return &movieRepository{db: db}
}

func (r *movieRepository) Create(ctx context.Context, m *model.Movie) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("create movie: %w", err)
	}
	return nil
}

func (r *movieRepository) GetByID(ctx context.Context, id string) (*model.Movie, error) {
	var m model.Movie
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get movie: %w", err)
	}
	return &m, nil
}

func (r *movieRepository) List(ctx context.Context, offset, limit int) ([]model.Movie, int64, error) {
	var (
		items []model.Movie
		total int64
	)

	q := r.db.WithContext(ctx).Model(&model.Movie{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count movies: %w", err)
	}

	err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list movies: %w", err)
	}

	return items, total, nil
}

func (r *movieRepository) Update(ctx context.Context, m *model.Movie) error {
	res := r.db.WithContext(ctx).Model(&model.Movie{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"title":       m.Title,
			"description": m.Description,
		})
	if res.Error != nil {
		return fmt.Errorf("update movie: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *movieRepository) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&model.Movie{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("delete movie: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
