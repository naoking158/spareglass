// Package vdisplay creates and removes virtual monitors that the OS treats as
// real displays, so the desktop extends onto them.
//
// Each OS has its own implementation, selected by build tags:
//
//	darwin   CGVirtualDisplay (private CoreGraphics API) via go-macos/virtualdisplay
//	windows  VirtualDrivers/Virtual-Display-Driver (IddCx), installed by the user
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

// Spec describes a display to create. Width and Height are in pixels.
type Spec struct {
	// Name should be stable (e.g. "spareglass 1"): some OSes remember
	// arrangement and color profiles per monitor identity.
	Name      string
	Width     int
	Height    int
	RefreshHz float64
	HiDPI     bool
}

// Display is a live virtual monitor.
type Display interface {
	// ID identifies the display for the capture layer
	// (CGDirectDisplayID on macOS, device name on Windows, PipeWire node on Linux).
	ID() string
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
