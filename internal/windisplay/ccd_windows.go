//go:build windows

package windisplay

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetDisplayConfigBufferSizes = user32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig          = user32.NewProc("QueryDisplayConfig")
	procSetDisplayConfig            = user32.NewProc("SetDisplayConfig")
	procDisplayConfigGetDeviceInfo  = user32.NewProc("DisplayConfigGetDeviceInfo")
)

const (
	qdcAllPaths        = 0x1
	qdcOnlyActivePaths = 0x2

	sdcUseSuppliedDisplayConfig = 0x20
	sdcValidate                 = 0x40
	sdcApply                    = 0x80
	sdcSaveToDatabase           = 0x200
	sdcAllowChanges             = 0x400

	pathActive        = 0x1
	modeIdxInvalid    = 0xFFFFFFFF
	getSourceNameType = 1
)

type luid struct {
	low  uint32
	high int32
}

// pathInfo is DISPLAYCONFIG_PATH_INFO.
type pathInfo struct {
	sourceAdapter   luid
	sourceID        uint32
	sourceModeIdx   uint32
	sourceStatus    uint32
	targetAdapter   luid
	targetID        uint32
	targetModeIdx   uint32
	outputTech      uint32
	rotation        uint32
	scaling         uint32
	refreshNum      uint32
	refreshDen      uint32
	scanLineOrder   uint32
	targetAvailable int32
	targetStatus    uint32
	flags           uint32
}

// modeInfo is DISPLAYCONFIG_MODE_INFO; the union is kept as raw bytes.
type modeInfo struct {
	infoType uint32
	id       uint32
	adapter  luid
	union    [6]uint64
}

// sourceDeviceName is DISPLAYCONFIG_SOURCE_DEVICE_NAME.
type sourceDeviceName struct {
	infoType uint32
	size     uint32
	adapter  luid
	id       uint32
	gdiName  [32]uint16
}

// Activate adds the device to the desktop with SetDisplayConfig, keeping
// every active display as it is; Windows picks the mode and position.
func Activate(device string) error {
	active, modes, err := queryConfig(qdcOnlyActivePaths)
	if err != nil {
		return err
	}
	all, _, err := queryConfig(qdcAllPaths)
	if err != nil {
		return err
	}
	var path *pathInfo
	for i := range all {
		p := all[i]
		if p.flags&pathActive == 0 && p.targetAvailable != 0 && sourceName(p.sourceAdapter, p.sourceID) == device {
			path = &p
			break
		}
	}
	if path == nil {
		return fmt.Errorf("SetDisplayConfig: no inactive path for %s", device)
	}
	path.flags |= pathActive
	path.sourceModeIdx, path.targetModeIdx = modeIdxInvalid, modeIdxInvalid
	paths := append(active, *path)
	flags := uint32(sdcUseSuppliedDisplayConfig | sdcAllowChanges | sdcSaveToDatabase)
	if err := setConfig(paths, modes, flags|sdcValidate); err != nil {
		return fmt.Errorf("SetDisplayConfig(validate %s): %w", device, err)
	}
	if err := setConfig(paths, modes, flags|sdcApply); err != nil {
		return fmt.Errorf("SetDisplayConfig(%s): %w", device, err)
	}
	return nil
}

func queryConfig(flags uint32) ([]pathInfo, []modeInfo, error) {
	for {
		var np, nm uint32
		if r, _, _ := procGetDisplayConfigBufferSizes.Call(uintptr(flags), uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&nm))); r != 0 {
			return nil, nil, fmt.Errorf("GetDisplayConfigBufferSizes: %w", windows.Errno(r))
		}
		paths := make([]pathInfo, np+1)
		modes := make([]modeInfo, nm+1)
		r, _, _ := procQueryDisplayConfig.Call(uintptr(flags), uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&paths[0])), uintptr(unsafe.Pointer(&nm)), uintptr(unsafe.Pointer(&modes[0])), 0)
		switch windows.Errno(r) {
		case 0:
			return paths[:np], modes[:nm], nil
		case windows.ERROR_INSUFFICIENT_BUFFER:
			continue
		default:
			return nil, nil, fmt.Errorf("QueryDisplayConfig: %w", windows.Errno(r))
		}
	}
}

func setConfig(paths []pathInfo, modes []modeInfo, flags uint32) error {
	var mp uintptr
	if len(modes) > 0 {
		mp = uintptr(unsafe.Pointer(&modes[0]))
	}
	r, _, _ := procSetDisplayConfig.Call(uintptr(len(paths)), uintptr(unsafe.Pointer(&paths[0])), uintptr(len(modes)), mp, uintptr(flags))
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

func sourceName(adapter luid, id uint32) string {
	n := sourceDeviceName{infoType: getSourceNameType, adapter: adapter, id: id}
	n.size = uint32(unsafe.Sizeof(n))
	if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&n))); r != 0 {
		return ""
	}
	return windows.UTF16ToString(n.gdiName[:])
}
