package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
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
		if spec.Width != 1180 || spec.Height != 820 || !spec.HiDPI {
			t.Fatalf("display = %dx%d HiDPI=%v, want 1180x820 HiDPI for a dpr 2 client", spec.Width, spec.Height, spec.HiDPI)
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
// ffmpeg starts slowly on a busy machine, so the wait is generous.
func TestStreamsVideo(t *testing.T) {
	if _, err := media.FFmpegPath(); err != nil {
		t.Skip("ffmpeg not installed")
	}
	fp := &fakeProvider{opened: make(chan vdisplay.Spec, 1)}
	pattern := func(_ media.Display, fps, _ int) (media.Pipeline, error) {
		return media.NewFFmpegPipeline(media.TestPatternArgs(320, 240, fps), fps)
	}
	streamOverWebRTC(t, fp, pattern, 640, 480, 45*time.Second)
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
	r := receiveVideo(t, nil, p, f, width, height, 30, wait)
	if r.mime != webrtc.MimeTypeH264 {
		t.Fatalf("codec = %s, want H264", r.mime)
	}
	sawStart := false
	for _, pkt := range r.pkts {
		if len(pkt.Payload) > 0 {
			typ := pkt.Payload[0] & 0x1f
			if typ == 7 || typ == 24 || typ == 28 || typ == 5 {
				sawStart = true
			}
		}
	}
	if !sawStart {
		t.Fatalf("received %d packets, no keyframe/SPS", len(r.pkts))
	}
}

// received is what receiveVideo saw on the video track.
type received struct {
	mime string
	exts []webrtc.RTPHeaderExtensionParameter
	pkts []*rtp.Packet
}

// receiveVideo connects a browser-like peer (built from api, or Pion's
// defaults when nil), says hello at width x height and returns the first n
// RTP packets of the video track.
func receiveVideo(t *testing.T, api *webrtc.API, p vdisplay.Provider, f media.Factory, width, height, n int, wait time.Duration) received {
	t.Helper()
	ts := newTestServer(t, p, true, f)
	auth := pairDevice(t, ts.URL)

	if api == nil {
		api = webrtc.NewAPI()
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
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

	done := make(chan received, 1)
	pc.OnTrack(func(tr *webrtc.TrackRemote, rcv *webrtc.RTPReceiver) {
		r := received{mime: tr.Codec().MimeType, exts: rcv.GetParameters().HeaderExtensions}
		for len(r.pkts) < n {
			pkt, _, err := tr.ReadRTP()
			if err != nil {
				break
			}
			r.pkts = append(r.pkts, pkt)
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
		if len(r.pkts) < n {
			t.Fatalf("received %d packets, want %d", len(r.pkts), n)
		}
		return r
	case <-time.After(wait):
		t.Fatal("no video received")
	}
	return received{}
}

// timedPipeline emits one-NAL access units whose second byte is the frame
// index, with Durations cycling through durations.
type timedPipeline struct {
	durations []time.Duration
	stop      chan struct{}
	once      sync.Once
}

func (p *timedPipeline) Run(ctx context.Context, out func(media.EncodedFrame) error) error {
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return nil
		case <-p.stop:
			return nil
		case <-time.After(5 * time.Millisecond):
		}
		f := media.EncodedFrame{Data: []byte{0, 0, 0, 1, 0x65, byte(i)}, Duration: p.durations[i%len(p.durations)], Keyframe: true}
		if err := out(f); err != nil {
			return err
		}
	}
}
func (p *timedPipeline) RequestKeyframe() {}
func (p *timedPipeline) Close() error     { p.once.Do(func() { close(p.stop) }); return nil }

// TestRTPTimestampsFollowFrameTimes checks that a frame's RTP timestamp is
// its own time: it is the previous frame's timestamp plus this frame's
// Duration (the time since the previous frame), not the previous one's.
func TestRTPTimestampsFollowFrameTimes(t *testing.T) {
	// Four entries, so the cycle stays aligned when the one-byte index wraps.
	durations := []time.Duration{10 * time.Millisecond, 40 * time.Millisecond, 25 * time.Millisecond, 15 * time.Millisecond}
	factory := func(media.Display, int, int) (media.Pipeline, error) {
		return &timedPipeline{durations: durations, stop: make(chan struct{})}, nil
	}
	pkts := receiveVideo(t, nil, &fakeProvider{opened: make(chan vdisplay.Spec, 1)}, factory, 640, 480, 20, 15*time.Second).pkts
	compared := 0
	for i := 1; i < len(pkts); i++ {
		prev, cur := pkts[i-1], pkts[i]
		if byte(cur.Payload[1]) != byte(prev.Payload[1])+1 {
			continue
		}
		compared++
		want := uint32(durations[int(cur.Payload[1])%len(durations)] * videoClockRate / time.Second)
		if got := cur.Timestamp - prev.Timestamp; got != want {
			t.Fatalf("frame %d: timestamp advanced %d ticks, want %d (its own Duration)", cur.Payload[1], got, want)
		}
	}
	if compared < 5 {
		t.Fatalf("only %d consecutive frames received", compared)
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
	m := NewManager(config.Default(), rp, func(media.Display, int, int) (media.Pipeline, error) { return nil, media.ErrNotImplemented }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	m := NewManager(config.Default(), up, func(media.Display, int, int) (media.Pipeline, error) { return nil, media.ErrNotImplemented }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := &Session{m: m, slot: 0, log: m.log}
	s.ensureDisplay(controlMsg{Type: "hello", Width: 3840, Height: 2160, DPR: 1})
	if up.calls != 1 {
		t.Fatalf("Open called %d times, want 1", up.calls)
	}
}

// macOS screen capture shears the picture when the display width is not a
// multiple of 4, so every size handed to the provider must be aligned.
func TestDisplaySizesAreAligned(t *testing.T) {
	for _, size := range [][2]float64{{1010, 450}, {1011, 451}, {3846, 1603}, {1603, 3846}} {
		rp := &refusingProvider{fakeProvider{opened: make(chan vdisplay.Spec, 1)}}
		m := NewManager(config.Default(), rp, func(media.Display, int, int) (media.Pipeline, error) { return nil, media.ErrNotImplemented }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
		s := &Session{m: m, slot: 0, log: m.log}
		s.ensureDisplay(controlMsg{Type: "hello", Width: size[0], Height: size[1], DPR: 1})
		select {
		case spec := <-rp.opened:
			if spec.Width%4 != 0 || spec.Height%2 != 0 {
				t.Errorf("%vx%v: opened %dx%d, want width a multiple of 4 and even height", size[0], size[1], spec.Width, spec.Height)
			}
		default:
			t.Errorf("%vx%v: no display opened", size[0], size[1])
		}
	}
}

// A resize to a size that aligns to the open display must not recreate it.
func TestResizeToSameAlignedSizeKeepsDisplay(t *testing.T) {
	fp := &fakeProvider{opened: make(chan vdisplay.Spec, 2)}
	m := NewManager(config.Default(), fp, func(media.Display, int, int) (media.Pipeline, error) { return nil, media.ErrNotImplemented }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := &Session{m: m, slot: 0, log: m.log}
	s.ensureDisplay(controlMsg{Type: "hello", Width: 1181, Height: 821, DPR: 1})
	s.ensureDisplay(controlMsg{Type: "resize", Width: 1182, Height: 820, DPR: 1})
	if n := len(fp.opened); n != 1 {
		t.Fatalf("Open called %d times, want 1", n)
	}
}

// shortRefusingProvider rejects displays shorter than 450 pixels, as macOS
// does for landscape virtual displays.
type shortRefusingProvider struct{ fakeProvider }

func (r *shortRefusingProvider) Open(s vdisplay.Spec) (vdisplay.Display, error) {
	if s.Height < 450 {
		return nil, fmt.Errorf("came up at a size other than the one requested")
	}
	return r.fakeProvider.Open(s)
}

func TestShortLandscapeScreenGetsDisplay(t *testing.T) {
	rp := &shortRefusingProvider{fakeProvider{opened: make(chan vdisplay.Spec, 1)}}
	m := NewManager(config.Default(), rp, func(media.Display, int, int) (media.Pipeline, error) { return nil, media.ErrNotImplemented }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := &Session{m: m, slot: 0, log: m.log}
	s.ensureDisplay(controlMsg{Type: "hello", Width: 915, Height: 412, DPR: 1})
	select {
	case spec := <-rp.opened:
		if got, want := float64(spec.Width)/float64(spec.Height), 915.0/412.0; math.Abs(got-want) > 0.01 {
			t.Fatalf("opened %dx%d, aspect %.3f, want %.3f", spec.Width, spec.Height, got, want)
		}
	default:
		t.Fatal("no display opened for a 915x412 screen")
	}
}

func TestFallbackSizesStayTallEnough(t *testing.T) {
	for _, size := range fallbackSizes(2000, 420) {
		if size[1] < minLandscapeHeight {
			t.Errorf("fallbackSizes(2000, 420) offers %v, which macOS refuses", size)
		}
	}
}

// TestZeroPlayoutDelay checks that a browser offering the playout-delay
// extension gets min=max=0 on every packet.
func TestZeroPlayoutDelay(t *testing.T) {
	api, err := newAPI()
	if err != nil {
		t.Fatal(err)
	}
	factory := func(media.Display, int, int) (media.Pipeline, error) {
		return &timedPipeline{durations: []time.Duration{16 * time.Millisecond}, stop: make(chan struct{})}, nil
	}
	r := receiveVideo(t, api, &fakeProvider{opened: make(chan vdisplay.Spec, 1)}, factory, 640, 480, 5, 15*time.Second)
	var id uint8
	for _, e := range r.exts {
		if e.URI == playoutDelayURI {
			id = uint8(e.ID)
		}
	}
	if id == 0 {
		t.Fatal("playout-delay was not negotiated")
	}
	for _, pkt := range r.pkts {
		var pd rtp.PlayoutDelayExtension
		if err := pd.Unmarshal(pkt.Header.GetExtension(id)); err != nil || pd.MinDelay != 0 || pd.MaxDelay != 0 {
			t.Fatalf("packet %d: playout-delay %+v (err %v), want min=max=0", pkt.SequenceNumber, pd, err)
		}
	}
}

func TestWantsHiDPI(t *testing.T) {
	for _, c := range []struct {
		w, h       int
		dpr, scale float64
		want       bool
	}{
		{1708, 1112, 2, 1, true},
		{1708, 1112, 1, 1, false},
		{1708, 1112, 2, 2, false},
		{2560, 1440, 2, 1, false}, // 5120x2880 is more than VideoToolbox's H.264 encoder takes
	} {
		if got := wantsHiDPI(c.w, c.h, c.dpr, c.scale); got != c.want {
			t.Errorf("wantsHiDPI(%d, %d, dpr %g, scale %g) = %v, want %v", c.w, c.h, c.dpr, c.scale, got, c.want)
		}
	}
}
