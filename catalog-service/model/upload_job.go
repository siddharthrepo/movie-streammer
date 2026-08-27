package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
)

type UploadJob struct {
	ID         string  `gorm:"type:char(36);primaryKey"`
	MovieID    string  `gorm:"type:char(36);not null;uniqueIndex"`
	SourceKey  string  `gorm:"type:varchar(512);not null"`
	SourceSize *int64  `gorm:"column:source_size"`
	S3UploadID *string `gorm:"type:varchar(255)"`
	State      string  `gorm:"type:varchar(32);not null;index"`
	PartSize   int64   `gorm:"not null"`
	PartCount  int     `gorm:"not null"`
	ChunkCount int     `gorm:"not null;default:0"`
	Error      *string `gorm:"type:text"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (UploadJob) TableName() string {
	return "upload_jobs"
}

func (j *UploadJob) BeforeCreate(tx *gorm.DB) error {
	if j.ID == "" {
		j.ID = uuid.NewString()
	}
	if j.State == "" {
		j.State = global.JobPendingUpload
	}
	return nil
}

func (j *UploadJob) ToResponse() global.UploadJobResponse {
	resp := global.UploadJobResponse{
		ID:         j.ID,
		MovieID:    j.MovieID,
		State:      j.State,
		SourceKey:  j.SourceKey,
		SourceSize: j.SourceSize,
		PartSize:   j.PartSize,
		PartCount:  j.PartCount,
		CreatedAt:  j.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  j.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if j.Error != nil {
		resp.Error = *j.Error
	}
	return resp
}
