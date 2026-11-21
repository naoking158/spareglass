package vdisplay

import (
	"testing"
	"time"
)

func TestReloadsSinceCountsOnlyThisBoot(t *testing.T) {
	rec := reloadRecord(2*time.Hour, 3)
	if got := reloadsSince(rec, 2*time.Hour+time.Second); got != 3 {
		t.Errorf("same boot = %d, want 3", got)
	}
	if got := reloadsSince(rec, time.Minute); got != 0 {
		t.Errorf("later boot = %d, want 0", got)
	}
	if got := reloadsSince("", time.Hour); got != 0 {
		t.Errorf("no record = %d, want 0", got)
	}
}
