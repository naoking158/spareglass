//go:build windows

package vdisplay

// Default returns the Windows provider.
//
// TODO(windows): drive an Indirect Display Driver (IddCx). The plan is to
// depend on the signed VirtualDrivers/Virtual-Display-Driver (VDD) rather than
// shipping our own driver, add/remove monitors through its control interface,
// and always remove them on disconnect (a watchdog process guards crashes).
func Default() Provider {
	return unsupported{
		name:   "Windows Indirect Display Driver",
		reason: "install Virtual-Display-Driver; integration is planned",
	}
}
