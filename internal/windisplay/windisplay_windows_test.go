//go:build windows

package windisplay

import (
	"testing"
	"unsafe"
)

func TestStructSizesMatchWin32(t *testing.T) {
	if got := unsafe.Sizeof(devMode{}); got != 220 {
		t.Errorf("sizeof(DEVMODEW) = %d, want 220", got)
	}
	if got := unsafe.Sizeof(displayDevice{}); got != 840 {
		t.Errorf("sizeof(DISPLAY_DEVICEW) = %d, want 840", got)
	}
	if got := unsafe.Sizeof(pathInfo{}); got != 72 {
		t.Errorf("sizeof(DISPLAYCONFIG_PATH_INFO) = %d, want 72", got)
	}
	if got := unsafe.Sizeof(modeInfo{}); got != 64 {
		t.Errorf("sizeof(DISPLAYCONFIG_MODE_INFO) = %d, want 64", got)
	}
	if got := unsafe.Sizeof(sourceDeviceName{}); got != 84 {
		t.Errorf("sizeof(DISPLAYCONFIG_SOURCE_DEVICE_NAME) = %d, want 84", got)
	}
}

func TestListFindsThePrimaryMonitor(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		if m.Primary {
			if !m.Attached || m.Width == 0 || m.Height == 0 {
				t.Fatalf("primary monitor without a mode: %+v", m)
			}
			return
		}
	}
	t.Fatalf("no primary monitor in %+v", list)
}
