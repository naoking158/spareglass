// Package media defines the capture → encode pipeline that feeds a WebRTC
// video track. Implementations are per OS and will likely need cgo:
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
	Data     []byte
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

// NewPipeline returns the platform pipeline. None exists yet.
func NewPipeline(displayID string, fps, bitrateKbps int) (Pipeline, error) {
	return nil, ErrNotImplemented
}
