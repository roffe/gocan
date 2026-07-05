package gocan

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"sync"
)

// ErrClosed is the context cause set when a bus is shut down via Close. It is
// reported as a clean shutdown (Bus.Err returns nil) rather than a failure.
var ErrClosed = errors.New("gocan: bus closed")

// Option configures a Bus before its adapter is opened.
type Option func(*Bus)

// WithEventFunc registers fn as a synchronous event handler. Handlers run in
// registration order on the goroutine that raised the event; keep them quick.
func WithEventFunc(fn func(Event)) Option {
	return func(b *Bus) { b.addSink(fn) }
}

// WithLogger forwards every adapter event to l at the matching slog level.
// Error and fatal events attach the underlying error under the "err" key.
func WithLogger(l *slog.Logger) Option {
	return func(b *Bus) {
		b.addSink(func(e Event) {
			var attrs []slog.Attr
			if e.Err != nil {
				attrs = append(attrs, slog.Any("err", e.Err))
			}
			l.LogAttrs(context.Background(), e.Type.Level(), e.Details, attrs...)
		})
	}
}

// Bus is a connection to a CAN bus through an adapter. It fans incoming
// frames out to subscribers and serializes outgoing frames to the adapter.
type Bus struct {
	adapter Adapter
	name    string

	// ctx is cancelled when the bus terminates. The cancel cause carries the
	// fatal adapter error, or ErrClosed on a clean shutdown.
	ctx    context.Context
	cancel context.CancelCauseFunc

	sendMu sync.Mutex

	subMu      sync.Mutex
	submap     map[uint32]map[*sub]struct{}
	globalSubs []*sub

	sinkMu sync.Mutex
	sinks  []*eventSink

	closeOnce sync.Once
}

// Open constructs the named adapter from the registry (see AdapterNames)
// with cfg and connects to it.
func Open(ctx context.Context, adapterName string, cfg Config, opts ...Option) (*Bus, error) {
	info, err := lookupAdapter(adapterName)
	if err != nil {
		return nil, err
	}
	adapter, err := info.New(cfg)
	if err != nil {
		return nil, err
	}
	return open(ctx, info.Name, adapter, opts...)
}

// OpenAdapter connects to an already-constructed adapter. Options can
// register event listeners before the adapter starts. AdapterName reports
// the name the adapter was constructed under (see gocan.AdapterName).
func OpenAdapter(ctx context.Context, adapter Adapter, opts ...Option) (*Bus, error) {
	return open(ctx, AdapterName(adapter), adapter, opts...)
}

func open(ctx context.Context, name string, adapter Adapter, opts ...Option) (*Bus, error) {
	cctx, cancel := context.WithCancelCause(ctx)
	b := &Bus{
		adapter: adapter,
		name:    name,
		ctx:     cctx,
		cancel:  cancel,
		submap:  make(map[uint32]map[*sub]struct{}),
	}
	for _, opt := range opts {
		opt(b)
	}
	// Wake pending Recv/Request/Subscribe consumers whenever the bus dies,
	// for whatever reason.
	context.AfterFunc(cctx, b.releaseAllSubs)
	if err := adapter.Open(cctx, b); err != nil {
		cancel(err)
		return nil, err
	}
	return b, nil
}

// Adapter returns the underlying adapter.
func (b *Bus) Adapter() Adapter { return b.adapter }

// AdapterName returns the registry name the bus was opened with.
func (b *Bus) AdapterName() string { return b.name }

// Context returns the bus context. It is cancelled when the bus is closed or
// the adapter fails fatally; context.Cause reports why (ErrClosed on a clean
// shutdown, otherwise the fatal adapter error).
func (b *Bus) Context() context.Context { return b.ctx }

// Done returns a channel that is closed when the bus terminates, for use in
// a select alongside your own work.
func (b *Bus) Done() <-chan struct{} { return b.ctx.Done() }

// Err returns the fatal adapter error that terminated the bus, or nil if it
// is still running or was shut down cleanly (via Close or context
// cancellation).
func (b *Bus) Err() error {
	cause := context.Cause(b.ctx)
	switch {
	case cause == nil,
		errors.Is(cause, ErrClosed),
		errors.Is(cause, context.Canceled),
		errors.Is(cause, context.DeadlineExceeded):
		return nil
	default:
		return cause
	}
}

