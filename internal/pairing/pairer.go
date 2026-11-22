package pairing

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"
)

// RequestTTL is how long a pairing request waits for an answer.
const RequestTTL = 2 * time.Minute

// maxPending caps unanswered requests so a page cannot flood the host.
const maxPending = 4

// ErrTooManyRequests is returned when maxPending requests are waiting.
var ErrTooManyRequests = errors.New("too many pairing requests are waiting")

// ErrNoSuchRequest is returned by Answer when the request is unknown or no
// longer waiting for an answer.
var ErrNoSuchRequest = errors.New("no such pairing request is waiting")

// ErrAnswered is the cause of the context an Approver gets when the request
// was answered through Pairer.Answer while the Approver was still asking.
var ErrAnswered = errors.New("answered elsewhere")

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
	// asking is the request the approver is asking about now; stopAsking
	// ends that question when the request is answered through Answer.
	asking     *Request
	stopAsking context.CancelCauseFunc
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
			deadlineCtx, cancelDeadline := context.WithDeadline(ctx, r.Created.Add(RequestTTL))
			askCtx, stop := context.WithCancelCause(deadlineCtx)
			if !p.startAsking(r, stop) {
				stop(nil)
				cancelDeadline()
				continue
			}
			ok := p.approver.Ask(askCtx, r)
			p.mu.Lock()
			p.asking, p.stopAsking = nil, nil
			switch {
			case ok:
				p.approveLocked(r)
			case deadlineCtx.Err() != nil:
				p.finishLocked(r, StateExpired)
			default:
				p.finishLocked(r, StateDenied)
			}
			p.mu.Unlock()
			stop(nil)
			cancelDeadline()
		}
	}
}

// Pending returns the requests still waiting for an answer, oldest first.
func (p *Pairer) Pending() []Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	var list []Request
	for _, r := range p.reqs {
		if r.state == StatePending && time.Since(r.Created) <= RequestTTL {
			list = append(list, Request{ID: r.ID, Code: r.Code, Name: r.Name, Addr: r.Addr, Slot: r.Slot, Created: r.Created, state: r.state})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.Before(list[j].Created) })
	return list
}

// Answer approves or denies a waiting request, whether or not the approver is
// asking about it yet. An approver still asking about it sees its context
// cancelled with cause ErrAnswered.
func (p *Pairer) Answer(id string, allow bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.reqs[id]
	if !ok || r.state != StatePending {
		return ErrNoSuchRequest
	}
	if time.Since(r.Created) > RequestTTL {
		p.finishLocked(r, StateExpired)
		return ErrNoSuchRequest
	}
	if allow {
		p.approveLocked(r)
	} else {
		p.finishLocked(r, StateDenied)
	}
	if p.asking == r {
		p.stopAsking(ErrAnswered)
	}
	return nil
}

// approveLocked stores the device unless the request already ended (for
// example it expired or was answered elsewhere while the owner was typing).
func (p *Pairer) approveLocked(r *Request) {
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

// startAsking marks r as the request the approver is asked about, unless it
// needs no question: it was answered through Answer or expired while it
// waited in the queue. Both happen under one lock, so Answer either finds r
// already ended or finds it being asked and stops the question.
func (p *Pairer) startAsking(r *Request, stop context.CancelCauseFunc) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.state == StatePending && time.Since(r.Created) > RequestTTL {
		p.finishLocked(r, StateExpired)
	}
	if r.state != StatePending {
		return false
	}
	p.asking, p.stopAsking = r, stop
	return true
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
