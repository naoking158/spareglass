package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/vdisplay"
)

// fakeProvider records Open calls instead of creating real displays.
type fakeProvider struct{ opened chan vdisplay.Spec }

func (f *fakeProvider) Name() string     { return "fake" }
func (f *fakeProvider) Available() error { return nil }
func (f *fakeProvider) CloseAll() error  { return nil }
func (f *fakeProvider) Open(s vdisplay.Spec) (vdisplay.Display, error) {
	f.opened <- s
	return fakeDisplay{s}, nil
}

type fakeDisplay struct{ s vdisplay.Spec }

func (d fakeDisplay) ID() string          { return "fake-1" }
func (d fakeDisplay) Spec() vdisplay.Spec { return d.s }
func (d fakeDisplay) Close() error        { return nil }

func newTestServer(t *testing.T, p vdisplay.Provider) (*httptest.Server, *Server) {
	t.Helper()
	cfg := config.Default()
	s := New(Options{Config: cfg, Token: "test-token-0123456789", Provider: p, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Close() })
	return ts, s
}

func TestSessionRequiresToken(t *testing.T) {
	ts, _ := newTestServer(t, &fakeProvider{opened: make(chan vdisplay.Spec, 1)})
	res, err := http.Post(ts.URL+"/api/session", "application/json", bytes.NewBufferString(`{"sdp":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func TestServesClient(t *testing.T) {
	ts, _ := newTestServer(t, &fakeProvider{opened: make(chan vdisplay.Spec, 1)})
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !bytes.Contains(b, []byte("app.js")) {
		t.Fatalf("unexpected index: %d", res.StatusCode)
	}
}

// TestNegotiateAndHello plays the browser's part with a Pion peer:
// offer → answer over HTTP, then a "hello" on the control channel must
// make the host open a virtual display at the client's size.
func TestNegotiateAndHello(t *testing.T) {
	fp := &fakeProvider{opened: make(chan vdisplay.Spec, 1)}
	ts, _ := newTestServer(t, fp)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		t.Fatal(err)
	}
	dc, err := pc.CreateDataChannel("control", nil)
	if err != nil {
		t.Fatal(err)
	}
	status := make(chan statusMsg, 4)
	dc.OnOpen(func() {
		b, _ := json.Marshal(controlMsg{Type: "hello", Width: 1180, Height: 820, DPR: 2})
		_ = dc.SendText(string(b))
	})
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		var s statusMsg
		_ = json.Unmarshal(m.Data, &s)
		status <- s
	})

	offer, _ := pc.CreateOffer(nil)
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-gathered

	body, _ := json.Marshal(sessionRequest{SDP: pc.LocalDescription().SDP})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/session", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token-0123456789")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d: %s", res.StatusCode, b)
	}
	var sr sessionResponse
	if err := json.NewDecoder(res.Body).Decode(&sr); err != nil {
		t.Fatal(err)
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sr.SDP}); err != nil {
		t.Fatal(err)
	}

	select {
	case spec := <-fp.opened:
		if spec.Width != 1180 || spec.Height != 820 {
			t.Fatalf("display size = %dx%d, want 1180x820", spec.Width, spec.Height)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("host never opened a virtual display")
	}
	select {
	case s := <-status:
		if s.State != "display-only" {
			t.Fatalf("status = %q, want display-only (pipeline not implemented)", s.State)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no status from host")
	}
}
