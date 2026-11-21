package vdisplay

import (
	"testing"
	"time"
)

func TestReloadsSinceCountsOnlyThisBoot(t *testing.T) {
	boot := time.Unix(1_800_000_000, 0)
	rec := reloadRecord(boot, 3)
	if got := reloadsSince(rec, boot.Add(2*time.Second)); got != 3 {
		t.Errorf("same boot = %d, want 3", got)
	}
	if got := reloadsSince(rec, boot.Add(time.Hour)); got != 0 {
		t.Errorf("later boot = %d, want 0", got)
	}
	if got := reloadsSince("", boot); got != 0 {
		t.Errorf("no record = %d, want 0", got)
	}
}
