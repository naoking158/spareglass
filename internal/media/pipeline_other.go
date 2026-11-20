//go:build !darwin && !windows

package media

// NewPipeline has no implementation on this OS yet.
// TODO(linux): PipeWire node from the ScreenCast portal + VA-API.
func NewPipeline(displayID string, fps, bitrateKbps int) (Pipeline, error) {
	return nil, ErrNotImplemented
}
