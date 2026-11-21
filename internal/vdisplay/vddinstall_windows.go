//go:build windows

package vdisplay

import (
	"fmt"
	"os/exec"
	"time"

	"golang.org/x/sys/windows/registry"
)

// DriverSetup is where release archives put the Virtual Display Driver
// installer, relative to the spareglass executable.
const DriverSetup = `driver\VirtualDisplayDriver-setup.exe`

const installWait = 30 * time.Second

// DriverInstalled reports whether the Virtual Display Driver's setup has run
// on this computer, even if the driver's monitor is missing now. Running the
// setup again then asks, in English, whether to uninstall it.
func DriverInstalled() bool {
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
// monitor appears. Windows still asks the user to allow the change and to
// install the device software.
func InstallDriver(setupPath, logPath string) error {
	cmd := exec.Command(setupPath, "/SILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-", "/COMPONENTS=VDD", "/LOG="+logPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w (log: %s)", setupPath, err, logPath)
	}
	p := &vddProvider{}
	deadline := time.Now().Add(installWait)
	for {
		err := p.Available()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w (setup log: %s)", err, logPath)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
