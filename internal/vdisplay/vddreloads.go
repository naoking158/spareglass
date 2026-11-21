package vdisplay

import (
	"fmt"
	"time"
)

// reloadsSince returns how many reloads the record counts for the system
// booted at boot. A record from an earlier boot counts none.
func reloadsSince(record string, boot time.Time) int {
	var unix int64
	var n int
	if _, err := fmt.Sscanf(record, "boot %d reloads %d", &unix, &n); err != nil {
		return 0
	}
	if d := boot.Sub(time.Unix(unix, 0)); d < -time.Minute || d > time.Minute {
		return 0
	}
	return n
}

func reloadRecord(boot time.Time, n int) string {
	return fmt.Sprintf("boot %d reloads %d\n", boot.Unix(), n)
}
