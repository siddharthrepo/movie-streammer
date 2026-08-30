package probe

import (
	"context"
	"errors"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
)

var (
	ErrUnreadable = errors.New("source is not a readable media file")
	ErrNoVideo    = errors.New("source has no video stream")
)

type Prober interface {
	Inspect(ctx context.Context, url string) (*global.MediaInfo, error)
}
