package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ErrNoFFmpeg means ffmpeg is neither next to the spareglass executable nor on PATH.
var ErrNoFFmpeg = errors.New("ffmpeg not found next to spareglass or on PATH (use the ffmpeg from the release archive, or: brew install ffmpeg)")

// FFmpegPath returns the ffmpeg executable, or ErrNoFFmpeg. The copy shipped
// next to the spareglass executable in release archives wins over PATH.
func FFmpegPath() (string, error) {
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		if p, ok := bundledFFmpeg(filepath.Dir(exe)); ok {
			return p, nil
		}
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", ErrNoFFmpeg
	}
	return p, nil
}

func bundledFFmpeg(dir string) (string, bool) {
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(dir, name)
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return p, true
}

// FFmpegHasEncoder reports whether ffmpeg lists the named encoder. The list
// is read once per ffmpeg executable; a failed read is retried next time.
func FFmpegHasEncoder(name string) bool {
	p, err := FFmpegPath()
	if err != nil {
		return false
	}
	return bytes.Contains(ffmpegEncoders(p), []byte(" "+name+" "))
}

var encoderLists sync.Map // ffmpeg path → `ffmpeg -encoders` output

func ffmpegEncoders(path string) []byte {
	if out, ok := encoderLists.Load(path); ok {
		return out.([]byte)
	}
	out, err := exec.Command(path, "-hide_banner", "-encoders").Output()
	if err != nil {
		return nil
	}
	encoderLists.Store(path, out)
	return out
}

// ffmpegPipeline runs ffmpeg writing raw H.264 (Annex-B) to stdout.
// It is the first, simplest pipeline: capture and hardware encoding are
// ffmpeg's job, and spareglass only splits access units for WebRTC.
type ffmpegPipeline struct {
	args []string
	fps  int

	mu     sync.Mutex
	cancel context.CancelFunc
	closed bool
}

// NewFFmpegPipeline runs ffmpeg with args; args must end in an H.264
// Annex-B output to pipe:1. Exposed for tests and experiments.
func NewFFmpegPipeline(args []string, fps int) (Pipeline, error) {
	if _, err := FFmpegPath(); err != nil {
		return nil, err
	}
	return &ffmpegPipeline{args: args, fps: fps}, nil
}

func (p *ffmpegPipeline) Run(ctx context.Context, out func(EncodedFrame) error) error {
	path, err := FFmpegPath()
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	parent := ctx
	ctx, p.cancel = context.WithCancel(ctx)
	p.mu.Unlock()

	cmd := exec.CommandContext(ctx, path, p.args...)
	var stderr tailBuffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	readErr := ReadAccessUnits(stdout, p.fps, out)
	stopped := p.isClosed() || parent.Err() != nil
	if readErr != nil {
		p.cancel()
	}
	waitErr := cmd.Wait()
	if stopped {
		return nil
	}
	if readErr != nil {
		return readErr
	}
	if waitErr != nil {
		return fmt.Errorf("ffmpeg: %w: %s", waitErr, stderr.String())
	}
	return nil
}

// RequestKeyframe cannot be forwarded to an ffmpeg process; the GOP is kept
// short (one second) instead.
func (p *ffmpegPipeline) RequestKeyframe() {}

func (p *ffmpegPipeline) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *ffmpegPipeline) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// tailBuffer keeps the last few KB of ffmpeg's stderr for error messages.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > 4096 {
		t.buf = t.buf[len(t.buf)-4096:]
	}
	return len(b), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

// TestPatternArgs encodes ffmpeg's test source; used by tests and handy for
// checking the browser side without a virtual display. It uses libx264 when
// ffmpeg has it, otherwise the platform encoder that the LGPL ffmpeg shipped
// in release archives has (VideoToolbox on macOS, Media Foundation on Windows).
func TestPatternArgs(width, height, fps int) []string {
	encoder := []string{"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-profile:v", "baseline", "-pix_fmt", "yuv420p"}
	switch {
	case FFmpegHasEncoder("libx264"):
	case FFmpegHasEncoder("h264_videotoolbox"):
		encoder = []string{"-c:v", "h264_videotoolbox", "-realtime", "1", "-allow_sw", "1", "-profile:v", "baseline", "-b:v", "4000k", "-pix_fmt", "nv12"}
	case FFmpegHasEncoder("h264_mf"):
		encoder = []string{"-c:v", "h264_mf", "-rate_control", "cbr", "-b:v", "4000k", "-pix_fmt", "nv12", "-flags", "low_delay"}
	}
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-re", "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=%d", width, height, fps),
	}
	args = append(args, encoder...)
	return append(args,
		"-g", fmt.Sprint(fps), "-bf", "0",
		"-f", "h264", "pipe:1",
	)
}
