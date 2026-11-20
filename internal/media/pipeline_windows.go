//go:build windows

package media

import (
	"fmt"
	"strconv"

	"github.com/naoking158/spareglass/internal/windisplay"
)

// NewPipeline captures the monitor named displayID (e.g. \\.\DISPLAY3) with
// ffmpeg's gdigrab over that monitor's desktop rectangle and encodes it with
// the Media Foundation H.264 encoder.
func NewPipeline(displayID string, fps, bitrateKbps int) (Pipeline, error) {
	if _, err := FFmpegPath(); err != nil {
		return nil, err
	}
	m, err := windisplay.Find(displayID)
	if err != nil {
		return nil, err
	}
	if !m.Attached || m.Width == 0 || m.Height == 0 {
		return nil, fmt.Errorf("display %s is not part of the desktop", displayID)
	}
	kbps := strconv.Itoa(bitrateKbps) + "k"
	args := []string{
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "gdigrab", "-framerate", strconv.Itoa(fps), "-draw_mouse", "1",
		"-offset_x", strconv.Itoa(m.X), "-offset_y", strconv.Itoa(m.Y),
		"-video_size", fmt.Sprintf("%dx%d", m.Width, m.Height),
		"-i", "desktop",
		"-c:v", "h264_mf", "-rate_control", "cbr",
		"-b:v", kbps, "-maxrate", kbps, "-bufsize", kbps,
		"-g", strconv.Itoa(fps), "-bf", "0", "-pix_fmt", "nv12",
		"-bsf:v", "dump_extra=freq=keyframe",
		"-flags", "low_delay",
		"-f", "h264", "pipe:1",
	}
	return NewFFmpegPipeline(args, fps)
}
