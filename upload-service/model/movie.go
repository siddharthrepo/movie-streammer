package model

import "time"

const (
	StatusPendingUpload = "pending_upload"
	StatusUploaded      = "uploaded"
	StatusAborted       = "aborted"
)

type Movie struct {
	ID          string `gorm:"type:uuid;default:gen_random_uuid()"`
	Filename    string
	ObjectKey   string
	SizeBytes   int64
	ContentType string
	Status      string
	UploadID    string
	PartSize    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Movie) TableName() string { return "movies" }
