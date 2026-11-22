package pairing

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuilder is a strings.Builder safe for the approver goroutine and the test.
type syncBuilder struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuilder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuilder) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitState(t *testing.T, p *Pairer, id string, want State) (string, string) {
	t.Helper()
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

func TestAnswerWhileTerminalAsks(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "devices.json"))
	pr, pw := io.Pipe()
	defer pw.Close()
	out := &syncBuilder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := NewPairer(ctx, s, NewTerminalApprover(pr, out))

	asked, _ := p.Start("asked", "192.168.0.2", 1)
	queued, _ := p.Start("queued", "192.168.0.3", 1)
	for !strings.Contains(out.String(), asked.Code) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := p.Pending(); len(got) != 2 || got[0].ID != asked.ID || got[1].ID != queued.ID {
		t.Fatalf("Pending() = %+v, want both requests oldest first", got)
	}

	if err := p.Answer(queued.ID, false); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, queued.ID, StateDenied)
	if err := p.Answer(asked.ID, true); err != nil {
		t.Fatal(err)
	}
	dev, sec := waitState(t, p, asked.ID, StateApproved)
	if _, ok, _ := s.Verify(dev, sec); !ok {
		t.Fatal("credentials approved through Answer should verify")
	}
	if err := p.Answer(asked.ID, false); err == nil {
		t.Fatal("answering a finished request should fail")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(out.String(), "answered in the spareglass window") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(out.String(), "answered in the spareglass window") {
		t.Fatalf("terminal should say the prompt was answered elsewhere, got:\n%s", out.String())
	}
	if strings.Count(out.String(), "Allow?") != 1 {
		t.Fatalf("the queued request was answered before its turn and must not be asked, got:\n%s", out.String())
	}
	if len(p.Pending()) != 0 {
		t.Fatal("no request should be pending")
	}
}

func TestAnswerWithoutTerminalInput(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "devices.json"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := NewPairer(ctx, s, NewTerminalApprover(strings.NewReader(""), io.Discard))

	r, _ := p.Start("no terminal", "192.168.0.2", 1)
	time.Sleep(100 * time.Millisecond)
	if st, _, _, _ := p.Status(r.ID); st != StatePending {
		t.Fatalf("with no terminal input the request must wait for an answer elsewhere, got %s", st)
	}
	if err := p.Answer(r.ID, true); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, r.ID, StateApproved)
}
