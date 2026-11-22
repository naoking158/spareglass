//go:build darwin && cgo

package media

/*
#include "native_darwin.h"
*/
import "C"

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

// CursorPosition reads the pointer from the window server. The native
// pipeline leaves the pointer out of the video, so the client draws it.
var CursorPosition CursorFunc = cursorPosition

// boundsMaxAge is how long a display's rectangle is reused. Reading it can
// take longer than one poll at 120Hz; a display the user drags to a new
// place is picked up within this time.
const boundsMaxAge = time.Second

type cachedBounds struct {
	x, y, w, h float64
	at         time.Time
}

var (
	boundsMu    sync.Mutex
	boundsCache = map[uint32]cachedBounds{}
)

func displayBounds(id uint32) cachedBounds {
	boundsMu.Lock()
	defer boundsMu.Unlock()
	b, ok := boundsCache[id]
	if ok && time.Since(b.at) < boundsMaxAge {
		return b
	}
	var x, y, w, h C.double
	C.sgDisplayBounds(C.uint32_t(id), &x, &y, &w, &h)
	b = cachedBounds{x: float64(x), y: float64(y), w: float64(w), h: float64(h), at: time.Now()}
	boundsCache[id] = b
	return b
}

func cursorPosition(displayID string) (Cursor, error) {
	id, err := strconv.ParseUint(displayID, 10, 32)
	if err != nil {
		return Cursor{}, fmt.Errorf("bad display id %q", displayID)
	}
	b := displayBounds(uint32(id))
	var cx, cy C.double
	C.sgCursorLocation(&cx, &cy)
	x, y := float64(cx)-b.x, float64(cy)-b.y
	on := b.w > 0 && x >= 0 && y >= 0 && x < b.w && y < b.h
	return Cursor{X: x, Y: y, OnDisplay: on}, nil
}
