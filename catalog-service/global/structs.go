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

type InitiateUploadRequest struct {
	Title       string `json:"title" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=5000"`
	FileName    string `json:"file_name" binding:"required,min=1,max=255"`
	FileSize    int64  `json:"file_size" binding:"required,min=1"`
}

type InitiateUploadResponse struct {
	UploadID  string          `json:"upload_id"`
	MovieID   string          `json:"movie_id"`
	SourceKey string          `json:"source_key"`
	PartSize  int64           `json:"part_size"`
	PartCount int             `json:"part_count"`
	ExpiresAt string          `json:"expires_at"`
	Parts     []PresignedPart `json:"parts"`
}

type CompleteUploadRequest struct {
	Parts []Part `json:"parts" binding:"required,min=1,dive"`
}

type UploadJobResponse struct {
	ID         string `json:"id"`
	MovieID    string `json:"movie_id"`
	State      string `json:"state"`
	SourceKey  string `json:"source_key"`
	SourceSize *int64 `json:"source_size"`
	PartSize   int64  `json:"part_size"`
	PartCount  int    `json:"part_count"`
	Error      string `json:"error,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type Rendition struct {
	Name         string `json:"name"`
	Height       int    `json:"height"`
	VideoBitrate int    `json:"video_bitrate"`
	AudioBitrate int    `json:"audio_bitrate"`
}

type MediaInfo struct {
	DurationMs int64  `json:"duration_ms"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec"`
	Bitrate    int64  `json:"bitrate"`
	HasAudio   bool   `json:"has_audio"`
}

type PlanResponse struct {
	JobID      string   `json:"job_id"`
	MovieID    string   `json:"movie_id"`
	DurationMs int64    `json:"duration_ms"`
	ChunkCount int      `json:"chunk_count"`
	Qualities  []string `json:"qualities"`
	TotalItems int      `json:"total_items"`
}

type FFStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Duration  string `json:"duration"`
}

type FFFormat struct {
	Duration string `json:"duration"`
	BitRate  string `json:"bit_rate"`
}

type FFOutput struct {
	Streams []FFStream `json:"streams"`
	Format  FFFormat   `json:"format"`
}

type ChunkCounts struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Leased  int `json:"leased"`
	Done    int `json:"done"`
	Failed  int `json:"failed"`
}
