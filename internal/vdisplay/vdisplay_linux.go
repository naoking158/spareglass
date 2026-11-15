//go:build linux

package vdisplay

import "os"

// Default returns the Linux provider.
//
// TODO(linux): on Wayland, request a VIRTUAL source from the
// org.freedesktop.portal.ScreenCast portal (KDE) or call
// org.gnome.Mutter.ScreenCast.Session.RecordVirtual directly (GNOME), then
// read the PipeWire node. On X11, fall back to xrandr virtual outputs.
func Default() Provider {
	session := os.Getenv("XDG_SESSION_TYPE")
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	return unsupported{
		name:   "Linux (" + orUnknown(session) + ", " + orUnknown(desktop) + ")",
		reason: "portal VIRTUAL / Mutter RecordVirtual integration is planned",
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
