package vdisplay

import (
	"errors"
	"fmt"

	"github.com/naoking158/spareglass/internal/windisplay"
)

const vddAdapter = "Virtual Display Driver"

// maxReloads caps RELOAD_DRIVER per Windows boot, counted across processes
// and users in a file next to the driver settings: the driver stops after a
// few reloads (VirtualDrivers/Virtual-Display-Driver#351), so Open falls back
// to the closest mode the monitor already offers.
const maxReloads = 4

var (
	errNoSource    = errors.New("the Virtual Display Driver lists no display")
	errNoMonitor   = errors.New("the Virtual Display Driver has no monitor")
	errNoCloseMode = errors.New("the Virtual Display Driver's monitor offers no mode close to the requested size")
)

// vddSource returns the driver's source in list, preferring one with a
// monitor and then one on the desktop; the driver leaves sources from
// earlier reloads behind. ok is false when list has none.
func vddSource(list []windisplay.Monitor) (src windisplay.Monitor, ok bool) {
	bestScore := -1
	for _, m := range list {
		if m.Description != vddAdapter {
			continue
		}
		score := 0
		if m.MonitorID != "" {
			score += 2
		}
		if m.Attached {
			score++
		}
		if score > bestScore {
			src, bestScore, ok = m, score, true
		}
	}
	return src, ok
}

// vddConnected reports whether the driver's source src has a monitor: a
// monitor ID, or a display path (from QueryDisplayConfig) whose source is src.
func vddConnected(src windisplay.Monitor, paths []windisplay.Path) bool {
	if src.MonitorID != "" {
		return true
	}
	for _, p := range paths {
		if p.Source == src.Device {
			return true
		}
	}
	return false
}

// vddOpenPlan decides how Open gets a w×h display from the driver's source
// (found false when the driver lists none; connected when it has a monitor,
// which offers modes) after reloads reloads this boot. It returns the mode
// to use, or reload when the driver must be reloaded first. A source without
// a monitor cannot show anything whatever modes it lists, so it needs a
// reload too. With no reload left, it uses the nearest mode, or fails when
// there is none.
func vddOpenPlan(found, connected bool, modes [][2]int, w, h, reloads int) (mode [2]int, reload bool, err error) {
	switch {
	case !found:
		err = errNoSource
	case !connected:
		err = errNoMonitor
	default:
		m, near := nearestMode(modes, w, h)
		if near || reloads >= maxReloads && m != [2]int{} {
			return m, false, nil
		}
		err = errNoCloseMode
	}
	if reloads >= maxReloads {
		return [2]int{}, false, fmt.Errorf("%w: %w, and spareglass already reloaded the driver %d times since Windows started; restart Windows and try again", ErrUnavailable, err, reloads)
	}
	return [2]int{}, true, nil
}