// Wait blocks until ctx is done or the bus terminates, returning the fatal
// adapter error if one occurred (nil on a clean shutdown or ctx cancellation).
func (b *Bus) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		if err := b.Err(); err != nil {
			return err
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return ctx.Err()
	case <-b.ctx.Done():
		return b.Err()
	}
}

// Close shuts down the bus and the underlying adapter. The bus context is
// cancelled first so adapter read loops can distinguish a clean shutdown
// (bus already done) from a dead port (report via Fatal).
func (b *Bus) Close() (err error) {
	b.closeOnce.Do(func() {
		b.cancel(ErrClosed)
		err = b.adapter.Close()
	})
	return
}

// alive returns nil while the bus is running, otherwise the reason it died.
func (b *Bus) alive() error {
	select {
	case <-b.ctx.Done():
		if err := b.Err(); err != nil {
			return err
		}
		return ErrClosed
	default:
		return nil
	}
}

// Send writes one frame to the bus, returning once the adapter has written
// it (or ctx is done). Concurrent callers are serialized, giving natural
// inter-frame pacing.
func (b *Bus) Send(ctx context.Context, f Frame) error {
	if err := b.alive(); err != nil {
		return err
	}
	b.sendMu.Lock()
	defer b.sendMu.Unlock()
	return b.adapter.Send(ctx, f)
}

// Recv waits for a single frame carrying one of the given identifiers (no
// identifiers = any frame). Bound it with a context deadline.
func (b *Bus) Recv(ctx context.Context, identifiers ...uint32) (Frame, error) {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s := b.newSub(sctx, 1, identifiers)
	return b.waitSub(ctx, s)
}

// Request sends frame and waits for a reply carrying one of the given
// identifiers. Bound it with a context deadline. Unless the caller already
// set one, an expected-responses hint of 1 is stamped for buffered adapters;
// use WithExpectedResponses for commands answered by multiple frames.
func (b *Bus) Request(ctx context.Context, frame Frame, replyIdentifiers ...uint32) (Frame, error) {
	if ExpectedResponses(ctx) == 0 {
		ctx = WithExpectedResponses(ctx, 1)
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s := b.newSub(sctx, 1, replyIdentifiers)
	if err := b.Send(ctx, frame); err != nil {
		return Frame{}, err
	}
	return b.waitSub(ctx, s)
}

// Subscribe returns a channel delivering frames that carry one of the given
// identifiers (no identifiers = all traffic). Delivery is non-blocking: a
// subscriber that stops draining loses frames. The channel is closed when
// ctx is cancelled or the bus terminates.
func (b *Bus) Subscribe(ctx context.Context, identifiers ...uint32) <-chan Frame {
	return b.newSub(ctx, 64, identifiers).ch
}

// Frames returns an iterator over frames carrying one of the given
// identifiers. It ends when ctx is cancelled, the bus terminates, or the
// loop breaks.
func (b *Bus) Frames(ctx context.Context, identifiers ...uint32) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		sctx, cancel := context.WithCancel(ctx)
		defer cancel()
		for f := range b.Subscribe(sctx, identifiers...) {
			if !yield(f) {
				return
			}
		}
	}
}

// waitSub returns the first frame delivered to s, or the reason none will come.
func (b *Bus) waitSub(ctx context.Context, s *sub) (Frame, error) {
	select {
	case f, ok := <-s.ch:
		if !ok {
			if err := b.alive(); err != nil {
				return Frame{}, err
			}
			if cause := context.Cause(ctx); cause != nil {
				return Frame{}, cause
			}
			return Frame{}, ErrClosed
		}
		return f, nil
	case <-ctx.Done():
		return Frame{}, context.Cause(ctx)
	}
}

// sub is a single subscription. Its channel is closed exactly once, under
// subMu, when the subscription is released.
type sub struct {
	ids  map[uint32]struct{}
	ch   chan Frame
	once sync.Once
}

