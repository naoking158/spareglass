//go:build darwin && cgo

package media

/*
#include "native_darwin.h"
*/
import "C"

import (
	"fmt"
	"strconv"
)

// CursorPosition reads the pointer from the window server, for clients that
// draw the pointer over the native pipeline's video.
var CursorPosition CursorFunc = cursorPosition

func cursorPosition(displayID string) (Cursor, error) {
	id, err := strconv.ParseUint(displayID, 10, 32)
	if err != nil {
		return Cursor{}, fmt.Errorf("bad display id %q", displayID)
	}
	var x, y C.double
	on := C.sgCursorPosition(C.uint32_t(id), &x, &y)
	return Cursor{X: float64(x), Y: float64(y), OnDisplay: on != 0}, nil
}
