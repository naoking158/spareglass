package windisplay

// Monitor is one display device. X, Y, Width and Height are desktop
// coordinates in physical pixels and are zero when it is not attached.
type Monitor struct {
	Device      string // e.g. \\.\DISPLAY3
	Description string // adapter name, e.g. "Virtual Display Driver"
	Attached    bool
	Primary     bool
	Flags       uint32 // DISPLAY_DEVICE stateFlags of the source
	MonitorID   string // DeviceID of the first monitor on the source, "" when none
	X, Y        int
	Width       int
	Height      int
}
