package pairing

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// TerminalApprover asks on a terminal: "... Allow? [y/N]".
type TerminalApprover struct {
	out   io.Writer
	lines chan string
}

// NewTerminalApprover reads answers from in (usually os.Stdin). If in reaches
// EOF (no terminal attached), Ask leaves the request to be answered elsewhere
// (Pairer.Answer) until its context ends. Lines typed while no prompt is shown
// are discarded, so a stray "y" never answers a later request.
func NewTerminalApprover(in io.Reader, out io.Writer) *TerminalApprover {
	t := &TerminalApprover{out: out, lines: make(chan string, 64)}
	go func() {
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			select {
			case t.lines <- sc.Text():
			default:
			}
		}
		close(t.lines)
	}()
	return t
}

// discardTyped drops lines read before the prompt. It reports false once the
// input has reached EOF.
func (t *TerminalApprover) discardTyped() bool {
	for {
		select {
		case _, ok := <-t.lines:
			if !ok {
				return false
			}
		default:
			return true
		}
	}
}

// Ask implements Approver.
func (t *TerminalApprover) Ask(ctx context.Context, r *Request) bool {
	if !t.discardTyped() {
		fmt.Fprintf(t.out, "\n  %s (%s) wants to connect as display %d (code %s). Answer in the spareglass window.\n", r.Name, r.Addr, r.Slot, r.Code)
		<-ctx.Done()
		return false
	}
	fmt.Fprintf(t.out, "\n  %s (%s) wants to connect as display %d.\n  Code shown on that screen: %s\n  Allow? [y/N] ",
		r.Name, r.Addr, r.Slot, r.Code)
	lines := t.lines
	for {
		select {
		case <-ctx.Done():
			if errors.Is(context.Cause(ctx), ErrAnswered) {
				fmt.Fprintln(t.out, "\n  (answered in the spareglass window)")
			} else {
				fmt.Fprintln(t.out, "\n  (no answer; request expired)")
			}
			return false
		case line, ok := <-lines:
			if !ok {
				lines = nil
				fmt.Fprintln(t.out, "\n  (no terminal input; answer in the spareglass window)")
				continue
			}
			if ctx.Err() != nil {
				continue // answered elsewhere or expired meanwhile; report that instead
			}
			switch strings.ToLower(strings.TrimSpace(line)) {
			case "y", "yes":
				fmt.Fprintf(t.out, "  Allowed %s.\n\n", r.Name)
				return true
			case "", "n", "no":
				fmt.Fprintf(t.out, "  Denied %s.\n\n", r.Name)
				return false
			default:
				fmt.Fprint(t.out, "  Please answer y or n: ")
			}
		}
	}
}
