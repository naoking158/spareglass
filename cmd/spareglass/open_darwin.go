package main

import "os/exec"

// openBrowser opens url in the default browser.
func openBrowser(url string) error {
	cmd := exec.Command("open", url)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
