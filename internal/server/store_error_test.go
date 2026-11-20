package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/pairing"
	"github.com/naoking158/spareglass/internal/vdisplay"
)

// A store that cannot be read must not look like "this browser is not paired":
// on 401 the client throws its credential away.
func TestSessionWithUnreadableStoreIsNotUnauthorized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	store, err := pairing.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id, secret, err := store.Add("Test on iPad")
	if err != nil {
		t.Fatal(err)
	}
	good, _ := os.ReadFile(path)
	ctx, cancel := context.WithCancel(context.Background())
	pairer := pairing.NewPairer(ctx, store, pairing.ApproverFunc(func(context.Context, *pairing.Request) bool { return false }))
	s := New(Options{Config: config.Default(), Pairer: pairer, Provider: &fakeProvider{opened: make(chan vdisplay.Spec, 1)}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Close(); cancel() })

	write := func(b []byte, at time.Time) {
		t.Helper()
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	auth := "Device " + id + ":" + secret

	write([]byte("{not json"), time.Now().Add(time.Hour))
	if res := postJSON(t, ts.URL+"/api/session", auth, sessionRequest{Slot: 1}); res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unreadable store: status %d, want 503", res.StatusCode)
	}

	write(good, time.Now().Add(2*time.Hour))
	if res := postJSON(t, ts.URL+"/api/session", auth, sessionRequest{Slot: 1}); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("repaired store: status %d, want 400 (authenticated, empty SDP)", res.StatusCode)
	}
}
