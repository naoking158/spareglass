package pairing

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestTerminalApproverIgnoresInputTypedBeforePrompt(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	var out strings.Builder
	a := NewTerminalApprover(pr, &out)

	if _, err := pw.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if a.Ask(ctx, &Request{Name: "Stranger", Code: "1111", Slot: 1}) {
		t.Fatal("a line typed before the prompt must not approve the request")
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = pw.Write([]byte("y\n"))
	}()
	if !a.Ask(context.Background(), &Request{Name: "Owner", Code: "2222", Slot: 1}) {
		t.Fatal("a line typed after the prompt should approve")
	}
}
