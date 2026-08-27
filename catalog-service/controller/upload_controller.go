package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/service"
)

type UploadController struct {
	svc service.UploadService
}

func NewUploadController(svc service.UploadService) *UploadController {
	return &UploadController{svc: svc}
}

func (c *UploadController) Initiate(ctx *gin.Context) {
	var req global.InitiateUploadRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		badRequest(ctx, err)
		return
	}

	resp, err := c.svc.Initiate(ctx.Request.Context(), req)
	if err != nil {
		failUpload(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, resp)
}

func (c *UploadController) Complete(ctx *gin.Context) {
	var req global.CompleteUploadRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		badRequest(ctx, err)
		return
	}

	job, err := c.svc.Complete(ctx.Request.Context(), ctx.Param("id"), req.Parts)
	if err != nil {
		failUpload(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, job.ToResponse())
}

func (c *UploadController) Get(ctx *gin.Context) {
	job, err := c.svc.Get(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		failUpload(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, job.ToResponse())
}

func (c *UploadController) Abort(ctx *gin.Context) {
	if err := c.svc.Abort(ctx.Request.Context(), ctx.Param("id")); err != nil {
		failUpload(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func failUpload(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		ctx.JSON(http.StatusNotFound, global.ErrorResponse{Error: "upload job not found"})
	case errors.Is(err, service.ErrInvalid):
		ctx.JSON(http.StatusBadRequest, global.ErrorResponse{Error: "invalid request", Details: err.Error()})
	case errors.Is(err, service.ErrConflict):
		ctx.JSON(http.StatusConflict, global.ErrorResponse{Error: "upload job state does not allow this"})
	case errors.Is(err, service.ErrMismatch):
		ctx.JSON(http.StatusUnprocessableEntity, global.ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrStorage):
		ctx.JSON(http.StatusBadGateway, global.ErrorResponse{Error: "object storage unavailable"})
	default:
		ctx.JSON(http.StatusInternalServerError, global.ErrorResponse{Error: "internal error"})
	}
}
