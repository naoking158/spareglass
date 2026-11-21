package server

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"

	"github.com/naoking158/spareglass/internal/media"
)

const (
	videoClockRate = 90000
	rtpMTU         = 1200
	// playoutDelayURI is the RTP header extension that tells the browser how
	// long it may hold frames before showing them.
	playoutDelayURI = "http://www.webrtc.org/experiments/rtp-hdrext/playout-delay"
)

// zeroPlayoutDelay asks the browser to show each frame as soon as it is decoded.
var zeroPlayoutDelay, _ = rtp.PlayoutDelayExtension{}.Marshal()

// newAPI builds a Pion API that can negotiate the playout-delay extension.
func newAPI() (*webrtc.API, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: playoutDelayURI}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, ir); err != nil {
		return nil, err
	}
	return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(ir)), nil
}

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
	// playoutDelayID is the negotiated extension id, 0 if the browser did not offer it.
	playoutDelayID uint8
}

func (w *frameWriter) setPlayoutDelayID(id uint8) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.playoutDelayID = id
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
		if w.playoutDelayID != 0 {
			if err := p.Header.SetExtension(w.playoutDelayID, zeroPlayoutDelay); err != nil {
				return err
			}
		}
		if err := w.track.WriteRTP(p); err != nil {
			return err
		}
	}
	return nil
}
