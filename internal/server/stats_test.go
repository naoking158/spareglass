package server

import (
	"fmt"
	"testing"
	"time"
)

func TestFrameStatsResetsBetweenReports(t *testing.T) {
	var s frameStats
	t0 := time.Now()
	s.add(1000, true, t0)
	s.add(200, false, t0.Add(20*time.Millisecond))

	first := fmt.Sprint(s.take()...)
	if want := "host=[frames=2 keyframes=1 avg_bytes=600 max_bytes=1000 max_keyframe_bytes=1000 max_gap_ms=20]"; first != want {
		t.Fatalf("first report = %s, want %s", first, want)
	}

	s.add(300, false, t0.Add(50*time.Millisecond))
	second := fmt.Sprint(s.take()...)
	if want := "host=[frames=1 keyframes=0 avg_bytes=300 max_bytes=300 max_keyframe_bytes=0 max_gap_ms=30]"; second != want {
		t.Fatalf("second report = %s, want %s", second, want)
	}
}
