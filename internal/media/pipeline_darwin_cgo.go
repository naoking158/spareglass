//go:build darwin && cgo

package media

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework ScreenCaptureKit -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreGraphics -framework Foundation
#include <stdlib.h>
#include "native_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"runtime/cgo"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// InProcess reports whether NewPipeline captures and encodes without ffmpeg.
const InProcess = true

// NewPipeline captures the display with ScreenCaptureKit and encodes it
// with VideoToolbox in-process. The encoder holds no frames, and a keyframe
// request re-encodes the last captured picture.
func NewPipeline(displayID string, fps, bitrateKbps int) (Pipeline, error) {
	id, err := strconv.ParseUint(displayID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("bad display id %q", displayID)
	}
	return &nativePipeline{displayID: uint32(id), fps: fps, kbps: bitrateKbps, stop: make(chan struct{})}, nil
}

// nativeFrame is one access unit from the encoder; pts is the capture time.
type nativeFrame struct {
	data []byte
	pts  time.Duration
	key  bool
}

// sink receives what the Objective-C side delivers for one capture.
type sink struct {
	frames  chan nativeFrame
	errs    chan error
	dropped atomic.Bool
}

func newSink() *sink {
	return &sink{frames: make(chan nativeFrame, 8), errs: make(chan error, 1)}
}

//export sgDeliverFrame
func sgDeliverFrame(h C.uintptr_t, data unsafe.Pointer, n C.int, pts C.int64_t, key C.int) {
	s := cgo.Handle(h).Value().(*sink)
	f := nativeFrame{data: C.GoBytes(data, n), pts: time.Duration(pts), key: key != 0}
	select {
	case s.frames <- f:
	default:
		s.dropped.Store(true)
	}
}

//export sgDeliverError
func sgDeliverError(h C.uintptr_t, msg *C.char) {
	s := cgo.Handle(h).Value().(*sink)
	select {
	case s.errs <- errors.New(C.GoString(msg)):
	default:
	}
}

type nativePipeline struct {
	displayID uint32
	fps, kbps int

	mu      sync.Mutex
	capture *C.sgCapture
	closed  bool
	stop    chan struct{}
}

func (p *nativePipeline) Run(ctx context.Context, out func(EncodedFrame) error) error {
	if p.isClosed() {
		return nil
	}
	s := newSink()
	h := cgo.NewHandle(s)
	defer h.Delete()

	var cerr *C.char
	c := C.sgCaptureStart(C.uint32_t(p.displayID), C.int(p.fps), C.int(p.kbps), C.uintptr_t(h), &cerr)
	if c == nil {
		defer C.free(unsafe.Pointer(cerr))
		return errors.New(C.GoString(cerr))
	}
	p.mu.Lock()
	p.capture = c
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.capture = nil
		p.mu.Unlock()
		C.sgCaptureStop(c)
	}()

	nominal := time.Second / time.Duration(max(p.fps, 1))
	var last time.Duration
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-p.stop:
			return nil
		case err := <-s.errs:
			if p.isClosed() {
				return nil
			}
			return err
		case f := <-s.frames:
			if s.dropped.Swap(false) {
				p.RequestKeyframe()
			}
			d := nominal
			if last != 0 && f.pts > last {
				d = f.pts - last
			}
			last = f.pts
			if err := out(EncodedFrame{Data: f.data, Duration: d, Keyframe: f.key}); err != nil {
				return err
			}
		}
	}
}

func (p *nativePipeline) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *nativePipeline) RequestKeyframe() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.capture != nil {
		C.sgCaptureRequestKeyframe(p.capture)
	}
}

func (p *nativePipeline) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.stop)
	}
	return nil
}

// encodeGenerated encodes n generated frames with the pipeline's encoder
// settings. It fails if a frame's output has not arrived within wait after
// the frame was submitted, i.e. if the encoder holds frames back.
func encodeGenerated(width, height, fps, kbps, n int, wait time.Duration) ([]nativeFrame, error) {
	s := newSink()
	h := cgo.NewHandle(s)
	defer h.Delete()
	var cerr *C.char
	e := C.sgEncoderNew(C.int(width), C.int(height), C.int(fps), C.int(kbps), C.uintptr_t(h), &cerr)
	if e == nil {
		defer C.free(unsafe.Pointer(cerr))
		return nil, errors.New(C.GoString(cerr))
	}
	defer C.sgEncoderFree(e)

	var got []nativeFrame
	interval := time.Second / time.Duration(fps)
	for i := range n {
		if st := C.sgEncoderEncodeTestFrame(e, C.int(i), C.int64_t(time.Duration(i)*interval)); st != 0 {
			return got, fmt.Errorf("encode frame %d: OSStatus %d", i, st)
		}
		select {
		case f := <-s.frames:
			got = append(got, f)
		case err := <-s.errs:
			return got, err
		case <-time.After(wait):
			return got, fmt.Errorf("frame %d: no output within %v; the encoder holds frames", i, wait)
		}
	}
	return got, nil
}
