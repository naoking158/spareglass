//go:build !darwin && !windows

package media

// InProcess reports whether NewPipeline captures and encodes without ffmpeg.
const InProcess = false

// NewPipeline has no implementation on this OS yet.
// TODO(linux): PipeWire node from the ScreenCast portal + VA-API.
func NewPipeline(displayID string, fps, bitrateKbps int) (Pipeline, error) {
	return nil, ErrNotImplemented
}
