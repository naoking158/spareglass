package vdisplay

import "testing"

func TestNearestMode(t *testing.T) {
	modes := [][2]int{{800, 600}, {1366, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}, {1088, 486}, {486, 1088}}
	for _, c := range []struct {
		w, h  int
		want  [2]int
		close bool
	}{
		{1088, 486, [2]int{1088, 486}, true},
		{1092, 486, [2]int{1088, 486}, true},
		{486, 1092, [2]int{486, 1088}, true},
		{1600, 900, [2]int{1366, 768}, true},
		{2000, 1125, [2]int{1920, 1080}, true},
		{1180, 820, [2]int{800, 600}, false},
		{800, 360, [2]int{1088, 486}, false},
	} {
		got, close := nearestMode(modes, c.w, c.h)
		if got != c.want || close != c.close {
			t.Errorf("nearestMode(%dx%d) = %v, %v; want %v, %v", c.w, c.h, got, close, c.want, c.close)
		}
	}
}
