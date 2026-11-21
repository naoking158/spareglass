package vdisplay

import (
	"fmt"
	"time"
)

// reloadsSince returns how many reloads the record counts for the current
// boot, given how long the system has been up. A record written later in
// uptime than now comes from an earlier boot and counts none.
func reloadsSince(record string, uptime time.Duration) int {
	var ms int64
	var n int
	if _, err := fmt.Sscanf(record, "uptime_ms %d reloads %d", &ms, &n); err != nil {
		return 0
	}
	if time.Duration(ms)*time.Millisecond > uptime {
		return 0
	}
	return n
}

func reloadRecord(uptime time.Duration, n int) string {
	return fmt.Sprintf("uptime_ms %d reloads %d\n", uptime.Milliseconds(), n)
}
