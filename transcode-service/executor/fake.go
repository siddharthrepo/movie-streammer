package executor

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/siddharthraturi/movie-streamer/transcode-service/model"
)

type fakeExecutor struct {
	duration    time.Duration
	failPercent int
}

func NewFake(duration time.Duration, failPercent int) Executor {
	return &fakeExecutor{duration: duration, failPercent: failPercent}
}

func (e *fakeExecutor) Execute(ctx context.Context, chunk model.TranscodeChunk) error {
	t := time.NewTimer(e.duration)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
	}

	if e.failPercent > 0 && rand.Intn(100) < e.failPercent {
		return fmt.Errorf("injected failure on chunk %d quality %s", chunk.ChunkIndex, chunk.Quality)
	}
	return nil
}
