package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/media"
	"github.com/naoking158/spareglass/internal/pairing"
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

func newTestServer(t *testing.T, p vdisplay.Provider, approve bool, factory ...media.Factory) *httptest.Server {
	t.Helper()
	store, err := pairing.OpenStore(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	pairer := pairing.NewPairer(ctx, store, pairing.ApproverFunc(func(context.Context, *pairing.Request) bool { return approve }))
	cfg := config.Default()
	var f media.Factory
	if len(factory) > 0 {
		f = factory[0]
	}
	s := New(Options{Config: cfg, Pairer: pairer, Provider: p, NewPipeline: f, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Close(); cancel() })
	return ts
}

func postJSON(t *testing.T, url, auth string, v any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(v)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// pairDevice runs the pairing flow and returns an Authorization header value.
func pairDevice(t *testing.T, base string) string {
	t.Helper()
	res := postJSON(t, base+"/api/pair", "", pairRequest{Name: "Test on iPad", Slot: 1})
	if res.StatusCode != 200 {
		t.Fatalf("pair start: %d", res.StatusCode)
	}
	var start pairStartResponse
	_ = json.NewDecoder(res.Body).Decode(&start)
	if len(start.Code) != 4 {
		t.Fatalf("code = %q", start.Code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r, err := http.Get(base + "/api/pair/" + start.ID)
		if err != nil {
			t.Fatal(err)
		}
		var st pairStatusResponse
		_ = json.NewDecoder(r.Body).Decode(&st)
		switch st.Status {
		case pairing.StateApproved:
			return fmt.Sprintf("Device %s:%s", st.DeviceID, st.Secret)
		case pairing.StateDenied, pairing.StateExpired:
			t.Fatalf("pairing ended as %s", st.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("pairing never finished")
	return ""
}

func TestRoutes(t *testing.T) {
	ts := newTestServer(t, &fakeProvider{opened: make(chan vdisplay.Spec, 1)}, true)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cases := map[string]int{"/": 302, "/1": 200, "/2": 200, "/3": 404, "/x": 404, "/app.js": 200, "/style.css": 200}
	for path, want := range cases {
		res, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, res.StatusCode, want)
		}
	}
}

func TestSessionRequiresPairedDevice(t *testing.T) {
	ts := newTestServer(t, &fakeProvider{opened: make(chan vdisplay.Spec, 1)}, true)
	for _, auth := range []string{"", "Device nope:nope", "Bearer x"} {
		res := postJSON(t, ts.URL+"/api/session", auth, sessionRequest{SDP: "x", Slot: 1})
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("auth %q: status = %d, want 401", auth, res.StatusCode)
		}
	}
}

func TestDeniedPairing(t *testing.T) {
	ts := newTestServer(t, &fakeProvider{opened: make(chan vdisplay.Spec, 1)}, false)
	res := postJSON(t, ts.URL+"/api/pair", "", pairRequest{Name: "Stranger", Slot: 1})
	var start pairStartResponse
	_ = json.NewDecoder(res.Body).Decode(&start)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := http.Get(ts.URL + "/api/pair/" + start.ID)
		var st pairStatusResponse
		_ = json.NewDecoder(r.Body).Decode(&st)
		if st.Status == pairing.StateDenied {
			if st.Secret != "" {
				t.Fatal("denied request must not carry a secret")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("request was never denied")
}

// TestNegotiateAndHello plays the browser's part with a Pion peer:
// offer → answer over HTTP, then a "hello" on the control channel must
// make the host open a virtual display at the client's size.
func TestNegotiateAndHello(t *testing.T) {
	fp := &fakeProvider{opened: make(chan vdisplay.Spec, 1)}
	ts := newTestServer(t, fp, true)
	auth := pairDevice(t, ts.URL)

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

	res := postJSON(t, ts.URL+"/api/session", auth, sessionRequest{SDP: pc.LocalDescription().SDP, Slot: 1})
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

	// The same display cannot be taken twice.
	if again := postJSON(t, ts.URL+"/api/session", auth, sessionRequest{SDP: pc.LocalDescription().SDP, Slot: 1}); again.StatusCode != http.StatusConflict {
		t.Fatalf("second session on display 1: %d, want 409", again.StatusCode)
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

// TestStreamsVideo checks the whole media path with ffmpeg's test pattern:
// pipeline → access units → Pion track → RTP at a browser-like peer.
func TestStreamsVideo(t *testing.T) {
	if _, err := media.FFmpegPath(); err != nil {
		t.Skip("ffmpeg not installed")
	}
	fp := &fakeProvider{opened: make(chan vdisplay.Spec, 1)}
	pattern := func(_ string, fps, _ int) (media.Pipeline, error) {
		return media.NewFFmpegPipeline(media.TestPatternArgs(320, 240, fps), fps)
	}
	streamOverWebRTC(t, fp, pattern, 640, 480, 15*time.Second)
}

// TestStreamsVirtualDisplay runs the same path with this OS's real virtual
// display and capture pipeline. It changes the desktop, so it only runs with
// SPAREGLASS_VDD_TEST=1 (CI sets it on a Windows runner with the driver).
func TestStreamsVirtualDisplay(t *testing.T) {
	if os.Getenv("SPAREGLASS_VDD_TEST") == "" {
		t.Skip("set SPAREGLASS_VDD_TEST=1 to create a real virtual display")
	}
	p := vdisplay.Default()
	if err := p.Available(); err != nil {
		t.Fatal(err)
	}
	streamOverWebRTC(t, p, nil, 1180, 820, 60*time.Second)
}

func streamOverWebRTC(t *testing.T, p vdisplay.Provider, f media.Factory, width, height int, wait time.Duration) {
	t.Helper()
	var factories []media.Factory
	if f != nil {
		factories = append(factories, f)
	}
	ts := newTestServer(t, p, true, factories...)
	auth := pairDevice(t, ts.URL)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	_, _ = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly})
	dc, _ := pc.CreateDataChannel("control", nil)
	dc.OnOpen(func() {
		b, _ := json.Marshal(controlMsg{Type: "hello", Width: float64(width), Height: float64(height), DPR: 1})
		_ = dc.SendText(string(b))
	})

	type result struct {
		mime     string
		packets  int
		sawStart bool
	}
	done := make(chan result, 1)
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		r := result{mime: tr.Codec().MimeType}
		for r.packets < 30 {
			pkt, _, err := tr.ReadRTP()
			if err != nil {
				break
			}
			r.packets++
			if len(pkt.Payload) > 0 {
				typ := pkt.Payload[0] & 0x1f
				if typ == 7 || typ == 24 || typ == 28 || typ == 5 {
					r.sawStart = true
				}
			}
		}
		done <- r
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

	select {
	case r := <-done:
		if r.mime != webrtc.MimeTypeH264 {
			t.Fatalf("codec = %s, want H264", r.mime)
		}
		if r.packets < 30 || !r.sawStart {
			t.Fatalf("received %d packets, keyframe/SPS seen: %v", r.packets, r.sawStart)
		}
	case <-time.After(wait):
		t.Fatal("no video received")
	}
}

func TestFallbackSizes(t *testing.T) {
	got := fallbackSizes(3840, 2160)
	want := [][2]int{{3840, 2160}, {2560, 1440}, {1920, 1080}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("4K: got %v, want %v", got, want)
	}
	if got := fallbackSizes(1180, 820); len(got) != 1 {
		t.Fatalf("small screens need no fallback, got %v", got)
	}
	if got := fallbackSizes(1440, 2560); fmt.Sprint(got[len(got)-1]) != "[1080 1920]" {
		t.Fatalf("portrait: got %v", got)
	}
}

// refusingProvider rejects anything larger than 1920x1080.
type refusingProvider struct{ fakeProvider }

func (r *refusingProvider) Open(s vdisplay.Spec) (vdisplay.Display, error) {
	if s.Width > 1920 {
		return nil, fmt.Errorf("never became active")
	}
	return r.fakeProvider.Open(s)
}

func TestFallsBackToSmallerDisplay(t *testing.T) {
	rp := &refusingProvider{fakeProvider{opened: make(chan vdisplay.Spec, 1)}}
	m := NewManager(config.Default(), rp, func(string, int, int) (media.Pipeline, error) { return nil, media.ErrNotImplemented }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := &Session{m: m, slot: 0, log: m.log}
	s.ensureDisplay(controlMsg{Type: "hello", Width: 3840, Height: 2160, DPR: 1})
	select {
	case spec := <-rp.opened:
		if spec.Width != 1920 || spec.Height != 1080 {
			t.Fatalf("opened %dx%d, want 1920x1080", spec.Width, spec.Height)
		}
	default:
		t.Fatal("no display opened")
	}
	// The same size again must not trigger another round of attempts.
	s.ensureDisplay(controlMsg{Type: "resize", Width: 3840, Height: 2160, DPR: 1})
	select {
	case <-rp.opened:
		t.Fatal("repeated request should be ignored")
	default:
	}
}

// unavailableProvider counts Open calls and always reports ErrUnavailable.
type unavailableProvider struct {
	fakeProvider
	calls int
}

func (u *unavailableProvider) Open(vdisplay.Spec) (vdisplay.Display, error) {
	u.calls++
	return nil, fmt.Errorf("%w: driver missing", vdisplay.ErrUnavailable)
}

func TestUnavailableDisplayIsNotRetriedSmaller(t *testing.T) {
	up := &unavailableProvider{}
	m := NewManager(config.Default(), up, func(string, int, int) (media.Pipeline, error) { return nil, media.ErrNotImplemented }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := &Session{m: m, slot: 0, log: m.log}
	s.ensureDisplay(controlMsg{Type: "hello", Width: 3840, Height: 2160, DPR: 1})
	if up.calls != 1 {
		t.Fatalf("Open called %d times, want 1", up.calls)
	}
}
