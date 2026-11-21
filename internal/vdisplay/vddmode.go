package vdisplay

import "math"

// nearestMode returns the mode closest to w×h: first by aspect ratio, then by
// area. near reports whether some mode can stand in for w×h, within 2% in
// aspect ratio and 1.5× in area; the closest such mode is returned then.
func nearestMode(modes [][2]int, w, h int) (best [2]int, near bool) {
	bestAspect, bestArea := math.Inf(1), math.Inf(1)
	for _, m := range modes {
		if m[0] <= 0 || m[1] <= 0 {
			continue
		}
		aspect := math.Abs(math.Log(float64(m[0]*h) / float64(m[1]*w)))
		area := math.Abs(math.Log(float64(m[0]*m[1]) / float64(w*h)))
		ok := aspect <= math.Log(1.02) && area <= math.Log(1.5)
		closer := aspect < bestAspect-0.005 || aspect < bestAspect+0.005 && area < bestArea
		if !(ok && !near || ok == near && closer) {
			continue
		}
		best, bestAspect, bestArea, near = m, aspect, area, ok
	}
	return best, near
}
