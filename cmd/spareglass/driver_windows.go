//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"github.com/naoking158/spareglass/internal/vdisplay"
)

// driverSetup is where release archives put the Virtual Display Driver
// installer, relative to spareglass.exe.
const driverSetup = `driver\VirtualDisplayDriver-setup.exe`

// installDriverIfMissing installs the Virtual Display Driver shipped next to
// spareglass.exe when it is missing and someone is at the console to answer
// Windows' prompts.
func installDriverIfMissing(p vdisplay.Provider) {
	if p.Available() == nil || !stdinIsConsole() {
		return
	}
	if err := runInstallDriver(); err != nil {
		fmt.Fprintln(os.Stderr, "Could not install the Virtual Display Driver:", err)
	}
}

func runInstallDriver() error {
	setup, err := driverSetupPath()
	if err != nil {
		return err
	}
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
	p := filepath.Join(filepath.Dir(exe), driverSetup)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("%w; extract the whole zip and start spareglass.exe from the extracted folder", err)
	}
	return p, nil
}

func stdinIsConsole() bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) == nil
}
