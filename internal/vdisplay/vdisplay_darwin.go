//go:build darwin

package vdisplay

import (
	"errors"
	"strconv"
	"time"

	vd "github.com/go-macos/virtualdisplay"
)

// retinaDPI is the pixel density reported for a HiDPI display. macOS starts
// a display this dense in its Retina mode when the display also offers a mode
// at half its pixel size, so no mode change is needed (a mode change can make
// a virtual display unremovable).
const retinaDPI = 220

const waitGone = 5 * time.Second

var errNotRetina = errors.New("display came up 1:1, not in its Retina mode")

// Default returns the macOS provider.
func Default() Provider { return macProvider{} }

type macProvider struct{}

func (macProvider) Name() string     { return "macOS CGVirtualDisplay (private API, macOS 14+)" }
func (macProvider) Available() error { return vd.Available() }
func (macProvider) CloseAll() error  { return vd.CloseAll() }

// Open creates s. For a HiDPI request that macOS does not start in its
// Retina mode (it picks 1:1 for small sizes), it makes a 1:1 display at the
// same size in points instead.
func (p macProvider) Open(s Spec) (Display, error) {
	if s.HiDPI {
		if d, err := openRetina(s); err == nil {
			return d, nil
		}
		s.HiDPI = false
	}
	d, err := vd.Open(vd.Spec{Name: s.Name, Width: uint32(s.Width), Height: uint32(s.Height), RefreshRate: s.RefreshHz})
	if err != nil {
		return nil, err
	}
	return &macDisplay{d: d, spec: s}, nil
}

func openRetina(s Spec) (Display, error) {
	pw, ph := s.PixelSize()
	d, err := vd.Open(vd.Spec{
		Name:        s.Name,
		Width:       uint32(pw),
		Height:      uint32(ph),
		RefreshRate: s.RefreshHz,
		HiDPI:       true,
		ExtraModes:  []vd.Mode{{Width: uint32(s.Width), Height: uint32(s.Height), RefreshRate: s.RefreshHz}},
		SizeMM:      vd.Size{Width: float64(pw) * 25.4 / retinaDPI, Height: float64(ph) * 25.4 / retinaDPI},
	})
	if err != nil {
		return nil, err
	}
	if !inPoints(d.ID(), s.Width, s.Height) {
		_ = d.Close()
		_ = vd.WaitGone(waitGone, d.ID())
		return nil, errNotRetina
	}
	return &macDisplay{d: d, spec: s}, nil
}

// inPoints reports whether display id is w x h points, i.e. it came up in
// the Retina mode rather than 1:1 at its full pixel size.
func inPoints(id uint32, w, h int) bool {
	ds, err := vd.ActiveDisplays()
	if err != nil {
		return false
	}
	for _, d := range ds {
		if d.ID == id {
			return int(d.Width) == w && int(d.Height) == h
		}
	}
	return false
}

type macDisplay struct {
	d    *vd.Display
	spec Spec
}

func (m *macDisplay) ID() string   { return strconv.FormatUint(uint64(m.d.ID()), 10) }
func (m *macDisplay) Spec() Spec   { return m.spec }
func (m *macDisplay) Close() error { return m.d.Close() }
