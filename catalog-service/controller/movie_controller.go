package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/model"
	"github.com/siddharthraturi/movie-streamer/catalog-service/service"
)

type MovieController struct {
	svc service.MovieService
}

func NewMovieController(svc service.MovieService) *MovieController {
	return &MovieController{svc: svc}
}

func (c *MovieController) Create(ctx *gin.Context) {
	var req global.CreateMovieRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		badRequest(ctx, err)
		return
	}

	m, err := c.svc.Create(ctx.Request.Context(), req)
	if err != nil {
		fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, m.ToResponse())
}

func (c *MovieController) Get(ctx *gin.Context) {
	m, err := c.svc.Get(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, m.ToResponse())
}

func (c *MovieController) List(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", strconv.Itoa(global.DefaultPageSize)))

	items, total, err := c.svc.List(ctx.Request.Context(), page, pageSize)
	if err != nil {
		fail(ctx, err)
		return
	}

	resp := global.ListMoviesResponse{
		Items:    toResponses(items),
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}
	ctx.JSON(http.StatusOK, resp)
}

func (c *MovieController) Update(ctx *gin.Context) {
	var req global.UpdateMovieRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		badRequest(ctx, err)
		return
	}

	m, err := c.svc.Update(ctx.Request.Context(), ctx.Param("id"), req)
	if err != nil {
		fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, m.ToResponse())
}

func (c *MovieController) Delete(ctx *gin.Context) {
	if err := c.svc.Delete(ctx.Request.Context(), ctx.Param("id")); err != nil {
		fail(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func toResponses(items []model.Movie) []global.MovieResponse {
	out := make([]global.MovieResponse, 0, len(items))
	for i := range items {
		out = append(out, items[i].ToResponse())
	}
	return out
}

func badRequest(ctx *gin.Context, err error) {
	ctx.JSON(http.StatusBadRequest, global.ErrorResponse{
		Error:   "invalid request",
		Details: err.Error(),
	})
}

func fail(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		ctx.JSON(http.StatusNotFound, global.ErrorResponse{Error: "movie not found"})
	case errors.Is(err, service.ErrInvalid):
		ctx.JSON(http.StatusBadRequest, global.ErrorResponse{Error: "invalid request"})
	default:
		ctx.JSON(http.StatusInternalServerError, global.ErrorResponse{Error: "internal error"})
	}
}
