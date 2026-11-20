package pairing

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// RequestTTL is how long a pairing request waits for an answer.
const RequestTTL = 2 * time.Minute

// maxPending caps unanswered requests so a page cannot flood the host.
const maxPending = 4

// ErrTooManyRequests is returned when maxPending requests are waiting.
var ErrTooManyRequests = errors.New("too many pairing requests are waiting")

// State of a pairing request.
type State string

const (
	StatePending  State = "pending"
	StateApproved State = "approved"
	StateDenied   State = "denied"
	StateExpired  State = "expired"
)

// Request is one browser asking to pair.
type Request struct {
	ID      string
	Code    string // shown on both screens so the owner knows which browser asks
	Name    string // e.g. "Safari on iPad"
	Addr    string // remote IP
	Slot    int
	Created time.Time

	state    State
	deviceID string
	secret   string
}

// Approver asks the host owner about a request and blocks until they answer,
// ctx is cancelled, or the request expires.
type Approver interface {
	Ask(ctx context.Context, r *Request) bool
}

// ApproverFunc adapts a function to Approver.
type ApproverFunc func(ctx context.Context, r *Request) bool

// Ask calls f.
func (f ApproverFunc) Ask(ctx context.Context, r *Request) bool { return f(ctx, r) }

// Pairer runs the pairing flow: Start → owner approves → Status hands the
// browser its credentials once.
type Pairer struct {
	store    *Store
	approver Approver
	queue    chan *Request

	mu      sync.Mutex
	reqs    map[string]*Request
	pending int
}

// NewPairer starts a worker that asks the approver one request at a time.
func NewPairer(ctx context.Context, store *Store, approver Approver) *Pairer {
	p := &Pairer{store: store, approver: approver, queue: make(chan *Request, maxPending), reqs: map[string]*Request{}}
	go p.run(ctx)
	return p
}

// Store returns the device store.
func (p *Pairer) Store() *Store { return p.store }

// Start creates a pairing request. An address with a request still waiting
// gets that request back, so one device cannot fill the queue.
func (p *Pairer) Start(name, addr string, slot int) (*Request, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gcLocked()
	for _, r := range p.reqs {
		if r.Addr == addr && r.state == StatePending && time.Since(r.Created) <= RequestTTL {
			return r, nil
		}
	}
	if p.pending >= maxPending {
		return nil, ErrTooManyRequests
	}
	r := &Request{ID: randomHex(8), Code: fourDigits(), Name: name, Addr: addr, Slot: slot, Created: time.Now(), state: StatePending}
	select {
	case p.queue <- r:
	default:
		return nil, ErrTooManyRequests
	}
	p.reqs[r.ID] = r
	p.pending++
	return r, nil
}

// Status reports a request's state. For an approved request it returns the
// device credentials the first time only.
func (p *Pairer) Status(id string) (st State, deviceID, secret string, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, found := p.reqs[id]
	if !found {
		return "", "", "", false
	}
	if r.state == StatePending && time.Since(r.Created) > RequestTTL {
		p.finishLocked(r, StateExpired)
	}
	st, deviceID, secret = r.state, r.deviceID, r.secret
	r.secret = ""
	return st, deviceID, secret, true
}

func (p *Pairer) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case r := <-p.queue:
			if time.Since(r.Created) > RequestTTL {
				p.finish(r, StateExpired)
				continue
			}
			askCtx, cancel := context.WithDeadline(ctx, r.Created.Add(RequestTTL))
			ok := p.approver.Ask(askCtx, r)
			expired := askCtx.Err() != nil
			cancel()
			switch {
			case ok:
				p.approve(r)
			case expired:
				p.finish(r, StateExpired)
			default:
				p.finish(r, StateDenied)
			}
		}
	}
}

// approve stores the device unless the request already ended (for example
// it expired while the owner was typing).
func (p *Pairer) approve(r *Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.state != StatePending {
		return
	}
	id, secret, err := p.store.Add(r.Name)
	if err != nil {
		p.finishLocked(r, StateDenied)
		return
	}
	r.deviceID, r.secret = id, secret
	p.finishLocked(r, StateApproved)
}

func (p *Pairer) finish(r *Request, st State) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finishLocked(r, st)
}

func (p *Pairer) finishLocked(r *Request, st State) {
	if r.state != StatePending {
		return
	}
	p.pending--
	r.state = st
}

// gcLocked drops requests nobody will ask about anymore.
func (p *Pairer) gcLocked() {
	for id, r := range p.reqs {
		if time.Since(r.Created) > 2*RequestTTL {
			if r.state == StatePending {
				p.pending--
			}
			delete(p.reqs, id)
		}
	}
}

func fourDigits() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%04d", n.Int64())
}
