package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/siddharthraturi/movie-streamer/upload-service/service"
)

type UploadController struct {
	svc *service.UploadService
}

func NewUploadController(svc *service.UploadService) *UploadController {
	return &UploadController{svc: svc}
}

type initUploadRequest struct {
	Filename    string `json:"filename" binding:"required"`
	Size        int64  `json:"size" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
}

func (ct *UploadController) InitUpload(c *gin.Context) {
	var req initUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := ct.svc.InitUpload(c.Request.Context(), req.Filename, req.Size, req.ContentType)
	if err != nil {
		ct.writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"movie_id":   res.MovieID,
		"upload_id":  res.UploadID,
		"part_size":  res.PartSize,
		"part_count": res.PartCount,
	})
}

type presignPartsRequest struct {
	PartNumbers []int `json:"part_numbers" binding:"required"`
}

func (ct *UploadController) PresignParts(c *gin.Context) {
	var req presignPartsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	urls, err := ct.svc.PresignParts(c.Request.Context(), c.Param("id"), req.PartNumbers)
	if err != nil {
		ct.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"urls": urls})
}

func (ct *UploadController) Status(c *gin.Context) {
	res, err := ct.svc.Status(c.Request.Context(), c.Param("id"))
	if err != nil {
		ct.writeError(c, err)
		return
	}
	uploaded := res.UploadedParts
	if uploaded == nil {
		uploaded = []int{}
	}
	c.JSON(http.StatusOK, gin.H{
		"status":         res.Status,
		"part_size":      res.PartSize,
		"part_count":     res.PartCount,
		"uploaded_parts": uploaded,
	})
}

func (ct *UploadController) Complete(c *gin.Context) {
	id := c.Param("id")
	if err := ct.svc.Complete(c.Request.Context(), id); err != nil {
		ct.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"movie_id": id, "status": "uploaded"})
}

func (ct *UploadController) Abort(c *gin.Context) {
	id := c.Param("id")
	if err := ct.svc.Abort(c.Request.Context(), id); err != nil {
		ct.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"movie_id": id, "status": "aborted"})
}

func (ct *UploadController) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrIncomplete):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
