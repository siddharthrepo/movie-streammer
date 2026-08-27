package service

import (
	"context"
	"errors"
	"strings"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/model"
	"github.com/siddharthraturi/movie-streamer/catalog-service/repository"
)

var (
	ErrNotFound = errors.New("movie not found")
	ErrInvalid  = errors.New("invalid request")
)

type MovieService interface {
	Create(ctx context.Context, req global.CreateMovieRequest) (*model.Movie, error)
	Get(ctx context.Context, id string) (*model.Movie, error)
	List(ctx context.Context, page, pageSize int) ([]model.Movie, int64, error)
	Update(ctx context.Context, id string, req global.UpdateMovieRequest) (*model.Movie, error)
	Delete(ctx context.Context, id string) error
}

type movieService struct {
	repo repository.MovieRepository
}

func NewMovieService(repo repository.MovieRepository) MovieService {
	return &movieService{repo: repo}
}

func (s *movieService) Create(ctx context.Context, req global.CreateMovieRequest) (*model.Movie, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, ErrInvalid
	}

	m := &model.Movie{
		Title:       title,
		Description: strings.TrimSpace(req.Description),
		Status:      global.StatusDraft,
	}

	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *movieService) Get(ctx context.Context, id string) (*model.Movie, error) {
	m, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrNotFound
	}
	return m, err
}

func (s *movieService) List(ctx context.Context, page, pageSize int) ([]model.Movie, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = global.DefaultPageSize
	}
	if pageSize > global.MaxPageSize {
		pageSize = global.MaxPageSize
	}
	return s.repo.List(ctx, (page-1)*pageSize, pageSize)
}

func (s *movieService) Update(ctx context.Context, id string, req global.UpdateMovieRequest) (*model.Movie, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return nil, ErrInvalid
		}
		m.Title = title
	}
	if req.Description != nil {
		m.Description = strings.TrimSpace(*req.Description)
	}

	if err := s.repo.Update(ctx, m); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

func (s *movieService) Delete(ctx context.Context, id string) error {
	err := s.repo.Delete(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
