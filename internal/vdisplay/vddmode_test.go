package vdisplay

import "testing"

func TestNearestMode(t *testing.T) {
	modes := [][2]int{{800, 600}, {1366, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}, {1088, 486}, {486, 1088}, {1414, 707}, {1010, 500}}
	for _, c := range []struct {
		w, h int
		want [2]int
		near bool
	}{
		{1088, 486, [2]int{1088, 486}, true},
		{1092, 486, [2]int{1088, 486}, true},
		{486, 1092, [2]int{486, 1088}, true},
		{1600, 900, [2]int{1366, 768}, true},
		{2000, 1125, [2]int{1920, 1080}, true},
		{1180, 820, [2]int{800, 600}, false},
		{800, 360, [2]int{1088, 486}, false},
		{1000, 500, [2]int{1010, 500}, true},
	} {
		got, near := nearestMode(modes, c.w, c.h)
		if got != c.want || near != c.near {
			t.Errorf("nearestMode(%dx%d) = %v, %v; want %v, %v", c.w, c.h, got, near, c.want, c.near)
		}
	}
}
