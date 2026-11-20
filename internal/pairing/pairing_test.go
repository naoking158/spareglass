package pairing

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreAddVerifyRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id, secret, err := s.Add("Safari on iPad")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Verify(id, secret); !ok {
		t.Fatal("fresh device should verify")
	}
	if _, ok, _ := s.Verify(id, secret+"x"); ok {
		t.Fatal("wrong secret should not verify")
	}

	// A second store (like the CLI) revokes; the first notices.
	other, _ := OpenStore(path)
	if err := other.Remove(id); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Verify(id, secret); ok {
		t.Fatal("revoked device should not verify")
	}
}

func TestParseAuthorization(t *testing.T) {
	id, sec, ok := ParseAuthorization("Device abc:def")
	if !ok || id != "abc" || sec != "def" {
		t.Fatalf("got %q %q %v", id, sec, ok)
	}
	for _, h := range []string{"", "Bearer x", "Device abc", "Device :x"} {
		if _, _, ok := ParseAuthorization(h); ok {
			t.Errorf("%q should not parse", h)
		}
	}
}

func TestPairerApproveAndDeny(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "devices.json"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := NewPairer(ctx, s, ApproverFunc(func(_ context.Context, r *Request) bool {
		return strings.Contains(r.Name, "good")
	}))

	good, _ := p.Start("good browser", "192.168.0.2", 1)
	bad, _ := p.Start("bad browser", "192.168.0.3", 1)

	waitFor := func(id string, want State) (string, string) {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			st, dev, sec, _ := p.Status(id)
			if st == want {
				return dev, sec
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("request %s never reached %s", id, want)
		return "", ""
	}
	dev, sec := waitFor(good.ID, StateApproved)
	if _, ok, _ := s.Verify(dev, sec); !ok {
		t.Fatal("approved credentials should verify")
	}
	if _, _, again, _ := p.Status(good.ID); again != "" {
		t.Fatal("secret must be handed out only once")
	}
	waitFor(bad.ID, StateDenied)
}

func TestTerminalApprover(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	var out strings.Builder
	a := NewTerminalApprover(pr, &out)
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = pw.Write([]byte("maybe\ny\n"))
	}()
	if !a.Ask(context.Background(), &Request{Name: "Chrome on Android", Code: "0420", Slot: 1}) {
		t.Fatal("expected approval")
	}
	if !strings.Contains(out.String(), "0420") {
		t.Fatal("prompt should show the code")
	}
}
