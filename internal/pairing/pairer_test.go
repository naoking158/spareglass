package pairing

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPairerOneLiveRequestPerAddress(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "devices.json"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := NewPairer(ctx, s, ApproverFunc(func(ctx context.Context, _ *Request) bool {
		<-ctx.Done()
		return false
	}))

	if _, err := p.Start("owner A", "192.168.0.2", 1); err != nil {
		t.Fatal(err)
	}
	first, err := p.Start("flooder", "192.168.0.66", 1)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		again, err := p.Start("flooder", "192.168.0.66", 1)
		if err != nil {
			t.Fatalf("repeat from the same address: %v", err)
		}
		if again.ID != first.ID || again.Code != first.Code {
			t.Fatal("repeat from the same address should return its waiting request")
		}
	}
	if _, err := p.Start("owner B", "192.168.0.3", 1); err != nil {
		t.Fatalf("another device must still be able to ask: %v", err)
	}
}
