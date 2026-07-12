package global

type Part struct {
	Number int
	ETag   string
	Size   int64
}

type InitResult struct {
	MovieID   string
	UploadID  string
	PartSize  int64
	PartCount int
}

type StatusResult struct {
	Status        string
	PartSize      int64
	PartCount     int
	UploadedParts []int
}

type InitUploadRequest struct {
	Filename    string `json:"filename" binding:"required"`
	Size        int64  `json:"size" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
}

type PresignPartsRequest struct {
	PartNumbers []int `json:"part_numbers" binding:"required"`
}
