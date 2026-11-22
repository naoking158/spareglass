//go:build windows

package windisplay

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayDevicesW      = user32.NewProc("EnumDisplayDevicesW")
	procEnumDisplaySettingsW     = user32.NewProc("EnumDisplaySettingsW")
	procChangeDisplaySettingsExW = user32.NewProc("ChangeDisplaySettingsExW")
)

const (
	enumCurrentSettings = 0xFFFFFFFF

	dmPosition    = 0x00000020
	dmPelsWidth   = 0x00080000
	dmPelsHeight  = 0x00100000
	dmDisplayFreq = 0x00400000

	cdsUpdateRegistry = 0x00000001
	cdsNoReset        = 0x10000000

	attachedToDesktop = 0x00000001
	primaryDevice     = 0x00000004
)

type displayDevice struct {
	cb           uint32
	deviceName   [32]uint16
	deviceString [128]uint16
	stateFlags   uint32
	deviceID     [128]uint16
	deviceKey    [128]uint16
}

// devMode is DEVMODEW with the display branch of its unions.
type devMode struct {
	deviceName       [32]uint16
	specVersion      uint16
	driverVersion    uint16
	size             uint16
	driverExtra      uint16
	fields           uint32
	positionX        int32
	positionY        int32
	orientation      uint32
	fixedOutput      uint32
	color            int16
	duplex           int16
	yResolution      int16
	ttOption         int16
	collate          int16
	formName         [32]uint16
	logPixels        uint16
	bitsPerPel       uint32
	pelsWidth        uint32
	pelsHeight       uint32
	displayFlags     uint32
	displayFrequency uint32
	icmMethod        uint32
	icmIntent        uint32
	mediaType        uint32
	ditherType       uint32
	reserved1        uint32
	reserved2        uint32
	panningWidth     uint32
	panningHeight    uint32
}

// Monitor is one display device. X, Y, Width and Height are desktop
// coordinates in physical pixels and are zero when it is not attached.
type Monitor struct {
	Device      string // e.g. \\.\DISPLAY3
	Description string // adapter name, e.g. "Virtual Display Driver"
	Attached    bool
	Primary     bool
	Flags       uint32 // DISPLAY_DEVICE stateFlags of the source
	MonitorID   string // DeviceID of the first monitor on the source, "" when none
	X, Y        int
	Width       int
	Height      int
}

// List returns every display device, attached or not.
func List() ([]Monitor, error) {
	var out []Monitor
	for i := uint32(0); ; i++ {
		var d displayDevice
		d.cb = uint32(unsafe.Sizeof(d))
		if r, _, _ := procEnumDisplayDevicesW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&d)), 0); r == 0 {
			break
		}
		m := Monitor{
			Device:      windows.UTF16ToString(d.deviceName[:]),
			Description: windows.UTF16ToString(d.deviceString[:]),
			Attached:    d.stateFlags&attachedToDesktop != 0,
			Primary:     d.stateFlags&primaryDevice != 0,
			Flags:       d.stateFlags,
			MonitorID:   monitorID(&d.deviceName[0]),
		}
		if m.Attached {
			if dm, ok := currentMode(m.Device); ok {
				m.X, m.Y = int(dm.positionX), int(dm.positionY)
				m.Width, m.Height = int(dm.pelsWidth), int(dm.pelsHeight)
			}
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, errors.New("EnumDisplayDevices returned no devices")
	}
	return out, nil
}

// monitorID returns the DeviceID of the first monitor EnumDisplayDevices
// lists under the source, or "" when the source has none.
func monitorID(source *uint16) string {
	var d displayDevice
	d.cb = uint32(unsafe.Sizeof(d))
	if r, _, _ := procEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(source)), 0, uintptr(unsafe.Pointer(&d)), 0); r == 0 {
		return ""
	}
	return windows.UTF16ToString(d.deviceID[:])
}

