package pairing

import (
	"bufio"
	"context"
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
// EOF (no terminal attached), every later request is denied. Lines typed while
// no prompt is shown are discarded, so a stray "y" never answers a later request.
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
		fmt.Fprintf(t.out, "\n  %s (%s) wants to connect as display %d, but there is no terminal input; request denied.\n", r.Name, r.Addr, r.Slot)
		return false
	}
	fmt.Fprintf(t.out, "\n  %s (%s) wants to connect as display %d.\n  Code shown on that screen: %s\n  Allow? [y/N] ",
		r.Name, r.Addr, r.Slot, r.Code)
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(t.out, "\n  (no answer; request expired)")
			return false
		case line, ok := <-t.lines:
			if !ok {
				fmt.Fprintln(t.out, "\n  (no terminal input; request denied)")
				return false
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
