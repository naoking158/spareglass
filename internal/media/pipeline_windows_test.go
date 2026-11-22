//go:build windows

package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/naoking158/spareglass/internal/windisplay"
)

var errEnough = errors.New("enough frames")

func TestPipelineCapturesPrimaryMonitor(t *testing.T) {
	if _, err := FFmpegPath(); err != nil {
		t.Skip("ffmpeg not installed")
	}
	list, err := windisplay.List()
	if err != nil {
		t.Fatal(err)
	}
	var primary string
	for _, m := range list {
		if m.Primary {
			primary = m.Device
		}
	}
	p, err := NewPipeline(Display{ID: primary}, 30, 4000)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	frames, keyframes := 0, 0
	err = p.Run(ctx, func(f EncodedFrame) error {
		frames++
		if f.Keyframe {
			keyframes++
		}
		if frames >= 30 {
			return errEnough
		}
		return nil
	})
	if !errors.Is(err, errEnough) {
		t.Fatalf("Run = %v after %d frames, want 30 frames", err, frames)
	}
	if keyframes == 0 {
		t.Fatal("no keyframe in 30 frames")
	}
}
