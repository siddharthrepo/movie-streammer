package global

import "time"

type CreateMovieRequest struct {
	Title       string `json:"title" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=5000"`
}

type UpdateMovieRequest struct {
	Title       *string `json:"title" binding:"omitempty,min=1,max=255"`
	Description *string `json:"description" binding:"omitempty,max=5000"`
}

type MovieResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DurationMs  *int64 `json:"duration_ms"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ListMoviesResponse struct {
	Items    []MovieResponse `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int64           `json:"total"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

type Part struct {
	PartNumber int32  `json:"part_number" binding:"required,min=1"`
	ETag       string `json:"etag" binding:"required"`
}

type PresignedPart struct {
	PartNumber int32  `json:"part_number"`
	URL        string `json:"url"`
}

type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"last_modified"`
}
