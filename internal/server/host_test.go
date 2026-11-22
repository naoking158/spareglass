package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/pairing"
	"github.com/naoking158/spareglass/internal/vdisplay"
)

const testToken = "t0ken"

// newHostTestServer starts the LAN handler and the host handler. Pairing
// requests wait until they are answered through the host API.
func newHostTestServer(t *testing.T) (lan, host *httptest.Server, stopped chan struct{}) {
	t.Helper()
	store, err := pairing.OpenStore(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	pairer := pairing.NewPairer(ctx, store, pairing.ApproverFunc(func(ctx context.Context, _ *pairing.Request) bool {
		<-ctx.Done()
		return false
	}))
	s := New(Options{Config: config.Default(), Pairer: pairer, Provider: &fakeProvider{opened: make(chan vdisplay.Spec, 1)}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	stopped = make(chan struct{})
	lan = httptest.NewServer(s.Handler())
	host = httptest.NewServer(s.HostHandler(HostOptions{Token: testToken, URLs: func() []string { return []string{"http://192.168.1.20/1"} }, Stop: func() { close(stopped) }}))
	t.Cleanup(func() { lan.Close(); host.Close(); s.Close(); cancel() })
	return lan, host, stopped
}

func hostDo(t *testing.T, method, url string, body any, edit func(*http.Request)) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set(TokenHeader, testToken)
	if edit != nil {
		edit(req)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func hostStateOf(t *testing.T, base string) hostState {
	t.Helper()
	res := hostDo(t, http.MethodGet, base+"/api/state", nil, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/state = %d", res.StatusCode)
	}
	var st hostState
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestHostAPIRejectsForeignRequests(t *testing.T) {
	lan, host, stopped := newHostTestServer(t)
	cases := map[string]func(*http.Request){
		"no token":        func(r *http.Request) { r.Header.Del(TokenHeader) },
		"wrong token":     func(r *http.Request) { r.Header.Set(TokenHeader, "guess") },
		"rebound Host":    func(r *http.Request) { r.Host = "evil.example:80" },
		"LAN Host":        func(r *http.Request) { r.Host = "192.168.1.20" },
		"cross-site POST": func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") },
	}
	for name, edit := range cases {
		for _, call := range []struct{ method, path string }{{http.MethodGet, "/api/state"}, {http.MethodPost, "/api/stop"}} {
			if res := hostDo(t, call.method, host.URL+call.path, nil, edit); res.StatusCode != http.StatusForbidden {
				t.Errorf("%s: %s %s = %d, want 403", name, call.method, call.path, res.StatusCode)
			}
		}
	}
	if res := hostDo(t, http.MethodPost, host.URL+"/api/pair/x", map[string]bool{}, nil); res.StatusCode != http.StatusBadRequest {
		t.Errorf("an answer without allow = %d, want 400", res.StatusCode)
	}
	s := New(Options{Config: config.Default(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/stop", nil)
	req.RemoteAddr = "192.0.2.1:5000"
	req.Header.Set(TokenHeader, testToken)
	rec := httptest.NewRecorder()
	s.HostHandler(HostOptions{Token: testToken, URLs: func() []string { return nil }, Stop: func() { t.Error("a LAN peer stopped the host") }}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a request from the LAN = %d, want 403", rec.Code)
	}
	select {
	case <-stopped:
		t.Fatal("a rejected request stopped the host")
	default:
	}
	for path, want := range map[string]int{"/": 200, "/host.js": 200, "/host.css": 200, "/qr.png": 200} {
		if res := hostDo(t, http.MethodGet, host.URL+path, nil, func(r *http.Request) { r.Header.Del(TokenHeader) }); res.StatusCode != want {
			t.Errorf("host page GET %s without token = %d, want %d", path, res.StatusCode, want)
		}
	}
	for _, path := range []string{"/api/state", "/api/stop", "/api/devices/x", "/host.js", "/host.css", "/qr.png"} {
		if res := hostDo(t, http.MethodGet, lan.URL+path, nil, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("LAN handler serves %s: %d, want 404", path, res.StatusCode)
		}
	}
}

func TestHostAPIPairForgetStop(t *testing.T) {
	lan, host, stopped := newHostTestServer(t)

	res := postJSON(t, lan.URL+"/api/pair", "", pairRequest{Name: "Safari on iPad", Slot: 2})
	var start pairStartResponse
	_ = json.NewDecoder(res.Body).Decode(&start)

	st := hostStateOf(t, host.URL)
	if len(st.Pending) != 1 || st.Pending[0].Code != start.Code || st.Pending[0].Slot != 2 || st.Pending[0].Name != "Safari on iPad" {
		t.Fatalf("pending = %+v, want the iPad request with code %s", st.Pending, start.Code)
	}
	if len(st.URLs) != 1 || len(st.Devices) != 0 {
		t.Fatalf("state = %+v", st)
	}

	if res := hostDo(t, http.MethodPost, host.URL+"/api/pair/"+start.ID, map[string]bool{"allow": true}, func(r *http.Request) {
		r.Header.Set("Origin", host.URL)
	}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("allow = %d", res.StatusCode)
	}
	var auth string
	deadline := time.Now().Add(2 * time.Second)
	for auth == "" && time.Now().Before(deadline) {
		r, _ := http.Get(lan.URL + "/api/pair/" + start.ID)
		var ps pairStatusResponse
		_ = json.NewDecoder(r.Body).Decode(&ps)
		r.Body.Close()
		if ps.Status == pairing.StateApproved {
			auth = "Device " + ps.DeviceID + ":" + ps.Secret
		}
		time.Sleep(10 * time.Millisecond)
	}
	if auth == "" {
		t.Fatal("the browser never learned it was approved")
	}

	st = hostStateOf(t, host.URL)
	if len(st.Pending) != 0 || len(st.Devices) != 1 {
		t.Fatalf("after approval: %+v", st)
	}
	if res := postJSON(t, lan.URL+"/api/session", auth, sessionRequest{SDP: "x", Slot: 1}); res.StatusCode == http.StatusUnauthorized {
		t.Fatal("an approved browser must pass the session auth check")
	}

	devID := strings.TrimPrefix(strings.SplitN(auth, ":", 2)[0], "Device ")
	if res := hostDo(t, http.MethodDelete, host.URL+"/api/devices/"+devID, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("forget = %d", res.StatusCode)
	}
	if res := postJSON(t, lan.URL+"/api/session", auth, sessionRequest{SDP: "x", Slot: 1}); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a forgotten browser got %d, want 401", res.StatusCode)
	}
	if res := hostDo(t, http.MethodDelete, host.URL+"/api/devices/"+devID, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("forgetting twice = %d, want 404", res.StatusCode)
	}

	if res := hostDo(t, http.MethodPost, host.URL+"/api/stop", nil, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("stop = %d", res.StatusCode)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not call Stop")
	}
}
