//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"github.com/naoking158/spareglass/internal/vdisplay"
)

// installDriverIfMissing installs the Virtual Display Driver shipped next to
// spareglass.exe when it has never been installed and someone is at the
// console to answer Windows' prompts.
func installDriverIfMissing(p vdisplay.Provider) {
	if p.Available() == nil || vdisplay.DriverInstalled() || !stdinIsConsole() {
		return
	}
	setup, err := driverSetupPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot install the Virtual Display Driver: %v.\nExtract the whole zip and start spareglass.exe from the extracted folder.\n", err)
		return
	}
	if err := installDriver(setup); err != nil {
		fmt.Fprintln(os.Stderr, "Could not install the Virtual Display Driver:", err)
	}
}

func runInstallDriver() error {
	setup, err := driverSetupPath()
	if err != nil {
		return err
	}
	return installDriver(setup)
}

func installDriver(setup string) error {
	return vdisplay.InstallDriver(os.Stdout, setup, filepath.Join(os.TempDir(), "spareglass-driver-setup.log"))
}

func driverSetupPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	p := filepath.Join(filepath.Dir(exe), vdisplay.DriverSetup)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("its installer %s is missing", p)
	}
	return p, nil
}

func stdinIsConsole() bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) == nil
}
