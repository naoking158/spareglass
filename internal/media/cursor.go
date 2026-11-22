package media

// Cursor is where the pointer is relative to one display.
type Cursor struct {
	// X and Y are in points from the display's top-left corner.
	X, Y float64
	// OnDisplay is false when the pointer is on another display.
	OnDisplay bool
}

// CursorFunc reads the pointer position relative to the display identified
// by vdisplay.Display.ID.
type CursorFunc func(displayID string) (Cursor, error)
