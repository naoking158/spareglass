//go:build windows

package main

import (
	"errors"
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
		return
	}
	if err := installDriver(setup); err != nil {
		fmt.Fprintln(os.Stderr, "Could not install the Virtual Display Driver:", err)
	}
}

func runInstallDriver() error {
	if vdisplay.DriverInstalled() {
		return errors.New(`the Virtual Display Driver is already installed (to reinstall it, first uninstall "Virtual Display Driver" in Settings > Apps)`)
	}
	setup, err := driverSetupPath()
	if err != nil {
		return err
	}
	return installDriver(setup)
}

func installDriver(setup string) error {
	fmt.Println("Installing the Virtual Display Driver (first time only).")
	fmt.Println(`Windows asks twice: answer "Yes", then "Install".`)
	if err := vdisplay.InstallDriver(setup, filepath.Join(os.TempDir(), "spareglass-driver-setup.log")); err != nil {
		return err
	}
	fmt.Println("The Virtual Display Driver is installed.")
	return nil
}

func driverSetupPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	p := filepath.Join(filepath.Dir(exe), vdisplay.DriverSetup)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("the driver installer is missing: %w", err)
	}
	return p, nil
}

func stdinIsConsole() bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) == nil
}