func (b *Bus) newSub(ctx context.Context, buffer int, identifiers []uint32) *sub {
	s := &sub{ch: make(chan Frame, buffer)}
	if len(identifiers) > 0 {
		s.ids = make(map[uint32]struct{}, len(identifiers))
		for _, id := range identifiers {
			s.ids[id] = struct{}{}
		}
	}
	b.subMu.Lock()
	if b.alive() != nil {
		// The bus is already dead and releaseAllSubs may have run; hand back
		// a closed channel instead of a subscription that never ends.
		b.subMu.Unlock()
		s.once.Do(func() { close(s.ch) })
		return s
	}
	if s.ids == nil {
		b.globalSubs = append(b.globalSubs, s)
	} else {
		for id := range s.ids {
			m, ok := b.submap[id]
			if !ok {
				m = make(map[*sub]struct{})
				b.submap[id] = m
			}
			m[s] = struct{}{}
		}
	}
	b.subMu.Unlock()
	context.AfterFunc(ctx, func() { b.releaseSub(s) })
	return s
}

func (b *Bus) releaseSub(s *sub) {
	s.once.Do(func() {
		b.subMu.Lock()
		defer b.subMu.Unlock()
		if s.ids == nil {
			for i, cur := range b.globalSubs {
				if cur == s {
					b.globalSubs = append(b.globalSubs[:i], b.globalSubs[i+1:]...)
					break
				}
			}
		} else {
			for id := range s.ids {
				if m, ok := b.submap[id]; ok {
					delete(m, s)
					if len(m) == 0 {
						delete(b.submap, id)
					}
				}
			}
		}
		// Closing under subMu is safe against Deliver, which sends under the
		// same lock.
		close(s.ch)
	})
}

func (b *Bus) releaseAllSubs() {
	b.subMu.Lock()
	subs := append([]*sub{}, b.globalSubs...)
	for _, m := range b.submap {
		for s := range m {
			subs = append(subs, s)
		}
	}
	b.subMu.Unlock()
	for _, s := range subs {
		b.releaseSub(s)
	}
}

// Deliver hands an incoming frame from the adapter to every matching
// subscriber. Delivery is non-blocking; subscribers that have fallen behind
// lose the frame.
func (b *Bus) Deliver(f Frame) {
	dropped := 0
	b.subMu.Lock()
	for _, s := range b.globalSubs {
		select {
		case s.ch <- f:
		default:
			dropped++
		}
	}
	if m, ok := b.submap[f.ID]; ok {
		for s := range m {
			select {
			case s.ch <- f:
			default:
				dropped++
			}
		}
	}
	b.subMu.Unlock()
	if dropped > 0 {
		// Emitted outside subMu so a sink may subscribe without deadlocking.
		b.Emit(Event{Type: EventTypeWarning, Details: fmt.Sprintf("dropped frame 0x%03X for %d slow subscriber(s)", f.ID, dropped)})
	}
}

// Emit forwards an adapter event to every registered listener, in
// registration order, on the calling goroutine.
func (b *Bus) Emit(e Event) {
	b.sinkMu.Lock()
	sinks := b.sinks
	b.sinkMu.Unlock()
	for _, s := range sinks {
		s.fn(e)
	}
}

// Fatal reports an unrecoverable adapter failure: it emits a final fatal
// event and terminates the bus with err as the cause.
func (b *Bus) Fatal(err error) {
	if err == nil {
		return
	}
	b.Emit(Event{Type: EventTypeFatal, Details: err.Error(), Err: err})
	b.cancel(err)
}

// eventSink wraps a registered event callback. A pointer identity lets
// OnEvent hand back a working unregister function (func values are not
// comparable).
type eventSink struct {
	fn func(Event)
}

func (b *Bus) addSink(fn func(Event)) *eventSink {
	if fn == nil {
		return nil
	}
	s := &eventSink{fn: fn}
	b.sinkMu.Lock()
	// Append onto a fresh slice so Emit can snapshot the header lock-free.
	b.sinks = append(b.sinks[:len(b.sinks):len(b.sinks)], s)
	b.sinkMu.Unlock()
	return s
}

// OnEvent registers fn to receive subsequent events and returns a function
// that unregisters it. Unlike the construction-time options it can be called
// at any time and supports multiple independent listeners.
func (b *Bus) OnEvent(fn func(Event)) (cancel func()) {
	s := b.addSink(fn)
	if s == nil {
		return func() {}
	}
	return func() {
		b.sinkMu.Lock()
		defer b.sinkMu.Unlock()
		for i, cur := range b.sinks {
			if cur == s {
				next := make([]*eventSink, 0, len(b.sinks)-1)
				next = append(next, b.sinks[:i]...)
				next = append(next, b.sinks[i+1:]...)
				b.sinks = next
				return
			}
		}
	}
}
