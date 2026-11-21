//go:build windows

package vdisplay

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"golang.org/x/sys/windows/registry"
)

const installWait = 30 * time.Second

// ErrDriverInstalled means the Virtual Display Driver's setup has already run
// on this computer.
var ErrDriverInstalled = errors.New(`the Virtual Display Driver is already installed (to reinstall it, first uninstall "Virtual Display Driver" in Settings > Apps)`)

// driverInstalled reports whether the Virtual Display Driver's setup has run
// on this computer, even if the driver is not working now.
func driverInstalled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, vddRegKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue("InstalledBy")
	return err == nil
}

// InstallDriver runs the Virtual Display Driver setup at setupPath without its
// wizard and without its VDD Control app (which needs the .NET Desktop
// Runtime), writes the setup log to logPath, and waits until the driver's
// monitor appears. It tells w what Windows is about to ask: whether to allow
// the change and whether to install the device software. It returns
// ErrDriverInstalled without running the setup, which would otherwise ask in
// English whether to uninstall the driver, when that setup has already run
// on this computer.
func InstallDriver(w io.Writer, setupPath, logPath string) error {
	if driverInstalled() {
		return ErrDriverInstalled
	}
	fmt.Fprintln(w, "Installing the Virtual Display Driver (first time only).")
	fmt.Fprintln(w, `Windows asks twice: answer "Yes", then "Install".`)
	cmd := exec.Command(setupPath, "/SILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-", "/COMPONENTS=VDD", "/LOG="+logPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w (setup log: %s)", setupPath, err, logPath)
	}
	deadline := time.Now().Add(installWait)
	for {
		if _, err := vddDevice(); err == nil {
			fmt.Fprintln(w, "The Virtual Display Driver is installed.")
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(`the setup finished but the driver's display did not appear; if "Install" was declined, uninstall "Virtual Display Driver" in Settings > Apps and start spareglass.exe again (setup log: %s)`, logPath)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
