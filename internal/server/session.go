package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/media"
	"github.com/naoking158/spareglass/internal/vdisplay"
)

// ErrSlotInUse is returned when another browser already shows that display.
var ErrSlotInUse = errors.New("that display is already in use by another browser")

// Manager owns the WebRTC sessions, one per connected display.
type Manager struct {
	cfg      config.Config
	provider vdisplay.Provider
	log      *slog.Logger

	mu       sync.Mutex
	sessions map[string]*Session
	slots    []bool
}

// NewManager builds a Manager.
func NewManager(cfg config.Config, p vdisplay.Provider, log *slog.Logger) *Manager {
	return &Manager{cfg: cfg, provider: p, log: log, sessions: map[string]*Session{}, slots: make([]bool, cfg.MaxClients)}
}

// Session is one connected browser and its virtual display.
type Session struct {
	id    string
	slot  int
	m     *Manager
	pc    *webrtc.PeerConnection
	track *webrtc.TrackLocalStaticSample
	log   *slog.Logger

	mu       sync.Mutex
	dc       *webrtc.DataChannel
	display  vdisplay.Display
	pipeline media.Pipeline
	closed   bool
}

// Create answers a browser offer for display slot (0-based) and returns the
// session id and answer SDP.
func (m *Manager) Create(ctx context.Context, offer string, slot int, device string) (string, string, error) {
	if !m.takeSlot(slot) {
		return "", "", ErrSlotInUse
	}
	id := newID()
	s := &Session{id: id, slot: slot, m: m, log: m.log.With("session", id, "display", slot+1, "device", device)}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{}) // LAN only: no STUN/TURN
	if err != nil {
		m.freeSlot(slot)
		return "", "", err
	}
	s.pc = pc

	fail := func(err error) (string, string, error) {
		s.Close()
		return "", "", err
	}

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer}); err != nil {
		return fail(fmt.Errorf("set offer: %w", err))
	}

	s.track, err = webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}, "video", "spareglass")
	if err != nil {
		return fail(err)
	}
	sender, err := pc.AddTrack(s.track)
	if err != nil {
		return fail(fmt.Errorf("add video track: %w", err))
	}
	go s.readRTCP(sender)

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "control" {
			return
		}
		s.mu.Lock()
		s.dc = dc
		s.mu.Unlock()
		dc.OnMessage(func(msg webrtc.DataChannelMessage) { s.handleControl(msg.Data) })
		dc.OnClose(func() { s.Close() })
	})
	pc.OnConnectionStateChange(func(st webrtc.PeerConnectionState) {
		s.log.Info("connection state", "state", st.String())
		if st == webrtc.PeerConnectionStateFailed || st == webrtc.PeerConnectionStateClosed {
			s.Close()
		}
	})

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return fail(fmt.Errorf("create answer: %w", err))
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		return fail(err)
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		return fail(ctx.Err())
	case <-time.After(5 * time.Second):
		return fail(errors.New("ICE gathering timed out"))
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	s.log.Info("session created")
	return id, pc.LocalDescription().SDP, nil
}

// CloseAll closes every session.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	for _, s := range all {
		s.Close()
	}
}

func (m *Manager) takeSlot(i int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.slots) || m.slots[i] {
		return false
	}
	m.slots[i] = true
	return true
}

func (m *Manager) freeSlot(i int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.slots[i] = false
}

// controlMsg is what the browser sends on the "control" data channel.
type controlMsg struct {
	Type        string  `json:"type"` // "hello" | "resize"
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	DPR         float64 `json:"dpr"`
	Orientation string  `json:"orientation"`
}

