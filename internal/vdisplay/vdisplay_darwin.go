//go:build darwin

package vdisplay

import (
	"strconv"

	vd "github.com/go-macos/virtualdisplay"
)

// Default returns the macOS provider.
func Default() Provider { return macProvider{} }

type macProvider struct{}

func (macProvider) Name() string     { return "macOS CGVirtualDisplay (private API, macOS 14+)" }
func (macProvider) Available() error { return vd.Available() }
func (macProvider) CloseAll() error  { return vd.CloseAll() }

func (macProvider) Open(s Spec) (Display, error) {
	d, err := vd.Open(vd.Spec{
		Name:        s.Name,
		Width:       uint32(s.Width),
		Height:      uint32(s.Height),
		RefreshRate: s.RefreshHz,
		HiDPI:       s.HiDPI,
	})
	if err != nil {
		return nil, err
	}
	return &macDisplay{d: d, spec: s}, nil
}

type macDisplay struct {
	d    *vd.Display
	spec Spec
}

func (m *macDisplay) ID() string   { return strconv.FormatUint(uint64(m.d.ID()), 10) }
func (m *macDisplay) Spec() Spec   { return m.spec }
func (m *macDisplay) Close() error { return m.d.Close() }
