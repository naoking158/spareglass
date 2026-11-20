//go:build windows

package vdisplay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/naoking158/spareglass/internal/windisplay"
)

// Default returns the Windows provider, which drives the Virtual Display
// Driver from github.com/VirtualDrivers/Virtual-Display-Driver (an IddCx
// driver the user installs once).
//
// The driver offers only the resolutions listed in its vdd_settings.xml, so
// Open adds the requested size there and sends RELOAD_DRIVER over the
// driver's named pipe when its monitor lacks that mode, then sets the mode
// and places the monitor right of the others. The driver cannot go below one
// monitor, so Close detaches it from the desktop instead of removing it.
func Default() Provider { return &vddProvider{} }

const (
	vddAdapter  = "Virtual Display Driver"
	vddPipe     = `\\.\pipe\MTTVirtualDisplayPipe`
	vddRegKey   = `SOFTWARE\MikeTheTech\VirtualDisplayDriver`
	vddDir      = `C:\VirtualDisplayDriver`
	reloadLimit = 20 * time.Second
)

// ErrNoDriver means the Virtual Display Driver is not installed.
var ErrNoDriver = errors.New("the Virtual Display Driver is not installed: run VirtualDisplayDriver-setup.exe from the spareglass folder (see HOW-TO-USE.txt)")

type vddProvider struct {
	mu   sync.Mutex
	open *vddDisplay
}

func (*vddProvider) Name() string {
	return "Windows Virtual Display Driver (VirtualDrivers/Virtual-Display-Driver)"
}

func (*vddProvider) Available() error {
	if _, err := os.Stat(vddSettingsPath()); err != nil {
		return ErrNoDriver
	}
	_, err := vddDevice()
	return err
}

func (p *vddProvider) Open(s Spec) (Display, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open != nil {
		return nil, errors.New("Windows supports one spareglass display at a time")
	}
	dev, err := vddDevice()
	if err != nil {
		return nil, err
	}
	if !windisplay.HasMode(dev, s.Width, s.Height) {
		if dev, err = addVDDMode(s.Width, s.Height, int(s.RefreshHz)); err != nil {
			return nil, err
		}
	}
	x, err := rightEdge(dev)
	if err != nil {
		return nil, err
	}
	if err := windisplay.SetMode(dev, s.Width, s.Height, int(s.RefreshHz), x, 0); err != nil {
		if err := windisplay.SetMode(dev, s.Width, s.Height, 0, x, 0); err != nil {
			return nil, err
		}
	}
	m, err := windisplay.Find(dev)
	if err != nil {
		return nil, err
	}
	if !m.Attached || m.Width != s.Width || m.Height != s.Height {
		_ = windisplay.Detach(dev)
		return nil, fmt.Errorf("Windows set %s to %dx%d instead of %dx%d", dev, m.Width, m.Height, s.Width, s.Height)
	}
	p.open = &vddDisplay{p: p, device: dev, spec: s}
	return p.open, nil
}

// CloseAll detaches every Virtual Display Driver monitor, including the one
// the driver attaches by itself after installation or a reload.
func (p *vddProvider) CloseAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.open = nil
	list, err := windisplay.List()
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range list {
		if m.Description == vddAdapter && m.Attached {
			errs = append(errs, windisplay.Detach(m.Device))
		}
	}
	return errors.Join(errs...)
}

type vddDisplay struct {
	p      *vddProvider
	device string
	spec   Spec
}

func (d *vddDisplay) ID() string { return d.device }
func (d *vddDisplay) Spec() Spec { return d.spec }

func (d *vddDisplay) Close() error {
	d.p.mu.Lock()
	defer d.p.mu.Unlock()
	if d.p.open != d {
		return nil
	}
	d.p.open = nil
	return windisplay.Detach(d.device)
}

// vddDevice returns the device name of the driver's monitor; it changes
// every time the driver reloads.
func vddDevice() (string, error) {
	list, err := windisplay.List()
	if err != nil {
		return "", err
	}
	for _, m := range list {
		if m.Description == vddAdapter {
			return m.Device, nil
		}
	}
	return "", ErrNoDriver
}

// addVDDMode lists width×height in the driver settings, reloads the driver
// and waits until its monitor offers that mode. It returns the monitor's new
// device name.
func addVDDMode(width, height, hz int) (string, error) {
	path := vddSettingsPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	updated, err := withVDDResolution(string(b), width, height, hz)
	if err != nil {
		return "", err
	}
	if updated != string(b) {
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return "", fmt.Errorf("update %s: %w", path, err)
		}
	}
	if err := vddCommand("RELOAD_DRIVER"); err != nil {
		return "", fmt.Errorf("reload the Virtual Display Driver: %w", err)
	}
	deadline := time.Now().Add(reloadLimit)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		if dev, err := vddDevice(); err == nil && windisplay.HasMode(dev, width, height) {
			return dev, nil
		}
	}
	return "", fmt.Errorf("the Virtual Display Driver did not offer %dx%d after a reload", width, height)
}

// rightEdge returns the x coordinate just right of every attached monitor
// other than dev.
func rightEdge(dev string) (int, error) {
	list, err := windisplay.List()
	if err != nil {
		return 0, err
	}
	x := 0
	for _, m := range list {
		if m.Attached && m.Device != dev && m.X+m.Width > x {
			x = m.X + m.Width
		}
	}
	return x, nil
}

func vddSettingsPath() string {
	dir := vddDir
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, vddRegKey, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("VDDPATH"); err == nil && v != "" {
			dir = v
		}
		k.Close()
	}
	return filepath.Join(dir, "vdd_settings.xml")
}

// vddCommand sends one UTF-16LE command to the driver's control pipe.
func vddCommand(cmd string) error {
	name, err := windows.UTF16PtrFromString(vddPipe)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", vddPipe, err)
	}
	defer windows.CloseHandle(h)
	u := utf16.Encode([]rune(cmd))
	buf := make([]byte, 2*len(u))
	for i, c := range u {
		buf[2*i], buf[2*i+1] = byte(c), byte(c>>8)
	}
	var n uint32
	return windows.WriteFile(h, buf, &n, nil)
}