// statusMsg is what the host sends back.
type statusMsg struct {
	Type   string `json:"type"` // "status"
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

func (s *Session) handleControl(data []byte) {
	var msg controlMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		s.log.Warn("bad control message", "err", err)
		return
	}
	switch msg.Type {
	case "hello", "resize":
		s.log.Info("client screen", "type", msg.Type, "css", fmt.Sprintf("%.0fx%.0f", msg.Width, msg.Height), "dpr", msg.DPR, "orientation", msg.Orientation)
		s.ensureDisplay(msg)
	default:
		s.log.Debug("ignored control message", "type", msg.Type)
	}
}

// ensureDisplay (re)creates the virtual display at the client's size.
// It recreates instead of resizing: on macOS a mode change can leave a
// display that cannot be removed until the process exits.
func (s *Session) ensureDisplay(msg controlMsg) {
	scale := s.m.cfg.RenderScale
	w := int(math.Round(msg.Width * scale))
	h := int(math.Round(msg.Height * scale))
	if w < 320 || h < 240 {
		s.send(statusMsg{Type: "status", State: "error", Detail: fmt.Sprintf("screen too small: %dx%d", w, h)})
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if d := s.display; d != nil && d.Spec().Width == w && d.Spec().Height == h {
		s.mu.Unlock()
		return
	}
	s.stopDisplayLocked()
	s.mu.Unlock()

	spec := vdisplay.Spec{
		Name:      fmt.Sprintf("spareglass %d", s.slot+1), // stable per slot
		Width:     w,
		Height:    h,
		RefreshHz: float64(s.m.cfg.FPS),
	}
	d, err := s.m.provider.Open(spec)
	if err != nil {
		s.log.Warn("virtual display unavailable", "err", err)
		s.send(statusMsg{Type: "status", State: "no-display", Detail: err.Error()})
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = d.Close()
		return
	}
	s.display = d
	s.mu.Unlock()
	s.log.Info("virtual display created", "id", d.ID(), "size", fmt.Sprintf("%dx%d", w, h))

	p, err := media.NewPipeline(d.ID(), s.m.cfg.FPS, s.m.cfg.BitrateKbps)
	if err != nil {
		s.send(statusMsg{Type: "status", State: "display-only", Detail: "virtual display created; streaming is not implemented yet", Width: w, Height: h})
		return
	}
	s.mu.Lock()
	s.pipeline = p
	s.mu.Unlock()
	s.send(statusMsg{Type: "status", State: "streaming", Width: w, Height: h})
	go func() {
		err := p.Run(context.Background(), func(f media.EncodedFrame) error {
			return s.track.WriteSample(sampleOf(f))
		})
		if err != nil {
			s.log.Warn("pipeline stopped", "err", err)
		}
	}()
}

func (s *Session) stopDisplayLocked() {
	if s.pipeline != nil {
		_ = s.pipeline.Close()
		s.pipeline = nil
	}
	if s.display != nil {
		if err := s.display.Close(); err != nil {
			s.log.Warn("close virtual display", "err", err)
		}
		s.display = nil
	}
}

// readRTCP drains RTCP and forwards keyframe requests to the encoder.
func (s *Session) readRTCP(sender *webrtc.RTPSender) {
	for {
		pkts, _, err := sender.ReadRTCP()
		if err != nil {
			return
		}
		for _, p := range pkts {
			switch p.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				s.mu.Lock()
				if s.pipeline != nil {
					s.pipeline.RequestKeyframe()
				}
				s.mu.Unlock()
			}
		}
	}
}

func (s *Session) send(v any) {
	s.mu.Lock()
	dc := s.dc
	s.mu.Unlock()
	if dc == nil {
		return
	}
	b, _ := json.Marshal(v)
	if err := dc.SendText(string(b)); err != nil {
		s.log.Debug("send control", "err", err)
	}
}

// Close is idempotent.
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.stopDisplayLocked()
	s.mu.Unlock()

	if s.pc != nil {
		_ = s.pc.Close()
	}
	s.m.mu.Lock()
	delete(s.m.sessions, s.id)
	s.m.slots[s.slot] = false
	s.m.mu.Unlock()
	s.log.Info("session closed")
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
