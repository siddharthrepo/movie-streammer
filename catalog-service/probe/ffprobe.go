package probe

import (
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
)

type ffProbe struct {
	binary  string
	timeout time.Duration
}

func NewFFProbe(binary string, timeout time.Duration) Prober {
	if binary == "" {
		binary = "ffprobe"
	}
	return &ffProbe{binary: binary, timeout: timeout}
}

func safeURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return "<source>"
	}
	u.RawQuery = ""
	u.User = nil
	return u.Redacted()
}

func redact(text, url string) string {
	cleaned := strings.ReplaceAll(text, url, safeURL(url))
	if i := strings.Index(cleaned, "X-Amz-Signature="); i >= 0 {
		cleaned = cleaned[:i] + "X-Amz-Signature=REDACTED"
	}
	return cleaned
}

func (p *ffProbe) Inspect(ctx context.Context, url string) (*global.MediaInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.binary,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-seekable", "1",
		url,
	)

	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("%w: %v: %s", ErrUnreadable, err, redact(stderr, url))
	}

	var parsed global.FFOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("%w: parse ffprobe output: %v", ErrUnreadable, err)
	}

	info := &global.MediaInfo{}
	for _, s := range parsed.Streams {
		switch s.CodecType {
		case "video":
			if info.VideoCodec == "" {
				info.VideoCodec = s.CodecName
				info.Width = s.Width
				info.Height = s.Height
			}
		case "audio":
			if info.AudioCodec == "" {
				info.AudioCodec = s.CodecName
				info.HasAudio = true
			}
		}
	}

	if info.VideoCodec == "" || info.Height == 0 {
		return nil, ErrNoVideo
	}

	seconds, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil || seconds <= 0 {
		return nil, fmt.Errorf("%w: duration %q", ErrUnreadable, parsed.Format.Duration)
	}
	info.DurationMs = int64(seconds * 1000)

	if br, err := strconv.ParseInt(parsed.Format.BitRate, 10, 64); err == nil {
		info.Bitrate = br
	}

	return info, nil
}
