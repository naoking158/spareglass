//go:build windows

package vdisplay

import (
	"errors"
	"fmt"
	"log/slog"
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
// driver installed once with InstallDriver).
//
// The driver offers only the resolutions listed in its vdd_settings.xml, so
// Open adds the requested size there and sends RELOAD_DRIVER over the
// driver's named pipe when its monitor lacks that mode or is missing, then
// sets the mode and places the monitor right of the others. The driver cannot
// go below one monitor, so Close detaches it from the desktop instead of
// removing it.
func Default() Provider { return &vddProvider{} }

const (
	vddAdapter  = "Virtual Display Driver"
	vddPipe     = `\\.\pipe\MTTVirtualDisplayPipe`
	vddRegKey   = `SOFTWARE\MikeTheTech\VirtualDisplayDriver`
	vddDir      = `C:\VirtualDisplayDriver`
	reloadLimit = 20 * time.Second
)

// ErrNoDriver means the Virtual Display Driver is not installed.
var ErrNoDriver = fmt.Errorf("%w: the Virtual Display Driver is not installed: start spareglass.exe from the extracted spareglass folder, or run \"spareglass install-driver\", to install it (see HOW-TO-USE.txt)", ErrUnavailable)

type vddProvider struct {
	mu   sync.Mutex
	open *vddDisplay
}

func (*vddProvider) Name() string {
	return "Windows Virtual Display Driver (VirtualDrivers/Virtual-Display-Driver)"
}

func (*vddProvider) Available() error {
	if !vddInstalled() {
		return ErrNoDriver
	}
	return nil
}

func (p *vddProvider) Open(s Spec) (Display, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open != nil {
		return nil, fmt.Errorf("%w: Windows supports one spareglass display at a time", ErrUnavailable)
	}
	dev, err := vddDevice()
	switch {
	case err == nil && windisplay.HasMode(dev, s.Width, s.Height):
		markVDDModeUsed(s.Width, s.Height)
	case err != nil && !vddInstalled():
		return nil, err
	default:
		if dev, err = addVDDMode(s.Width, s.Height, int(s.RefreshHz)); err != nil {
			_ = detachVDDMonitors()
			return nil, err
		}
	}
	if err := attach(dev, s); err != nil {
		_ = windisplay.Detach(dev)
		return nil, err
	}
	p.open = &vddDisplay{p: p, device: dev, spec: s}
	return p.open, nil
}

// attach sets dev to the spec's size right of the other monitors and checks
// that Windows applied it.
func attach(dev string, s Spec) error {
	x, err := rightEdge(dev)
	if err != nil {
		return err
	}
	if err := setMode(dev, s, x); err != nil {
		before := "unknown"
		if m, ferr := windisplay.Find(dev); ferr == nil {
			before = describe(m)
		}
		aerr := windisplay.Activate(dev)
		if aerr == nil {
			if aerr = setMode(dev, s, x); aerr != nil {
				aerr = errors.Join(aerr, windisplay.Detach(dev))
			}
		}
		if aerr != nil {
			return fmt.Errorf("%w; then %w (monitor before: %s)", err, aerr, before)
		}
		slog.Info("virtual display attached through SetDisplayConfig", "device", dev, "legacy_err", err)
	}
	m, err := windisplay.Find(dev)
	if err != nil {
		return err
	}
	if !m.Attached || m.Width != s.Width || m.Height != s.Height {
		return fmt.Errorf("Windows set %s to %dx%d instead of %dx%d", dev, m.Width, m.Height, s.Width, s.Height)
	}
	return nil
}

func setMode(dev string, s Spec, x int) error {
	if err := windisplay.SetMode(dev, s.Width, s.Height, int(s.RefreshHz), x, 0); err != nil {
		return windisplay.SetMode(dev, s.Width, s.Height, 0, x, 0)
	}
	return nil
}

func describe(m windisplay.Monitor) string {
	if !m.Attached {
		return m.Device + " detached"
	}
	return fmt.Sprintf("%s %dx%d at %d,%d", m.Device, m.Width, m.Height, m.X, m.Y)
}

// CloseAll detaches every Virtual Display Driver monitor, including ones left
// attached by an earlier run or by the driver itself.
func (p *vddProvider) CloseAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.open = nil
	return detachVDDMonitors()
}

// detachVDDMonitors detaches every attached Virtual Display Driver monitor;
// the driver attaches one by itself after installation and every reload.
func detachVDDMonitors() error {
	list, err := windisplay.List()
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range list {
		if m.Description != vddAdapter {
			continue
		}
		slog.Info("Virtual Display Driver monitor", "state", describe(m))
		if m.Attached {
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
		return "", fmt.Errorf("%s: %w", path, err)
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

// markVDDModeUsed records in the driver settings that width×height was used
// again, so that it outlives sizes used less recently. The driver reads the
// file on its next reload; nothing is reloaded now.
func markVDDModeUsed(width, height int) {
	path := vddSettingsPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if updated, ok := withVDDResolutionUsed(string(b), width, height); ok && updated != string(b) {
		_ = os.WriteFile(path, []byte(updated), 0o644)
	}
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

// vddInstalled reports whether the driver's settings file exists, which
// stays true while its monitor is gone after a failed reload.
func vddInstalled() bool {
	_, err := os.Stat(vddSettingsPath())
	return err == nil
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
