package model

import "time"

type TranscodeChunk struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	JobID          string `gorm:"type:char(36);not null;index"`
	MovieID        string `gorm:"type:char(36);not null"`
	ChunkIndex     int    `gorm:"not null"`
	Quality        string `gorm:"type:varchar(8);not null"`
	StartMs        int64  `gorm:"not null"`
	EndMs          int64  `gorm:"not null"`
	SegmentOffset  int    `gorm:"not null"`
	State          string `gorm:"type:varchar(16);not null"`
	LeaseOwner     *string
	LeaseExpiresAt *time.Time
	Attempts       int `gorm:"not null;default:0"`
	Error          *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (TranscodeChunk) TableName() string {
	return "transcode_chunks"
}
