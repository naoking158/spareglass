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
