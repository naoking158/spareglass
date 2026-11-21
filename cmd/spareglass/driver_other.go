//go:build !windows

package main

import (
	"errors"

	"github.com/naoking158/spareglass/internal/vdisplay"
)

func installDriverIfMissing(vdisplay.Provider) {}

func runInstallDriver() error {
	return errors.New("install-driver is for Windows; other systems need no driver")
}
