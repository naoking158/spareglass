package vdisplay

import "math"

// nearestMode returns the mode whose aspect ratio is closest to w×h, and of
// those the one closest in area. close reports whether it is near enough to
// stand in for w×h: within 2% in aspect ratio and 1.5× in area.
func nearestMode(modes [][2]int, w, h int) (best [2]int, close bool) {
	score := func(m [2]int) (aspect, area float64) {
		aspect = math.Abs(math.Log(float64(m[0]*h) / float64(m[1]*w)))
		area = math.Abs(math.Log(float64(m[0]*m[1]) / float64(w*h)))
		return aspect, area
	}
	bestAspect, bestArea := math.Inf(1), math.Inf(1)
	for _, m := range modes {
		if m[0] <= 0 || m[1] <= 0 {
			continue
		}
		aspect, area := score(m)
		if aspect < bestAspect-0.005 || aspect < bestAspect+0.005 && area < bestArea {
			best, bestAspect, bestArea = m, aspect, area
		}
	}
	return best, bestAspect <= math.Log(1.02) && bestArea <= math.Log(1.5)
}
