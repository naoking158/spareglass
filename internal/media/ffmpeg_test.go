package media

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testPattern(t *testing.T) Pipeline {
	t.Helper()
	if _, err := FFmpegPath(); err != nil {
		t.Skip("ffmpeg not installed")
	}
	p, err := NewFFmpegPipeline(TestPatternArgs(320, 240, 30), 30)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFFmpegPipelineReturnsOutputError(t *testing.T) {
	p := testPattern(t)
	boom := errors.New("track write failed")
	err := p.Run(context.Background(), func(EncodedFrame) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("Run = %v, want the output error", err)
	}
}

func TestFFmpegPipelineCloseIsNotAnError(t *testing.T) {
	p := testPattern(t)
	first := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- p.Run(context.Background(), func(EncodedFrame) error {
			select {
			case first <- struct{}{}:
			default:
			}
			return nil
		})
	}()
	select {
	case <-first:
	case <-time.After(10 * time.Second):
		t.Fatal("no frame from ffmpeg")
	}
	_ = p.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after Close = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after Close")
	}
}
