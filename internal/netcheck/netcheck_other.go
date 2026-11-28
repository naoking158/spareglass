//go:build !darwin && !windows

package netcheck

func check() []Problem { return nil }
