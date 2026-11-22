// Package vdisplay creates and removes virtual monitors that the OS treats as
// real displays, so the desktop extends onto them.
//
// Each OS has its own implementation, selected by build tags:
//
//	darwin   CGVirtualDisplay (private CoreGraphics API) via go-macos/virtualdisplay
//	windows  VirtualDrivers/Virtual-Display-Driver (IddCx), installed once from the release archive
//	linux    xdg-desktop-portal ScreenCast VIRTUAL / Mutter RecordVirtual (planned)
package vdisplay

import (
	"errors"
	"fmt"
)

// ErrNotImplemented means this platform has no working provider yet.
var ErrNotImplemented = errors.New("virtual displays are not implemented on this platform yet")

// ErrUnavailable means Open cannot succeed at any size right now (a driver
// is missing, or the only display slot is taken), so trying a smaller size
// is pointless.
var ErrUnavailable = errors.New("virtual displays are unavailable")

// Spec describes a display to create.
type Spec struct {
	// Name should be stable (e.g. "spareglass 1"): some OSes remember
	// arrangement and color profiles per monitor identity.
	Name string
	// Width and Height are the desktop size in points, which is what sets
	// the size of the UI on the display.
	Width     int
	Height    int
	RefreshHz float64
	// HiDPI asks for twice as many pixels as points in each direction
	// (Retina). Providers that cannot do it create a 1:1 display, and
	// Display.Spec reports which one was made.
	HiDPI bool
}

// PixelSize returns the display's size in pixels.
func (s Spec) PixelSize() (w, h int) {
	if s.HiDPI {
		return 2 * s.Width, 2 * s.Height
	}
	return s.Width, s.Height
}

// Display is a live virtual monitor.
type Display interface {
	// ID identifies the display for the capture layer
	// (CGDirectDisplayID on macOS, device name on Windows, PipeWire node on Linux).
	ID() string
	// Spec is the display that was made, which can differ from the one
	// requested (HiDPI is false when the provider made a 1:1 display).
	Spec() Spec
	// Close removes the display. Callers recreate rather than resize:
	// on macOS a mode change can make a display impossible to remove.
	Close() error
}

// Provider creates displays on the current OS.
type Provider interface {
	Name() string
	// Available reports whether Open can work on this machine.
	Available() error
	Open(Spec) (Display, error)
	// CloseAll removes every display this process created, and any leftover
	// that the platform can identify as spareglass's.
	CloseAll() error
}

type unsupported struct{ name, reason string }

func (u unsupported) Name() string     { return u.name }
func (u unsupported) Available() error { return fmt.Errorf("%w: %s", ErrNotImplemented, u.reason) }
func (u unsupported) Open(Spec) (Display, error) {
	return nil, u.Available()
}
func (u unsupported) CloseAll() error { return nil }
