package executor

import (
	"context"

	"github.com/siddharthraturi/movie-streamer/transcode-service/model"
)

type Executor interface {
	Execute(ctx context.Context, chunk model.TranscodeChunk) error
}
