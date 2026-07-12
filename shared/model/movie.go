package model

import "time"

const (
	StatusPendingUpload = "pending_upload"
	StatusUploaded      = "uploaded"
	StatusProcessing    = "processing"
	StatusReady         = "ready"
)

type Movie struct {
	ID           string `gorm:"type:uuid;default:gen_random_uuid()"`
	Filename     string
	ObjectKey    string
	SizeBytes    int64
	ContentType  string
	Status       string
	OutputPrefix string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (Movie) TableName() string { return "movies" }