// Find returns the monitor with the given device name.
func Find(device string) (Monitor, error) {
	list, err := List()
	if err != nil {
		return Monitor{}, err
	}
	for _, m := range list {
		if m.Device == device {
			return m, nil
		}
	}
	return Monitor{}, fmt.Errorf("display %s not found", device)
}

// Modes returns the distinct sizes the device offers.
func Modes(device string) [][2]int {
	name, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return nil
	}
	seen := map[[2]int]bool{}
	var out [][2]int
	for i := uint32(0); ; i++ {
		var dm devMode
		dm.size = uint16(unsafe.Sizeof(dm))
		if r, _, _ := procEnumDisplaySettingsW.Call(uintptr(unsafe.Pointer(name)), uintptr(i), uintptr(unsafe.Pointer(&dm))); r == 0 {
			return out
		}
		m := [2]int{int(dm.pelsWidth), int(dm.pelsHeight)}
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
}

// HasMode reports whether the device offers width×height at any refresh rate.
func HasMode(device string, width, height int) bool {
	for _, m := range Modes(device) {
		if m == [2]int{width, height} {
			return true
		}
	}
	return false
}

// SetMode attaches the device to the desktop at (x, y) with the given size
// and refresh rate (0 keeps the driver's choice), and applies it.
func SetMode(device string, width, height, hz, x, y int) error {
	var dm devMode
	dm.size = uint16(unsafe.Sizeof(dm))
	dm.fields = dmPelsWidth | dmPelsHeight | dmPosition
	dm.pelsWidth, dm.pelsHeight = uint32(width), uint32(height)
	dm.positionX, dm.positionY = int32(x), int32(y)
	if hz > 0 {
		dm.fields |= dmDisplayFreq
		dm.displayFrequency = uint32(hz)
	}
	return change(device, &dm)
}

// Detach removes the device from the desktop (Windows forgets its area);
// the device itself stays and can be attached again with SetMode.
func Detach(device string) error {
	var dm devMode
	dm.size = uint16(unsafe.Sizeof(dm))
	dm.fields = dmPelsWidth | dmPelsHeight | dmPosition
	return change(device, &dm)
}

func change(device string, dm *devMode) error {
	name, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return err
	}
	r, _, _ := procChangeDisplaySettingsExW.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(dm)), 0, cdsUpdateRegistry|cdsNoReset, 0)
	if int32(r) != 0 {
		return fmt.Errorf("ChangeDisplaySettingsEx(%s, %dx%d at %d,%d): %s", device, dm.pelsWidth, dm.pelsHeight, dm.positionX, dm.positionY, dispChange(int32(r)))
	}
	r, _, _ = procChangeDisplaySettingsExW.Call(0, 0, 0, 0, 0)
	if int32(r) != 0 {
		return fmt.Errorf("ChangeDisplaySettingsEx(apply): %s", dispChange(int32(r)))
	}
	return nil
}

func dispChange(code int32) string {
	names := map[int32]string{
		1: "DISP_CHANGE_RESTART", -1: "DISP_CHANGE_FAILED", -2: "DISP_CHANGE_BADMODE", -3: "DISP_CHANGE_NOTUPDATED",
		-4: "DISP_CHANGE_BADFLAGS", -5: "DISP_CHANGE_BADPARAM", -6: "DISP_CHANGE_BADDUALVIEW",
	}
	if n, ok := names[code]; ok {
		return fmt.Sprintf("%s (%d)", n, code)
	}
	return fmt.Sprintf("code %d", code)
}

func currentMode(device string) (devMode, bool) {
	var dm devMode
	dm.size = uint16(unsafe.Sizeof(dm))
	name, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return dm, false
	}
	r, _, _ := procEnumDisplaySettingsW.Call(uintptr(unsafe.Pointer(name)), enumCurrentSettings, uintptr(unsafe.Pointer(&dm)))
	return dm, r != 0
}
