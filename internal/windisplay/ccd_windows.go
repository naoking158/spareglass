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

// Activate puts the target behind device on the desktop with
// SetDisplayConfig, keeping every active display as it is; Windows picks the
// mode and position. The target may sit behind another source of the same
// adapter, so Activate returns the GDI name that now shows it.
func Activate(device string) (string, error) {
	active, modes, err := queryConfig(qdcOnlyActivePaths)
	if err != nil {
		return "", err
	}
	raw, _, err := queryConfig(qdcAllPaths)
	if err != nil {
		return "", err
	}
	paths := describePaths(raw)
	i, on, err := pickPath(paths, device)
	if err != nil {
		return "", fmt.Errorf("SetDisplayConfig: %w; paths %s", err, formatPaths(adapterPaths(paths, device)))
	}
	if on {
		return paths[i].Source, nil
	}
	path := raw[i]
	path.flags |= pathActive
	path.sourceModeIdx, path.targetModeIdx = modeIdxInvalid, modeIdxInvalid
	supplied := append(active, path)
	flags := uint32(sdcUseSuppliedDisplayConfig | sdcAllowChanges | sdcSaveToDatabase)
	if err := setConfig(supplied, modes, flags|sdcValidate); err != nil {
		return "", fmt.Errorf("SetDisplayConfig(validate %s): %w; paths %s", paths[i], err, formatPaths(adapterPaths(paths, device)))
	}
	if err := setConfig(supplied, modes, flags|sdcApply); err != nil {
		return "", fmt.Errorf("SetDisplayConfig(%s): %w", paths[i], err)
	}
	return paths[i].Source, nil
}

// Paths returns QueryDisplayConfig(QDC_ALL_PATHS) for the adapter that owns
// device, or every path when no path names device.
func Paths(device string) ([]Path, error) {
	raw, _, err := queryConfig(qdcAllPaths)
	if err != nil {
		return nil, err
	}
	return adapterPaths(describePaths(raw), device), nil
}

func describePaths(raw []pathInfo) []Path {
	out := make([]Path, len(raw))
	for i, p := range raw {
		out[i] = Path{
			SourceAdapter: p.sourceAdapter.uint64(),
			SourceID:      p.sourceID,
			Source:        sourceName(p.sourceAdapter, p.sourceID),
			TargetAdapter: p.targetAdapter.uint64(),
			TargetID:      p.targetID,
			Active:        p.flags&pathActive != 0,
			Available:     p.targetAvailable != 0,
			TargetStatus:  p.targetStatus,
		}
	}
	return out
}

func (l luid) uint64() uint64 { return uint64(uint32(l.high))<<32 | uint64(l.low) }

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
