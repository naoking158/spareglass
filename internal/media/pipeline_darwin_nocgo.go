//go:build darwin && !cgo

package media

// InProcess reports whether NewPipeline captures and encodes without ffmpeg.
const InProcess = false

// NewPipeline captures and encodes the display with ffmpeg. Builds with cgo
// use ScreenCaptureKit and VideoToolbox in-process instead.
func NewPipeline(displayID string, fps, bitrateKbps int) (Pipeline, error) {
	return newFFmpegScreenPipeline(displayID, fps, bitrateKbps)
}
