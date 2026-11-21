package server

import (
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"

	"github.com/naoking158/spareglass/internal/media"
)

const (
	videoClockRate = 90000
	rtpMTU         = 1200
)

// frameWriter sends H.264 access units on an RTP track. Each frame's RTP
// timestamp is advanced by its Duration (time since the previous frame)
// before the frame is packetized, so the timestamp is the frame's own time
// and the browser sees no jitter that the host did not have.
type frameWriter struct {
	track *webrtc.TrackLocalStaticRTP

	mu      sync.Mutex
	pkt     rtp.Packetizer
	elapsed time.Duration
	ticks   uint64
}

func newFrameWriter(track *webrtc.TrackLocalStaticRTP) *frameWriter {
	return &frameWriter{
		track: track,
		pkt:   rtp.NewPacketizer(rtpMTU, 0, 0, &codecs.H264Payloader{}, rtp.NewRandomSequencer(), videoClockRate),
	}
}

func (w *frameWriter) write(f media.EncodedFrame) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Ticks come from the total elapsed time, so rounding does not drift.
	w.elapsed += f.Duration
	ticks := uint64(w.elapsed/time.Second)*videoClockRate + uint64(w.elapsed%time.Second)*videoClockRate/uint64(time.Second)
	w.pkt.SkipSamples(uint32(ticks - w.ticks))
	w.ticks = ticks
	for _, p := range w.pkt.Packetize(f.Data, 0) {
		if err := w.track.WriteRTP(p); err != nil {
			return err
		}
	}
	return nil
}
