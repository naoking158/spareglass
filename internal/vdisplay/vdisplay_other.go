//go:build !darwin && !windows && !linux

package vdisplay

// Default returns a provider that reports the platform as unsupported.
func Default() Provider {
	return unsupported{name: "unsupported OS", reason: "only macOS, Windows and Linux are targeted"}
}
