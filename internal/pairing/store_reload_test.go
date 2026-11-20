package pairing

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreFailsClosedWhenFileBecomesUnreadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id, secret, err := s.Add("Safari on iPad")
	if err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	touch := func(b []byte, at time.Time) {
		t.Helper()
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	touch([]byte("{not json"), time.Now().Add(time.Hour))

	if _, ok, err := s.Verify(id, secret); ok || err == nil {
		t.Fatalf("corrupt store: ok=%v err=%v, want rejection with an error", ok, err)
	}
	if err := s.Remove(id); err == nil {
		t.Fatal("Remove must not rewrite the file from a stale copy")
	}

	touch(good, time.Now().Add(2*time.Hour))
	if _, ok, err := s.Verify(id, secret); !ok || err != nil {
		t.Fatalf("repaired store: ok=%v err=%v, want the device to verify again", ok, err)
	}
	if _, ok, err := s.Verify(id, "wrong"); ok || err != nil {
		t.Fatalf("wrong secret: ok=%v err=%v", ok, err)
	}
}
