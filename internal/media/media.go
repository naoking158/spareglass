// Package media defines the capture → encode pipeline that feeds a WebRTC
// video track.
//
// On macOS, builds with cgo capture with ScreenCaptureKit and encode with
// VideoToolbox in-process; builds without cgo run ffmpeg (avfoundation +
// VideoToolbox) and read its Annex-B output. Windows runs ffmpeg too. The
// plan is in-process capture/encode on every OS:
//
//	          capture                 encode
//	darwin   ScreenCaptureKit        VideoToolbox (H.264)
//	windows  Windows.Graphics.Capture Media Foundation / NVENC / AMF / QSV
//	linux    PipeWire (portal node)   VA-API / NVENC
//
// Frames should stay on the GPU between capture and encode
// (IOSurface / D3D11 texture / DMA-BUF) to avoid CPU copies.
package media

import (
	"context"
	"errors"
	"time"
)

// ErrNotImplemented means no pipeline exists for this platform yet.
var ErrNotImplemented = errors.New("capture/encode pipeline not implemented yet")

// Frame is an opaque captured frame (usually a GPU handle).
type Frame interface {
	Timestamp() time.Time
	Release()
}

// Capture produces frames of one display.
type Capture interface {
	// Start captures the display identified by vdisplay.Display.ID.
	Start(ctx context.Context, displayID string, fps int) (<-chan Frame, error)
	Stop() error
}

// EncodedFrame is one H.264 access unit in Annex-B format.
type EncodedFrame struct {
	Data []byte
	// Duration is the time since the previous frame (not until the next one,
	// unlike Pion's media.Sample).
	Duration time.Duration
	Keyframe bool
}

// Encoder turns frames into H.264 with low-latency settings
// (no B-frames, short GOP, CBR-ish rate control).
type Encoder interface {
	Encode(Frame) (*EncodedFrame, error)
	// RequestKeyframe is called when the browser sends a PLI.
	RequestKeyframe()
	SetBitrate(kbps int)
	Close() error
}

// Pipeline wires a capture source to an encoder.
type Pipeline interface {
	Run(ctx context.Context, out func(EncodedFrame) error) error
	RequestKeyframe()
	Close() error
}

// Display is the virtual display a pipeline captures.
type Display struct {
	// ID is the platform's display identifier (see vdisplay.Display.ID).
	ID string
	// Width and Height are the display's size in pixels.
	Width, Height int
}

// Factory builds the pipeline for one virtual display. NewPipeline is the
// platform default; tests substitute their own.
type Factory func(d Display, fps, bitrateKbps int) (Pipeline, error)
