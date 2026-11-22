package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/media"
	"github.com/naoking158/spareglass/internal/pairing"
	"github.com/naoking158/spareglass/internal/vdisplay"
)

// TestSendsCursor checks that a client with a "cursor" channel gets the
// pointer position on its display, a hidden pointer once it leaves, and
// sequence numbers that keep growing when the display is recreated.
func TestSendsCursor(t *testing.T) {
	var onDisplay atomic.Bool
	onDisplay.Store(true)
	var askedFor atomic.Value
	cursor := func(id string) (media.Cursor, error) {
		askedFor.Store(id)
		return media.Cursor{X: 12.5, Y: 34, OnDisplay: onDisplay.Load()}, nil
	}
	store, err := pairing.OpenStore(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	pairer := pairing.NewPairer(ctx, store, pairing.ApproverFunc(func(context.Context, *pairing.Request) bool { return true }))
	factory := func(media.Display, int, int) (media.Pipeline, error) {
		return &timedPipeline{durations: []time.Duration{time.Second}, stop: make(chan struct{})}, nil
	}
	srv := New(Options{Config: config.Default(), Pairer: pairer, Provider: &fakeProvider{opened: make(chan vdisplay.Spec, 4)},
		NewPipeline: factory, Cursor: cursor, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close(); cancel() })
	auth := pairDevice(t, ts.URL)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	_, _ = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly})
	control, _ := pc.CreateDataChannel("control", nil)
	control.OnOpen(func() {
		b, _ := json.Marshal(controlMsg{Type: "hello", Width: 640, Height: 480, DPR: 1})
		_ = control.SendText(string(b))
	})
	ordered, retransmits := false, uint16(0)
	cursorDC, _ := pc.CreateDataChannel("cursor", &webrtc.DataChannelInit{Ordered: &ordered, MaxRetransmits: &retransmits})
	got := make(chan cursorMsg, 64)
	cursorDC.OnMessage(func(m webrtc.DataChannelMessage) {
		var c cursorMsg
		if err := json.Unmarshal(m.Data, &c); err != nil {
			t.Errorf("bad cursor message %q: %v", m.Data, err)
			return
		}
		select {
		case got <- c:
		default:
		}
	})

	offer, _ := pc.CreateOffer(nil)
	gathered := webrtc.GatheringCompletePromise(pc)
	_ = pc.SetLocalDescription(offer)
	<-gathered
	res := postJSON(t, ts.URL+"/api/session", auth, sessionRequest{SDP: pc.LocalDescription().SDP, Slot: 1})
	if res.StatusCode != 200 {
		t.Fatalf("session: %d", res.StatusCode)
	}
	var sr sessionResponse
	_ = json.NewDecoder(res.Body).Decode(&sr)
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sr.SDP}); err != nil {
		t.Fatal(err)
	}

	wait := func(want func(cursorMsg) bool) cursorMsg {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case c := <-got:
				if want(c) {
					return c
				}
			case <-deadline:
				t.Fatal("no matching cursor message")
			}
		}
	}
	first := wait(func(c cursorMsg) bool { return c.Visible })
	if first.X != 12.5 || first.Y != 34 {
		t.Fatalf("cursor at %v,%v, want 12.5,34", first.X, first.Y)
	}
	if id, _ := askedFor.Load().(string); id != "fake-1" {
		t.Fatalf("cursor read for display %q, want fake-1", id)
	}
	onDisplay.Store(false)
	hidden := wait(func(c cursorMsg) bool { return !c.Visible })
	if hidden.Seq <= first.Seq {
		t.Fatalf("hidden seq %d not after visible seq %d", hidden.Seq, first.Seq)
	}

	b, _ := json.Marshal(controlMsg{Type: "resize", Width: 800, Height: 600, DPR: 1})
	_ = control.SendText(string(b))
	onDisplay.Store(true)
	again := wait(func(c cursorMsg) bool { return c.Visible })
	if again.Seq <= hidden.Seq {
		t.Fatalf("seq after display recreation = %d, want > %d", again.Seq, hidden.Seq)
	}
}
