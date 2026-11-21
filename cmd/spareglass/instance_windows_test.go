//go:build windows

package main

import (
	"errors"
	"testing"
)

func TestSecondInstanceIsRefused(t *testing.T) {
	release, err := claimInstance()
	if errors.Is(err, errAlreadyRunning) {
		t.Skip("a spareglass host is running on this computer")
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimInstance(); !errors.Is(err, errAlreadyRunning) {
		t.Fatalf("second claim: err = %v, want errAlreadyRunning", err)
	}
	release()
	again, err := claimInstance()
	if err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	again()
}
