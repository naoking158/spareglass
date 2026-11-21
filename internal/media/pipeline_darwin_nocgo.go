//go:build darwin && !cgo

package media

// NewPipeline captures and encodes the display with ffmpeg. Builds with cgo
// use ScreenCaptureKit and VideoToolbox in-process instead.
func NewPipeline(displayID string, fps, bitrateKbps int) (Pipeline, error) {
	return newFFmpegScreenPipeline(displayID, fps, bitrateKbps)
}
