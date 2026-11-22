package server

import (
	"log/slog"
	"sync"
	"time"
)

// frameStats accumulates what the host sent since the last client report.
type frameStats struct {
	mu     sync.Mutex
	counts frameCounts
}

type frameCounts struct {
	frames    int
	keyframes int
	bytes     int
	maxBytes  int
	maxKey    int
	maxGap    time.Duration
	last      time.Time
}

func (s *frameStats) add(size int, key bool, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := &s.counts
	f.frames++
	f.bytes += size
	f.maxBytes = max(f.maxBytes, size)
	if key {
		f.keyframes++
		f.maxKey = max(f.maxKey, size)
	}
	if !f.last.IsZero() {
		f.maxGap = max(f.maxGap, now.Sub(f.last))
	}
	f.last = now
}

// restart drops the previous frame's time, so the gap across a pipeline
// restart (display recreated) is not counted.
func (s *frameStats) restart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts.last = time.Time{}
}

// take returns the counters as log attributes and resets them.
func (s *frameStats) take() []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.counts
	avg := 0
	if f.frames > 0 {
		avg = f.bytes / f.frames
	}
	attrs := []any{
		slog.Group("host",
			"frames", f.frames,
			"keyframes", f.keyframes,
			"avg_bytes", avg,
			"max_bytes", f.maxBytes,
			"max_keyframe_bytes", f.maxKey,
			"max_gap_ms", f.maxGap.Milliseconds(),
		),
	}
	s.counts = frameCounts{last: f.last}
	return attrs
}

// clientStats is the browser's periodic "stats" report. Delay fields are
// per-frame averages in milliseconds over the report interval.
type clientStats struct {
	IntervalMs       float64 `json:"interval_ms"`
	FPS              float64 `json:"fps"`
	JitterBuffer     float64 `json:"jitter_buffer_ms"`
	Decode           float64 `json:"decode_ms"`
	Processing       float64 `json:"processing_ms"`
	ReceiveToPresent float64 `json:"receive_to_present_ms"`
	RTT              float64 `json:"rtt_ms"`
	Kbps             float64 `json:"kbps"`
	Dropped          float64 `json:"frames_dropped"`
	Freezes          float64 `json:"freezes"`
	PacketsLost      float64 `json:"packets_lost"`
	Keyframes        float64 `json:"keyframes"`
}

func (c clientStats) attrs() []any {
	return []any{
		slog.Group("client",
			"interval_ms", c.IntervalMs,
			"fps", c.FPS,
			"jitter_buffer_ms", c.JitterBuffer,
			"decode_ms", c.Decode,
			"processing_ms", c.Processing,
			"receive_to_present_ms", c.ReceiveToPresent,
			"rtt_ms", c.RTT,
			"kbps", c.Kbps,
			"frames_dropped", c.Dropped,
			"freezes", c.Freezes,
			"packets_lost", c.PacketsLost,
			"keyframes", c.Keyframes,
		),
	}
}
