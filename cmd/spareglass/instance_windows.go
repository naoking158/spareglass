//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

// instanceName is machine-wide: the Virtual Display Driver's monitors are
// shared by every user session.
const instanceName = `Global\spareglass-serve`

var errAlreadyRunning = errors.New("spareglass is already running in another window; use that one, or close it and start again")

// claimInstance makes sure no other spareglass host runs on this computer.
// A second one would detach the first one's display when it starts. The
// claim lasts until release is called or the process exits.
func claimInstance() (release func(), err error) {
	name, err := windows.UTF16PtrFromString(instanceName)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(h)
		return nil, errAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = windows.CloseHandle(h) }, nil
}
